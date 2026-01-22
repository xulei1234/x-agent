package common

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/xulei1234/x-proto/xps"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

// SyncExec 同步执行命令，并返回执行结果。
// 特点：
//  1. 使用 CombinedOutput 简化流程，减少管道/Wait 竞态
//  2. 统一提取退出码
//  3. 限制输出大小，避免大输出导致内存膨胀
func SyncExec(cmd *exec.Cmd) *xps.Body {
	logrus.Infoln("==> 同步执行命令开始")

	var body xps.Body
	if cmd == nil {
		body.Code = -1
		body.Stderr = []byte("nil cmd")
		return &body
	}

	// 统一以配置兜底，避免未设置时无限输出
	maxBytes := viper.GetInt("Cmd.MaxOutputBytes")
	if maxBytes <= 0 {
		// 默认 1MiB
		maxBytes = 1 << 20
	}

	out, err := cmd.CombinedOutput()
	out = clampBytes(out, maxBytes)

	body.Stdout = out
	body.Code = 0

	if err == nil {
		logrus.Infoln("==> 同步执行命令结束: success")
		return &body
	}

	// 提取退出码与错误信息
	code := exitCodeFromErr(err)
	body.Code = int32(code)

	// ctx/超时信息更明确（CommandContext 常见）
	if cmd.ProcessState == nil && errors.Is(err, context.DeadlineExceeded) {
		body.Stderr = []byte(err.Error())
		logrus.WithError(err).Warn("==> 同步执行命令结束: deadline exceeded")
		return &body
	}

	// ExitError 的 Stderr 在 CombinedOutput 已合在 Stdout，这里附加错误文本即可
	body.Stderr = []byte(err.Error())
	logrus.WithError(err).Warnf("==> 同步执行命令结束: code=%d", code)
	return &body
}

// AsyncExec 异步执行命令：按行实时输出（stdout/stderr 都会透传到 resCh）。
// 保证：函数退出时一定 close(resCh)，并避免无限阻塞/无限内存增长。
func AsyncExec(cmd *exec.Cmd, resCh chan<- ExecRes) {
	defer func() {
		// 保证消费者可退出
		if resCh != nil {
			close(resCh)
		}
	}()

	if cmd == nil {
		trySend(resCh, ExecRes{Err: fmt.Errorf("nil cmd")})
		return
	}
	if resCh == nil {
		// 无法回传，直接执行并返回
		_ = cmd.Run()
		return
	}

	// 输出上限与单行上限
	maxBytes := viper.GetInt("Cmd.MaxOutputBytes")
	if maxBytes <= 0 {
		maxBytes = 1 << 20 // 1MiB
	}
	maxLine := viper.GetInt("Cmd.MaxLineBytes")
	if maxLine <= 0 {
		maxLine = 64 << 10 // 64KiB
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		trySend(resCh, ExecRes{Err: fmt.Errorf("StdoutPipe: %w", err)})
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		trySend(resCh, ExecRes{Err: fmt.Errorf("StderrPipe: %w", err)})
		return
	}

	if err := cmd.Start(); err != nil {
		trySend(resCh, ExecRes{Err: fmt.Errorf("Start: %w", err)})
		return
	}

	// 并行读取 stdout/stderr，避免互相阻塞
	var wg sync.WaitGroup
	wg.Add(2)

	var used int64
	var usedMu sync.Mutex

	consume := func(r io.Reader, stream string) {
		defer wg.Done()

		sc := bufio.NewScanner(r)
		// Scanner 默认 token 太小，必须放大，否则长行会 ErrTooLong
		buf := make([]byte, 0, 64<<10)
		sc.Buffer(buf, maxLine)

		for sc.Scan() {
			b := sc.Bytes()
			if len(b) == 0 {
				continue
			}

			// 统一加换行，保持“按行”语义，避免客户端拼接困难
			line := append([]byte(nil), b...)
			if len(line) == 0 || line[len(line)-1] != '\n' {
				line = append(line, '\n')
			}

			// 总量限制：到达上限后停止继续读取，防止内存/带宽被打爆
			usedMu.Lock()
			if used+int64(len(line)) > int64(maxBytes) {
				remain := int64(maxBytes) - used
				if remain > 0 {
					line = line[:remain]
					used = int64(maxBytes)
					usedMu.Unlock()
					trySend(resCh, ExecRes{Buf: prefixStream(stream, line)})
				} else {
					usedMu.Unlock()
				}
				return
			}
			used += int64(len(line))
			usedMu.Unlock()

			trySend(resCh, ExecRes{Buf: prefixStream(stream, line)})
		}

		if err := sc.Err(); err != nil {
			trySend(resCh, ExecRes{Err: fmt.Errorf("%s scan: %w", stream, err)})
		}
	}

	go consume(stdout, "stdout")
	go consume(stderr, "stderr")

	// 等待输出消费完成再 Wait，避免 Wait 先返回导致 pipe 未读完
	wg.Wait()

	err = cmd.Wait()
	if err != nil {
		// 让上游知道失败原因与退出码
		trySend(resCh, ExecRes{Err: fmt.Errorf("wait: %w (code=%d)", err, exitCodeFromErr(err))})
		return
	}
}

// exitCodeFromErr 从 error 中提取退出码（兼容正常退出/信号/非 ExitError）。
func exitCodeFromErr(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		// Unix 下可取 WaitStatus
		if st, ok := ee.Sys().(syscall.WaitStatus); ok {
			if st.Signaled() {
				// 常见：被 SIGKILL/SIGTERM
				return 128 + int(st.Signal())
			}
			return st.ExitStatus()
		}
	}
	// context 取消/超时等情况给统一非 0
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return -1
	}
	return -1
}

// clampBytes 将 b 截断到 maxBytes（<=0 不截断）。
func clampBytes(b []byte, maxBytes int) []byte {
	if maxBytes <= 0 || len(b) <= maxBytes {
		return b
	}
	// 保留尾部提示，便于定位被截断
	const suffix = "\n[truncated]\n"
	if maxBytes <= len(suffix) {
		return append([]byte(nil), b[:maxBytes]...)
	}
	out := make([]byte, 0, maxBytes)
	out = append(out, b[:maxBytes-len(suffix)]...)
	out = append(out, suffix...)
	return out
}

// trySend 非阻塞投递，避免生产者在消费者卡住时挂死。
// 如需严格不丢日志，可去掉 default 并调整缓冲策略。
func trySend(ch chan<- ExecRes, v ExecRes) {
	select {
	case ch <- v:
	default:
	}
}

// prefixStream 给输出加简单前缀，区分 stdout/stderr。
func prefixStream(stream string, line []byte) []byte {
	s := strings.TrimSpace(stream)
	if s == "" {
		return line
	}
	p := []byte(s + ": ")
	out := make([]byte, 0, len(p)+len(line))
	out = append(out, p...)
	out = append(out, line...)
	return out
}
