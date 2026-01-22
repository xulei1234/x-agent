package transport

import (
	"context"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/xulei1234/x-agent/module/common"
	proto "github.com/xulei1234/x-proto"
	"github.com/xulei1234/x-proto/xps"
	"io"
	"sync/atomic"
	"time"
)

func (g *GrpcMgr) TaskReportHBS() {
	logrus.Infoln("TaskReportHBS: start")

	ticker := time.NewTicker(viper.GetDuration("IntervalTick.HeartBeat"))
	defer ticker.Stop()

	for {
		g.SendHeartBeat()
		<-ticker.C
	}
}

// WatchGrpcAddressUpdate  业务层关注连接地址的变更，进行取消当前连接，并重新注册
func (g *GrpcMgr) WatchGrpcAddressUpdate() {
	logrus.Infoln("WatchGrpcAddressUpdate: start")

	for range common.AddressChangeBuffer {
		g.SendAgentInfo(true)
		if g.streamCancel != nil {
			g.streamCancel()
		}
	}

	logrus.Warn("WatchGrpcAddressUpdate: AddressChangeBuffer closed, exit")
}

func (g *GrpcMgr) TaskReportAgentInfo() {
	logrus.Infoln("TaskReportAgentInfo: start")

	ticker := time.NewTicker(viper.GetDuration("IntervalTick.ReportAgent"))
	defer ticker.Stop()

	for {
		g.SendAgentInfo(false)
		<-ticker.C
	}
}

func (g *GrpcMgr) TaskReportOSInfo() {
	logrus.Infoln("TaskReportOSInfo: start")

	ticker := time.NewTicker(viper.GetDuration("IntervalTick.ReportOS"))
	defer ticker.Stop()

	for {
		g.SendOSInfo()
		<-ticker.C
	}
}

func (g *GrpcMgr) TaskConsumerCmds() {
	logrus.Infoln("TaskConsumerCmds: start")

	for i := 0; i < g.cmdtask.poolSize; i++ {
		workerId := i
		go func() {
			for t := range g.cmdtask.tasks {
				if t == nil {
					continue
				}
				logrus.WithFields(logrus.Fields{
					"worker":  workerId,
					"task_id": t.Id,
				}).Debug("TaskConsumerCmds: received task")
				g.ConsumerCmd(t)
			}
			logrus.WithField("worker", workerId).Warn("TaskConsumerCmds: tasks channel closed, worker exit")
		}()
	}
}

func (g *GrpcMgr) TaskPullCommands() {
	// retry to establish stream forever
	logrus.Infoln("TaskPullCommands: start")
	var attempt int64
	for {
		// 每次重建 stream 都用新的 ctx/cancel
		ctx, cancel := context.WithCancel(context.Background())
		g.streamCancel = cancel

		stream, err := g.GetCommandStreamClient(ctx, new(xps.Empty))
		if err != nil {
			cancel()
			d := backoffDuration(atomic.AddInt64(&attempt, 1))
			logrus.WithError(err).Warnf("TaskPullCommands: open stream failed, backoff=%s", d)
			time.Sleep(d)
			continue
		}
		// stream 已建立，重置重试计数
		atomic.StoreInt64(&attempt, 0)

		g.SendAgentInfo(true)
		logrus.Info("TaskPullCommands: listen on stream to receive commands")

		for {
			cr, err := stream.Recv()
			if err == nil {
				// 上报确认消息
				go g.SendMsgResult(cr.Id, proto.MCodeConfirm, &xps.Body{}, xps.Status_SUCC)

				if g.isClosed() {
					logrus.WithField("task_id", cr.Id).Warn("TaskPullCommands: tasks channel closed, drop command")
					continue
				}

				select {
				case g.cmdtask.tasks <- cr:
				default:
					// 队列满时避免阻塞 stream 读取；按需可改为阻塞/丢弃策略
					logrus.WithField("task_id", cr.Id).Warn("TaskPullCommands: tasks queue full, drop command")
				}
				continue
			}

			if err == io.EOF {
				logrus.Warn("TaskPullCommands: stream recv io.EOF, will reconnect")
			} else {
				logrus.WithError(err).Warn("TaskPullCommands: stream recv error, will reconnect")
			}

			// 断开本轮 stream
			cancel()
			break
		}
	}
}

func backoffDuration(attempt int64) time.Duration {
	// 退避：1s 起步，指数增长，封顶 60s
	if attempt < 1 {
		attempt = 1
	}
	d := time.Second << minInt64(attempt-1, 6) // 1,2,4,8,16,32,64（后续封顶）
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	// 轻量扰动（避免羊群效应），不引入 rand 依赖：用 attempt 做确定性扰动
	jitter := time.Duration(attempt%7) * 200 * time.Millisecond
	return d + jitter
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
