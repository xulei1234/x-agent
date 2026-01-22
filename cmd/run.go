package cmd

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/xulei1234/x-agent/banner"
	"github.com/xulei1234/x-agent/module/server"
	"os"
	"os/signal"
)

func init() {
	_, _ = fmt.Fprintln(os.Stdout, banner.Banner)
}

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "運行 x-agent 進程",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := run(cmd); err != nil {
				cmd.PrintErrf("run failed: %v\n", err)
				return err
			}
			return nil
		},
	}
}

func run(cmd *cobra.Command) error {
	if err := server.Check(); err != nil {
		cmd.PrintErrf("server check failed: %v\n", err)
		return fmt.Errorf("server check: %w", err)
	}

	ctx, stop := notifyContext(context.Background())
	defer stop()

	if err := server.SetUp(); err != nil {
		return err
	}

	if err := server.Run(ctx); err != nil {
		cmd.PrintErrf("server run failed: %v\n", err)
		return fmt.Errorf("server run: %w", err)
	}

	return nil
}

// 只監聽 os.Interrupt。
func notifyContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			cancel()
			return
		}
	}()

	return ctx, func() {
		signal.Stop(ch)
		cancel()
	}
}
