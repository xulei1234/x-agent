package common

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"
	utilnet "github.com/shirou/gopsutil/net"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	proto "github.com/xulei1234/x-proto"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

func GetDeviceUUID() string {
	// 配置优先
	confUUID := strings.TrimSpace(viper.GetString("UUID"))
	if confUUID != "" {
		return strings.ToUpper(confUUID)
	}

	// 1) 优先尝试 dmidecode（常见需要 root）
	if uuid, err := readUUIDFromDMI(); err == nil && uuid != "" {
		logrus.WithField("uuid", uuid).Info("GetDeviceUUID: use dmidecode uuid")
		return strings.ToUpper(uuid)
	} else if err != nil {
		logrus.WithError(err).Warn("GetDeviceUUID: dmidecode failed, fallback to host id")
	}

	// 2) 回退 host id
	if hi, err := host.Info(); err == nil && strings.TrimSpace(hi.HostID) != "" {
		confUUID = strings.TrimSpace(hi.HostID)
		logrus.WithField("uuid", confUUID).Info("GetDeviceUUID: use host id")
		return strings.ToUpper(confUUID)
	}

	// 3) 最后兜底：hostname
	if hn, err := os.Hostname(); err == nil && strings.TrimSpace(hn) != "" {
		confUUID = strings.TrimSpace(hn)
		logrus.WithField("uuid", confUUID).Warn("GetDeviceUUID: fallback to hostname as uuid")
		return strings.ToUpper(confUUID)
	}

	logrus.Warn("GetDeviceUUID: unable to determine uuid, return empty")
	return ""
}

func readUUIDFromDMI() (string, error) {
	// 超时与可复现性更好
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 不使用 `sh -c`，避免管道/注入问题
	cmd := exec.CommandContext(ctx, "dmidecode", "-t", "1")
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err != nil {
		// timeout 也在这里体现
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", ctx.Err()
		}
		return "", err
	}

	uuid := parseUUIDFromDMIDecode(string(out))
	return uuid, nil
}

func parseUUIDFromDMIDecode(s string) string {
	// dmidecode 输出通常包含：`UUID: XXXXX`
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(line), "UUID:") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(line, "UUID:"))
		// 过滤无效值
		up := strings.ToUpper(v)
		if up == "" || up == "NOT SETTABLE" || up == "NOT PRESENT" || strings.HasPrefix(up, "00000000-0000-0000-0000-000000000000") {
			return ""
		}
		return v
	}
	return ""
}

func GetDeviceHostname() string {
	configHostName := strings.TrimSpace(viper.GetString("HostName"))
	if configHostName != "" {
		return configHostName
	}
	realHostName, err := os.Hostname()
	if err != nil {
		logrus.WithError(err).Error("GetDeviceHostname: failed")
		return ""
	}
	return realHostName
}

func GetDeviceZone() string {
	zone := strings.TrimSpace(viper.GetString("IDC.Zone"))
	if zone == "" {
		zone = "ZONE-DEFAULT"
		logrus.WithField("zone", zone).Trace("GetDeviceZone: use default zone")
	}
	return zone
}

func GetConfigIP() string {
	configIP := strings.TrimSpace(viper.GetString("IP"))
	if configIP != "" {
		return configIP
	}

	ips := GetDeviceIPList()
	if len(ips) == 0 {
		logrus.Warn("GetConfigIP: no usable ip found")
		return ""
	}
	logrus.WithField("ip", ips[0]).Trace("GetConfigIP: choose first usable ip")
	return ips[0]
}

func GetDeviceIPList() []string {
	var iplist []string

	ifaces, err := net.Interfaces()
	if err != nil {
		logrus.WithError(err).Warn("GetDeviceIPList: net.Interfaces failed")
		return iplist
	}

	for _, iface := range ifaces {
		// 跳过 down/loopback
		if (iface.Flags&net.FlagUp) == 0 || (iface.Flags&net.FlagLoopback) != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP == nil {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}
			// 排除 127.0.0.0/8
			if ip4.IsLoopback() {
				continue
			}
			iplist = append(iplist, ip4.String())
		}
	}

	return iplist
}

func GetDeviceOsInfo() *proto.OSInfo {
	osinfo := &proto.OSInfo{}

	// Host
	if hi, err := host.Info(); err == nil {
		// 保持原结构：依旧使用 json 映射到 proto 的 HostInfo 字段（避免直接依赖字段结构）
		if b, e := json.Marshal(hi); e == nil {
			_ = json.Unmarshal(b, &osinfo.HostInfo)
		}
	} else {
		logrus.WithError(err).Warn("GetDeviceOsInfo: host.Info failed")
	}

	// Interfaces
	if ifs, err := utilnet.Interfaces(); err == nil {
		if b, e := json.Marshal(ifs); e == nil {
			_ = json.Unmarshal(b, &osinfo.InterfaceInfo)
		}
	} else {
		logrus.WithError(err).Warn("GetDeviceOsInfo: net.Interfaces failed")
	}

	// Mem
	if vm, err := mem.VirtualMemory(); err == nil {
		if b, e := json.Marshal(vm); e == nil {
			_ = json.Unmarshal(b, &osinfo.MemInfo)
		}
	} else {
		logrus.WithError(err).Warn("GetDeviceOsInfo: mem.VirtualMemory failed")
	}

	// CPU
	if ci, err := cpu.Info(); err == nil {
		if b, e := json.Marshal(ci); e == nil {
			_ = json.Unmarshal(b, &osinfo.CPUInfo)
		}
	} else {
		logrus.WithError(err).Warn("GetDeviceOsInfo: cpu.Info failed")
	}

	return osinfo
}
