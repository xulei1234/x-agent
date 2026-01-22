package cmd

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
	"sync"

	"github.com/spf13/viper"
)

var (
	cfgFile  string
	loglevel string

	initOnce sync.Once
	initErr  error
)

func Execute() error {
	err := newRootCmd().Execute()
	if err != nil {
		// 确保命令错误至少能在终端看到（即使日志初始化失败）
		_, _ = fmt.Fprintf(os.Stderr, "command failed: %v\n", err)
	}
	return err
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "x-agent",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// version 不应依赖配置文件存在
			if cmd.Name() == "version" || cmd.CommandPath() == "x-agent version" {
				return nil
			}
			initOnce.Do(func() { initErr = initConfigAndLogging() })
			return initErr
		},
	}

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "./config/x-agent.json", "config file")
	rootCmd.PersistentFlags().StringVarP(&loglevel, "loglevel", "l", "info", "panic fatal error warn info debug trace")

	rootCmd.AddCommand(newRunCmd())
	rootCmd.AddCommand(NewVersionCmd())

	return rootCmd
}

// initConfig reads in config file and ENV variables if set.
func initConfigAndLogging() error {
	// 初始化日志
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})

	lvl, err := logrus.ParseLevel(loglevel)
	if err != nil {
		return fmt.Errorf("invalid loglevel %q: %w", loglevel, err)
	}
	logrus.SetLevel(lvl)

	// 配置文件
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	}

	viper.AutomaticEnv() // read in environment variables that match

	// 讀取配置
	if err = viper.ReadInConfig(); err != nil {
		return fmt.Errorf("read config: error %w", err)
	}

	// 日誌輸出：若沒配置路徑，兜底到 stdout，避免寫入空文件名
	logPath := viper.GetString("LogFile.Path")
	if logPath == "" {
		logrus.SetOutput(os.Stdout)
		logrus.WithField("level", lvl).Infoln("log path is empty, fallback to stdout")
	} else {
		logrus.SetOutput(&lumberjack.Logger{
			// 日志输出文件路径
			Filename: logPath,
			// 日志文件最大 size, 单位是 MB
			MaxSize: viper.GetInt("LogFile.MaxSize"),
			// 最大过期日志保留的个数
			MaxBackups: viper.GetInt("LogFile.MaxBackups"),
			// 保留过期文件的最大时间间隔,单位是天
			MaxAge: viper.GetInt("LogFile.MaxAge"),
			// 是否需要压缩滚动日志, 使用的 gzip 压缩
			Compress: true,
		})
	}
	// 設置日誌級別
	logrus.WithField("level", lvl).Infoln("初始化设置当前日志输出级别")
	logrus.WithField("config", viper.ConfigFileUsed()).Infof("Using config file")
	logrus.WithField("log", viper.GetString("LogFile.Path")).Info("Using log file")
	return nil
}
