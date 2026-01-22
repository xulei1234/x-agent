/**
 * @Author: wanrui
 * @Description: 上报客户端基础信息
 * @File:  report
 * @Version: 1.0.0
 * @Date: 2020/5/9 1:45 下午
 */

package transport

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/xulei1234/x-agent/module/common"
	"github.com/xulei1234/x-proto/xps"
	"time"
)

var (
	agentmd5  = make([]byte, 16)
	osInfomd5 = make([]byte, 16)
)

// bool indicate whether if report forcely
func (g *GrpcMgr) SendAgentInfo(force bool) {
	in := xps.RegRequest{
		Hostname: common.GetDeviceHostname(),
		Ip:       common.GetConfigIP(),
		Version:  common.Version,
		Idc:      common.GetDeviceZone(),
	}
	sum := md5.Sum([]byte(in.String()))
	hash := sum[:] // to bytes
	if bytes.Compare(hash, agentmd5) == 0 && !force {
		logrus.Traceln("SendAgentInfo: Hostname & Ip & Version & Idc not changed")
		return
	}
	logrus.Warnf("SendAgentInfo: Hostname & Ip & Version & Idc changed, should upload")
	timeout := viper.GetDuration("Timeout.Report")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	logrus.Traceln("SendAgentInfo: RegisterAgent timeout = ", timeout)

	if _, err := g.client.RegisterAgent(ctx, &in); err != nil {
		logrus.Error("SendAgentInfo: RegisterAgent failed: ", err.Error())
	} else {
		copy(agentmd5, hash)
		logrus.Infoln("SendAgentInfo：RegisterAgent upload Suc :", in)
	}

}

func (g *GrpcMgr) SendOSInfo() {
	req := common.GetDeviceOsInfo()
	body, marshErr := json.Marshal(req)
	if marshErr != nil {
		logrus.Error(marshErr.Error())
		return
	}

	msg := &xps.MsgRequest{
		Id: "",
		//Type: proto.MsgTypeSysInfo,
		Body: &xps.Body{
			Code:   0,
			Stdout: body,
		},
	}
	sum := md5.Sum([]byte(req.HostInfo.Hostname))
	hash := sum[:] // to bytes
	if bytes.Compare(hash, osInfomd5) == 0 {
		logrus.Traceln("SendOSInfo: common.GetDeviceOsInfo()  has not changed ")
		return
	}
	logrus.Warnf("SendOSInfo: common.GetDeviceOsInfo()  has changed, will upload with client.Msg ")
	timeout := viper.GetDuration("Timeout.Report")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	logrus.Traceln("SendOSInfo: g.client.Msg timeout = ", timeout)
	defer cancel()
	if _, err := g.client.Msg(ctx, msg); err != nil {
		logrus.Errorln("SendOSInfo: g.client.Msg failed = ", err.Error())
	} else {
		copy(osInfomd5, hash)
		logrus.Infoln("SendOSInfo: g.client.Msg success ", req)
	}
}

func (g *GrpcMgr) SendMsgResult(id string, code uint32, body *xps.Body, status xps.Status) {
	timeout := viper.GetDuration("Timeout.Report")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	logrus.Traceln("SendMsgResult: g.client.Msg timeout = ", timeout)
	defer cancel()
	_, err := g.client.Msg(ctx, &xps.MsgRequest{
		Id: id,
		Dt: code,
		//Status: status,
		Body: body,
	})
	if err != nil {
		logrus.WithField("task_id", id).Error("SendMsgResult: g.client.Msg failed [", code, "] 错误为: ", err.Error())
	} else {
		logrus.WithField("task_id", id).Infoln("SendMsgResult: g.client.Msg success ", code, status, body)
	}

}

func (g *GrpcMgr) SendLocalLog(id string, pos int32, out string, pc int32) {
	timeout := viper.GetDuration("Timeout.Report")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	logrus.Traceln("SendLocalLog： timeout = ", timeout)
	defer cancel()
	_, err := g.client.Log(ctx, &xps.LogRequest{
		Id: id,
		// Pc: pc, // deprecated
		Line: &xps.Line{
			Pc:   pc,
			Pos:  pos,
			Out:  string(out),
			Time: time.Now().Unix(),
		},
	})

	if err != nil {
		logrus.WithField("task_id", id).Errorln("SendLocalLog： g.client.Log failed = ", err.Error())
	} else {
		logrus.WithField("task_id", id).Traceln("SendLocalLog： g.client.Log success ", out)
	}

}

func (g *GrpcMgr) SendHeartBeat() {
	in := &xps.HBSRequest{Ts: time.Now().Unix()}
	timeout := viper.GetDuration("Timeout.HeartBeat")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	logrus.Traceln("SendHeartBeat： timeout = ", timeout)
	defer cancel()
	_, err := g.client.ReportHBS(ctx, in)
	if err != nil {
		logrus.Error("SendHeartBeat： g.client.ReportHBS failed = ", err.Error())
	} else {
		logrus.Traceln("SendHeartBeat： g.client.ReportHBS success")
	}

}
