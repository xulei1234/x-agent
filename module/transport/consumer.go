package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/xulei1234/x-agent/module/common"
	proto "github.com/xulei1234/x-proto"
	"github.com/xulei1234/x-proto/xps"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func (g *GrpcMgr) ConsumerCmd(cr *xps.CmdReply) {
	if cr == nil || cr.GetCmd() == nil {
		logrus.Warn("ConsumerCmds: nil command")
		return
	}

	tasklog := logrus.WithField("task_id", cr.Id)

	// 解析Extra参数
	cmdExtra := proto.CmdExtra{}
	if extra := cr.GetCmd().GetExtra(); len(extra) > 0 {
		if err := json.Unmarshal(extra, &cmdExtra); err != nil {
			tasklog.WithError(err).Warn("ConsumerCmds: unmarshal extra, use default settings")
		}
	}

	// timeout：优先 Extra.Timeout，失败则使用配置兜底
	cmdTimeout := parseCmdTimeout(cmdExtra.Timeout, viper.GetDuration("Timeout.CmdRun"), tasklog)

	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, cr.GetCmd().GetName(), cr.GetCmd().GetArgs()...)
	cmd.Env = buildCmdEnv(g)
	if dir := cr.GetCmd().GetDir(); dir != "" {
		cmd.Dir = dir
	}
	if envs := cr.GetCmd().GetEnvs(); len(envs) != 0 {
		cmd.Env = append(cmd.Env, envs...)
	}

	// 切换用户（仅当 user 存在）
	applyUser(cmd, cmdExtra.User, tasklog)

	tasklog.WithFields(logrus.Fields{
		"cmd":     cr.GetCmd().GetName(),
		"args":    strings.Join(cr.GetCmd().GetArgs(), " "),
		"dir":     cmd.Dir,
		"timeout": cmdTimeout.String(),
		"user":    cmdExtra.User,
		"code":    cmdExtra.Code,
	}).Infoln("ConsumerCmd: start")

	switch cmdExtra.Code {
	case proto.MCodeLogLine:
		// 异步执行：实时返回输出
		logCh := make(chan common.ExecRes, 50)
		go common.AsyncExec(cmd, logCh)

		var pos int32
		for {
			select {
			case <-ctx.Done():
				// 超时/取消
				tasklog.WithError(ctx.Err()).Warn("ConsumerCmd: ctx done while streaming logs")
				// 让下游知道结束
				g.SendMsgResult(cr.Id, cmdExtra.Code, &xps.Body{Code: -1, Stderr: []byte(ctx.Err().Error())}, xps.Status_FAIL)
				return
			case r, ok := <-logCh:
				if !ok {
					tasklog.Infoln("ConsumerCmd: async exec finished")
					return
				}
				if r.Err != nil {
					tasklog.WithError(r.Err).Warn("ConsumerCmd: async exec error")
					g.SendMsgResult(cr.Id, cmdExtra.Code, &xps.Body{Code: -1, Stderr: []byte(r.Err.Error())}, xps.Status_FAIL)
					return
				}
				if len(r.Buf) == 0 {
					pos++
					continue
				}
				g.SendLocalLog(cr.Id, pos, string(r.Buf), 0)
				pos++
			}
		}

	default:
		// 同步执行：一次性返回结果
		body := common.SyncExec(cmd)
		status := xps.Status_SUCC
		if body.Code != 0 {
			status = xps.Status_FAIL
		}
		g.SendMsgResult(cr.Id, cmdExtra.Code, body, status)
	}
}

func parseCmdTimeout(raw string, fallback time.Duration, l *logrus.Entry) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		l.WithError(err).WithField("timeout_raw", raw).Warn("ConsumerCmd: invalid timeout, fallback to config")
		return fallback
	}
	// 防御：过小会导致任务几乎立刻被取消
	if d < time.Second {
		l.WithField("timeout", d.String()).Warn("ConsumerCmd: timeout too small, clamp to 1s")
		return time.Second
	}
	return d
}

func buildCmdEnv(g *GrpcMgr) []string {
	env := os.Environ()

	// 注入 channel 信息（需要保护 nil）
	if g != nil && g.client3 != nil {
		if conn := g.client3.ActiveConnection(); conn != nil {
			if host, port, ok := splitHostPortLoose(conn.Target()); ok {
				env = append(env, "SERVER_CHANNEL_HOST="+host, "SERVER_CHANNEL_PORT="+port)
			}
		}
	}

	// 注入 RuntimeEnv（避免每条都打 debug）
	for k, v := range viper.GetStringMapString("RuntimeEnv") {
		if kk := strings.TrimSpace(k); kk != "" {
			env = append(env, fmt.Sprintf("%s=%s", strings.ToUpper(kk), v))
		}
	}
	return env
}

// splitHostPortLoose：兼容 target 可能是 `dns:///host:port` 或 `host:port`
func splitHostPortLoose(target string) (host, port string, ok bool) {
	t := strings.TrimSpace(target)
	if t == "" {
		return "", "", false
	}
	// 简单去掉常见 scheme 前缀
	t = strings.TrimPrefix(t, "dns:///")
	t = strings.TrimPrefix(t, "dns:///")
	t = strings.TrimPrefix(t, "passthrough:///")

	// 只取最后一段（防止包含多段路径）
	if i := strings.LastIndex(t, "/"); i >= 0 {
		t = t[i+1:]
	}

	parts := strings.Split(t, ":")
	if len(parts) < 2 {
		return "", "", false
	}
	host = strings.Join(parts[:len(parts)-1], ":")
	port = parts[len(parts)-1]
	if host == "" || port == "" {
		return "", "", false
	}
	return host, port, true
}

func applyUser(cmd *exec.Cmd, username string, l *logrus.Entry) {
	username = strings.TrimSpace(username)
	if username == "" {
		return
	}

	u, err := user.Lookup(username)
	if err != nil {
		l.WithError(err).WithField("user", username).Warn("ConsumerCmd: user lookup failed, run as current user")
		return
	}

	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		l.WithError(err).WithField("uid", u.Uid).Warn("ConsumerCmd: invalid uid, skip setuid")
		return
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		l.WithError(err).WithField("gid", u.Gid).Warn("ConsumerCmd: invalid gid, skip setgid")
		return
	}

	// 只在 SysProcAttr 可用时设置；保持原行为（Linux 为主）
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		},
	}
}
