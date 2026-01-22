# x-agent

一个基于 gRPC 的主机 Agent：负责连接 Channel（etcd v3）、接收并执行命令、实时回传日志与结果，并周期性上报主机与 Agent 信息。

## 功能概览

- 连接 Channel（etcd v3），支持 TLS 证书校验与重连
- 执行命令
    - 同步执行：一次性返回结果（stdout/stderr/退出码）
    - 异步执行：按行实时回传 stdout/stderr 日志
    - 支持超时解析与防御（超时最小 1s）
- 注入运行环境变量
    - 继承系统环境
    - 从配置 `RuntimeEnv` 注入自定义变量
    - 自动注入 `SERVER_CHANNEL_HOST` / `SERVER_CHANNEL_PORT`（来自活动连接 Target）
- 周期上报
    - Agent 信息（Hostname/IP/Version/Zone）
    - OS 信息（host/cpu/mem/net）
    - 心跳

## 目录结构（简要）

- `module/transport/`
    - `grpc.go`：连接 Channel、创建 gRPC client、地址变更通知
    - `report.go`：上报 Agent/OS/心跳、发送任务结果与实时日志
- `module/common/`
    - `cmd.go`：命令同步/异步执行、输出大小限制、退出码提取
    - `linux.go`：采集 UUID/IP/Hostname/OS 信息（Linux）
    - `init.go`：全局缓冲（如 AddressChangeBuffer）
- `module/config/init.go`：Viper 默认配置
- `configs/x-agent.json`：示例配置
- `deployments/`：init\.d / systemd 部署脚本
- `scripts/`：安装脚本与辅助脚本

## 构建与运行（Windows）

> Makefile 已做 Windows 兼容（输出到 `bin/`）。

- 构建：
    - `make build`
- 运行：
    - `make run`
- 测试：
    - `make test`

也可以直接使用 Go：
- `go build -o bin/x-agent ./main.go`
- `bin/x-agent run`

## 配置

项目使用 Viper 读取配置，默认值在 `module/config/init.go`；示例文件见 `configs/x-agent.json`。

常用配置项（节选）：

- `Channel`：Channel 列表（形如 `host:port`）
- `TlsConf.Certfile`：TLS 证书文件路径（PEM）
- `TlsConf.SrvName`：TLS ServerName
- `Timeout.*`
    - `Timeout.CmdRun`：命令执行超时
    - `Timeout.Report`：上报 RPC 超时
    - `Timeout.Connect`：连接超时
    - `Timeout.HeartBeat`：心跳超时
- `IntervalTick.*`：定时上报周期
- `RuntimeEnv`：注入到命令执行环境的变量（map）
- `LogFile.*`：日志文件配置

示例（精简）：

```json
{
  "Channel": ["127.0.0.1:5050"],
  "TlsConf": {
    "Certfile": "./cert/server.pem",
    "SrvName": "cruiser-channel-grpc"
  },
  "Timeout": {
    "CmdRun": "120s",
    "Report": "2s",
    "Connect": "4s",
    "HeartBeat": "2s"
  },
  "RuntimeEnv": {
    "PATH": ":/opt/x-agent/libexec:/bin:/usr/bin"
  }
}
