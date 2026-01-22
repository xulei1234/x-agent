package cmd

import (
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/xulei1234/x-agent/module/config"
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
	"strings"
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
	return newRootCmd().Execute()
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "x-agent",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "version" {
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
	// 1) 注入 viper 預設值 + 環境變數（先預設，後覆寫）
	config.SetupDefaultViper()
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 2) 配置檔：可選（不存在不視為錯）
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		if err := viper.ReadInConfig(); err != nil {
			// 若檔案不存在就忽略，讓預設值生效；其它錯誤才返回
			var configFileNotFoundError viper.ConfigFileNotFoundError
			if !errors.As(err, &configFileNotFoundError) {
				return fmt.Errorf("read config: %w", err)
			}
		}
	}

	// 3) 日誌：同時輸出到終端 \+ 文件（使用 viper 取到的值：配置檔/環境/預設）
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})

	lvl, err := logrus.ParseLevel(loglevel)
	if err != nil {
		return fmt.Errorf("invalid loglevel %q: %w", loglevel, err)
	}

	logrus.SetLevel(lvl)
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
	logrus.WithField("config", viper.ConfigFileUsed()).Infof("Using config file")
	return nil
}
