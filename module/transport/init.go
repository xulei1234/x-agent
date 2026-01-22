package transport

import (
	"context"
	"go.etcd.io/etcd/client/v3"
	"sync"
	"sync/atomic"

	"github.com/sirupsen/logrus"
	"github.com/xulei1234/x-proto/xps"
)

// WorkerPool
type WorkerPool struct {
	tasks    chan *xps.CmdReply //任务队列
	poolSize int                //启动goroutine的数目
}

type GrpcMgr struct {
	client       xps.XServiceClient
	client3      *clientv3.Client
	cmdtask      *WorkerPool
	streamCancel context.CancelFunc

	// closed: 标记任务队列是否已关闭（避免 close 后继续写入导致 panic）
	closed atomic.Bool
	// closeOnce: 确保 Close() 幂等
	closeOnce sync.Once
}

var gMgr *GrpcMgr

func init() {
	gMgr = &GrpcMgr{
		cmdtask: &WorkerPool{
			tasks:    make(chan *xps.CmdReply, 200),
			poolSize: 10,
		},
	}

}

func SetUp() {
	gMgr.ConnectToChannel()
}

func Run() error {
	go gMgr.TaskReportAgentInfo()
	go gMgr.TaskReportHBS()
	go gMgr.TaskReportOSInfo()
	go gMgr.TaskConsumerCmds()
	go gMgr.TaskPullCommands()
	go gMgr.WatchGrpcAddressUpdate()
	return nil
}

// isClosed 用于在投递任务前快速判断，避免 panic
func (g *GrpcMgr) isClosed() bool {
	return g.closed.Load()
}

func Close() {
	logrus.Info("just say good bye for grpc manager.")
	gMgr.closeOnce.Do(func() {
		// 先标记 closed，减少发送方的竞态窗口
		gMgr.closed.Store(true)

		if gMgr.cmdtask != nil && gMgr.cmdtask.tasks != nil {
			close(gMgr.cmdtask.tasks)
		}
		if gMgr.client3 != nil {
			_ = gMgr.client3.Close()
		}
	})
}
