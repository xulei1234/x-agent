package config

import "github.com/spf13/viper"

func SetupDefaultViper() {
	viper.SetDefault("Channel", []string{
		"47.94.106.61:5050",
		"10.1.64.11:5050",
		"10.1.64.12:5050",
		"10.32.214.2:5050",
		"10.32.214.3:5050",
		"10.254.144.94:5050",
		"10.254.144.95:5050",
	})
	viper.SetDefault("Channel.Port", "5050")
	viper.SetDefault("IDC.Zone", "")
	viper.SetDefault("IDC.Region", "")
	viper.SetDefault("TlsConf.Certfile", "")
	viper.SetDefault("TlsConf.SrvName", "")
	viper.SetDefault("IP", "")
	viper.SetDefault("HostName", "")
	viper.SetDefault("UUID", "")
	viper.SetDefault("LogFile.Path", "./x-agent.log")
	viper.SetDefault("LogFile.MaxSize", "5000")
	viper.SetDefault("LogFile.MaxBackups", "10")
	viper.SetDefault("LogFile.MaxAge", "30")
	viper.SetDefault("LogOnceCount", 50000)
	viper.SetDefault("IntervalTick.HeartBeat", "10s")
	viper.SetDefault("IntervalTick.ReportOS", "20s")
	viper.SetDefault("IntervalTick.ReportAgent", "20s")
	viper.SetDefault("Timeout.CmdRun", "120s")
	viper.SetDefault("Timeout.HearBeat", "60s")
	viper.SetDefault("Timeout.Report", "2s")
	viper.SetDefault("Timeout.Connect", "4s")
	viper.SetDefault("RuntimeEnv", map[string]string{
		"PATH": ":/opt/x-agent/libexec:/bin:/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin",
	})
}
