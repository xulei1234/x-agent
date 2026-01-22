package common

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// Test helper: use the current test binary as a subprocess to simulate commands deterministically.
// Pattern: exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "caseName")
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	// Find "--" separator
	args := os.Args
	sep := -1
	for i := range args {
		if args[i] == "--" {
			sep = i
			break
		}
	}
	if sep < 0 || sep+1 >= len(args) {
		os.Exit(2)
	}
	switch args[sep+1] {
	case "ok":
		os.Stdout.WriteString("hello\n")
		os.Stderr.WriteString("warn\n")
		os.Exit(0)
	case "fail42":
		os.Stdout.WriteString("out\n")
		os.Stderr.WriteString("err\n")
		os.Exit(42)
	case "bigout":
		// deterministic big output without allocating too much in helper itself
		for i := 0; i < 2000; i++ {
			os.Stdout.WriteString("0123456789")
		}
		os.Exit(0)
	case "sleep":
		time.Sleep(2 * time.Second)
		os.Exit(0)
	case "asyncLines":
		// interleave stdout/stderr, line-based
		os.Stdout.WriteString("a1\n")
		os.Stderr.WriteString("e1\n")
		os.Stdout.WriteString("a2\n")
		os.Stderr.WriteString("e2\n")
		os.Exit(0)
	default:
		os.Exit(3)
	}
}

func helperCmd(t *testing.T, helperCase string, ctx context.Context) *exec.Cmd {
	t.Helper()
	if ctx == nil {
		return exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", helperCase)
	}
	return exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess", "--", helperCase)
}

func setupViperForCmdTests() func() {
	// Keep tests isolated
	old := viper.New()
	*viper.GetViper() = *old

	viper.Set("Cmd.MaxOutputBytes", 1024)
	viper.Set("Cmd.MaxLineBytes", 64*1024)

	return func() {
		// reset best-effort
		*viper.GetViper() = *viper.New()
	}
}

func TestSyncExec_Success(t *testing.T) {
	defer setupViperForCmdTests()()

	cmd := helperCmd(t, "ok", nil)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	body := SyncExec(cmd)
	if body == nil {
		t.Fatalf("body is nil")
	}
	if body.Code != 0 {
		t.Fatalf("expected code=0, got=%d, stderr=%s", body.Code, string(body.Stderr))
	}
	if !strings.Contains(string(body.Stdout), "hello") {
		t.Fatalf("stdout mismatch: %s", string(body.Stdout))
	}
}

func TestSyncExec_FailureExitCode(t *testing.T) {
	defer setupViperForCmdTests()()

	cmd := helperCmd(t, "fail42", nil)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	body := SyncExec(cmd)
	if body == nil {
		t.Fatalf("body is nil")
	}
	if body.Code != 42 {
		t.Fatalf("expected code=42, got=%d", body.Code)
	}
	// CombinedOutput merges stderr into stdout; stderr field carries err.Error()
	if len(body.Stdout) == 0 {
		t.Fatalf("expected stdout not empty")
	}
	if len(body.Stderr) == 0 {
		t.Fatalf("expected stderr not empty (should contain error text)")
	}
}

func TestSyncExec_OutputClamped(t *testing.T) {
	defer setupViperForCmdTests()()

	viper.Set("Cmd.MaxOutputBytes", 128)

	cmd := helperCmd(t, "bigout", nil)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	body := SyncExec(cmd)
	if body == nil {
		t.Fatalf("body is nil")
	}
	if body.Code != 0 {
		t.Fatalf("expected code=0, got=%d", body.Code)
	}
	if len(body.Stdout) > 128 {
		t.Fatalf("expected clamped stdout <= 128, got=%d", len(body.Stdout))
	}
	if !strings.Contains(string(body.Stdout), "truncated") {
		t.Fatalf("expected truncated suffix in stdout, got=%q", string(body.Stdout))
	}
}

func TestSyncExec_Timeout(t *testing.T) {
	defer setupViperForCmdTests()()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cmd := helperCmd(t, "sleep", ctx)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	body := SyncExec(cmd)
	if body == nil {
		t.Fatalf("body is nil")
	}
	if body.Code == 0 {
		t.Fatalf("expected non-zero code on timeout, got=0")
	}
	if len(body.Stderr) == 0 {
		t.Fatalf("expected stderr not empty on timeout")
	}
}

func TestAsyncExec_StreamsAndClose(t *testing.T) {
	defer setupViperForCmdTests()()

	cmd := helperCmd(t, "asyncLines", nil)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	ch := make(chan ExecRes, 128)
	go AsyncExec(cmd, ch)

	var got []string
	for r := range ch {
		if r.Err != nil {
			t.Fatalf("unexpected err: %v", r.Err)
		}
		if len(r.Buf) == 0 {
			continue
		}
		got = append(got, string(r.Buf))
	}

	joined := strings.Join(got, "")
	// expect both stdout and stderr lines to be present (prefixing may or may not exist depending on impl)
	if !strings.Contains(joined, "a1") || !strings.Contains(joined, "e1") || !strings.Contains(joined, "a2") || !strings.Contains(joined, "e2") {
		t.Fatalf("missing expected output, got=%q", joined)
	}
}

func TestAsyncExec_CommandFailureSendsErr(t *testing.T) {
	defer setupViperForCmdTests()()

	cmd := helperCmd(t, "fail42", nil)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	ch := make(chan ExecRes, 128)
	go AsyncExec(cmd, ch)

	seenErr := false
	for r := range ch {
		if r.Err != nil {
			seenErr = true
			break
		}
	}
	if !seenErr {
		t.Fatalf("expected to see an error event")
	}
}

func TestTrySend_NonBlocking(t *testing.T) {
	ch := make(chan ExecRes, 1)
	ch <- ExecRes{Buf: []byte("full")}
	// should not block even if channel is full
	done := make(chan struct{})
	go func() {
		trySend(ch, ExecRes{Buf: []byte("drop")})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("trySend blocked")
	}
}

func TestClampBytes(t *testing.T) {
	in := []byte("abcdefghijklmnopqrstuvwxyz")
	out := clampBytes(in, 10)
	if len(out) != 10 {
		t.Fatalf("expected len=10 got=%d", len(out))
	}
	// maxBytes <= 0: no clamp
	out2 := clampBytes(in, 0)
	if len(out2) != len(in) {
		t.Fatalf("expected no clamp")
	}
}

func TestExitCodeFromErr(t *testing.T) {
	// nil
	if exitCodeFromErr(nil) != 0 {
		t.Fatalf("expected 0")
	}
	// context errors should map to non-zero (\-1)
	if exitCodeFromErr(context.DeadlineExceeded) == 0 {
		t.Fatalf("expected non-zero for deadline")
	}
	if exitCodeFromErr(context.Canceled) == 0 {
		t.Fatalf("expected non-zero for canceled")
	}

	// ExitError extraction (skip on windows because WaitStatus differs)
	if runtime.GOOS == "windows" {
		t.Skip("unix wait status test skipped on windows")
	}

	cmd := exec.Command("sh", "-c", "exit 7")
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected err")
	}
	code := exitCodeFromErr(err)
	if code != 7 {
		t.Fatalf("expected 7 got=%d", code)
	}

	// non ExitError / unknown error
	code = exitCodeFromErr(errors.New("x"))
	if code == 0 {
		t.Fatalf("expected non-zero for generic error")
	}
}

// Optional: verify AsyncExec never hangs when consumer is slow (best-effort).
func TestAsyncExec_SlowConsumer_NoDeadlock(t *testing.T) {
	defer setupViperForCmdTests()()

	cmd := helperCmd(t, "asyncLines", nil)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	ch := make(chan ExecRes, 1) // small buffer to stress trySend / producer behavior
	go AsyncExec(cmd, ch)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range ch {
			time.Sleep(10 * time.Millisecond)
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("possible deadlock: async exec did not complete")
	}
}

// Windows note: if your SyncExec / AsyncExec uses syscall.WaitStatus directly, this will fail on Windows.
// Keep OS-specific extraction in cmd.go (or guard tests) if needed.
func TestWaitStatus_TypeAssertion_SafetyHint(t *testing.T) {
	if runtime.GOOS == "windows" {
		_ = syscall.Errno(0) // keep syscall imported on Windows builds
	}
}
