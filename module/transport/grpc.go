package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/xulei1234/x-agent/module/common"
	"github.com/xulei1234/x-agent/module/cred"
	"github.com/xulei1234/x-proto/xps"
	"go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"io/ioutil"
	"sync/atomic"
	"time"
)

type logger struct {
}

// Info
func (logger) Info(args ...interface{})                 { logrus.Info(args...) }
func (logger) Infoln(args ...interface{})               { logrus.Infoln(args...) }
func (logger) Infof(format string, args ...interface{}) { logrus.Infof(format, args...) }

// Warning \= Warn
func (logger) Warning(args ...interface{})                 { logrus.Warn(args...) }
func (logger) Warningln(args ...interface{})               { logrus.Warnln(args...) }
func (logger) Warningf(format string, args ...interface{}) { logrus.Warnf(format, args...) }

// Error
func (logger) Error(args ...interface{})                 { logrus.Error(args...) }
func (logger) Errorln(args ...interface{})               { logrus.Errorln(args...) }
func (logger) Errorf(format string, args ...interface{}) { logrus.Errorf(format, args...) }

// Fatal: avoid os\.Exit\(\) from deep dependency; downgrade to Error.
func (logger) Fatal(args ...interface{})                 { logrus.Error(args...) }
func (logger) Fatalln(args ...interface{})               { logrus.Errorln(args...) }
func (logger) Fatalf(format string, args ...interface{}) { logrus.Errorf(format, args...) }

// V controls verbose logging.
// Map etcd verbosity to current logrus level so it does not always spam.
func (logger) V(l int) bool {
	// A simple mapping:
	// l <= 0: always
	// l <= 1: require Info or more verbose
	// l <= 2: require Debug or more verbose
	// else: require Trace
	level := logrus.GetLevel()
	switch {
	case l <= 0:
		return true
	case l <= 1:
		return level >= logrus.InfoLevel
	case l <= 2:
		return level >= logrus.DebugLevel
	default:
		return level >= logrus.TraceLevel
	}
}

// 保存上一次连接的 target，用于检测变更并触发 AddressChangeBuffer
var lastConnTarget atomic.Value // stores string

func notifyAddressChange() {
	// 非阻塞通知：避免在网络/重连路径上被 channel 堵死
	select {
	case common.AddressChangeBuffer <- struct{}{}:
	default:
	}
}

func recordConnTargetAndNotifyIfChanged(target string) {
	if target == "" {
		return
	}
	prev, _ := lastConnTarget.Load().(string)
	if prev != "" && prev != target {
		notifyAddressChange()
	}
	lastConnTarget.Store(target)
}

func (g *GrpcMgr) ConnectToChannel() {
	clientv3.SetLogger(&logger{})

	tlsCred, err := credentials.NewClientTLSFromFile(
		viper.GetString("TlsConf.Certfile"),
		viper.GetString("TlsConf.SrvName"),
	)

	if err != nil {
		logrus.Fatalf("ConnectToChannel: fail to NewClientTLSFromFile (%v) ", err.Error())
	}
	logrus.Infof("ConnectToChannel: success to NewClientTLSFromFile (%v)", tlsCred.Info())

	endpoint := viper.GetStringSlice("Channel")
	common.Shuffle(endpoint)

	b, err := ioutil.ReadFile(viper.GetString("TlsConf.Certfile"))
	if err != nil {
		logrus.Fatalf("ConnectToChannel: failed to read  TlsConf.Certfile")
	}
	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM(b) {
		logrus.Fatalf("ConnectToChannel: failed to append certificates")
	}

	g.client3, err = clientv3.New(clientv3.Config{
		Endpoints:            endpoint,
		TLS:                  &tls.Config{ServerName: viper.GetString("TlsConf.SrvName"), RootCAs: cp},
		DialKeepAliveTime:    time.Second * 2,
		DialKeepAliveTimeout: time.Second * 1,
		DialTimeout:          viper.GetDuration("Timeout.Connect"),
		DialOptions: []grpc.DialOption{
			grpc.WithTransportCredentials(tlsCred),
			grpc.WithBlock(),
			grpc.WithPerRPCCredentials(&cred.Credentials{common.GetDeviceUUID()}),
		},
	})
	if err != nil {
		logrus.Fatalf("ConnectToChannel: fail to new client v3(%s)", err.Error())
	}

	conn := g.client3.ActiveConnection()
	if conn != nil {
		target := conn.Target()
		logrus.WithField("channel", target).Info("ConnectToChannel: success to new client v3")
		// 记录并在 target 变化时触发 AddressChangeBuffer
		recordConnTargetAndNotifyIfChanged(target)
	}

	g.client = xps.NewXServiceClient(g.client3.ActiveConnection())
}

func (g *GrpcMgr) GetCommandStreamClient(ctx context.Context, body *xps.Empty) (xps.XService_CommandClient, error) {
	return g.client.Command(ctx, body)
}
