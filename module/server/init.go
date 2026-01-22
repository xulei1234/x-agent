package server

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/xulei1234/x-agent/module/config"
	"github.com/xulei1234/x-agent/module/transport"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// Check current user is root
func Check() error {
	// 在 Unix/Linux 下，root 的 euid == 0
	if os.Geteuid() != 0 {
		return fmt.Errorf("root privileges required")
	}
	return nil
}

func SetUp() {
	config.SetupDefaultViper()
	transport.SetUp()
}

func Run(ctx context.Context) error {
	if err := transport.Run(); err != nil {
		return fmt.Errorf("transport run: %w", err)
	}

	var closeOnce sync.Once
	closeTransport := func() {
		closeOnce.Do(func() {
			transport.Close()
		})
	}

	// 注册退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigCh)
		close(sigCh)
	}()

	select {
	case <-ctx.Done():
		closeTransport()
		logrus.Warn("收到上下文取消信号，退出进程。")
		return ctx.Err()
	case sig := <-sigCh:
		closeTransport()
		logrus.WithField("signal", sig.String()).Warn("收到中斷信號，退出進程。")
		return nil
	}
}
