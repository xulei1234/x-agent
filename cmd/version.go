package cmd

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xulei1234/x-agent/module/common"
)

func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "打印版本資訊",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), FormatVersion())
			return err
		},
	}
}

func FormatVersion() string {
	// 只組裝字串，無任何副作用，方便測試與重用
	lines := []string{
		fmt.Sprintf("app: %s", "x-agent"),
		fmt.Sprintf("version: %s", safe(common.Version)),
		fmt.Sprintf("git_commit: %s", safe(common.GitCommit)),
		fmt.Sprintf("go_version: %s", pick(common.GoVersion, runtime.Version())),
		fmt.Sprintf("build_time: %s", safe(common.BuildTime)),
		fmt.Sprintf("build_host: %s", safe(common.BuildHost)),
	}
	return strings.Join(lines, "\n")
}

func safe(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unknown"
	}
	return s
}

func pick(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return safe(fallback)
}
