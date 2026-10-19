# frp2 精简与体积裁剪 — 变更摘要

更新时间：2026-10-19

## 体积（-s -w，嵌入 dashboard）

```
frps（最终）                12.78 MB
frpc（最终）                14.23 MB
frps（最初全功能）             20.10 MB
frpc（最初全功能）             16.27 MB
```

## 第一轮：代理类型只保留 type=tcp
- 删除 udp/http/https/tcpmux/stcp/xtcp/sudp 代理、visitor 子系统、vhost、tcpmux、nathole、UDPPacket/wire UDP codec

## 第二轮：按选择裁掉四大块功能
- quic + kcp 传输：frpc↔frps 仅 tcp / websocket / wss
- Prometheus 监控（/metrics）：dashboard 内存统计仍在
- OIDC 认证：仅剩 token（含 tokenSource）
- SSH 隧道网关 + 虚拟网卡：pkg/ssh、pkg/virtual、pkg/vnet、virtual_net 插件、featureGates 全部删除

## 依赖清理效果
- 二进制中已无 quic-go、prometheus、go-jose、oauth2、water/wireguard
- 仍含 kcp-go（经 fatedier/golib/net 的 dial 间接引入）、k8s.io/apimachinery（仅 annotation 名校验）

## 验证
```
go build ./... / go vet ./...            OK
go test ./pkg/... ./client/... ./server/... ./cmd/...   全部通过
TCP 代理端到端冒烟（frps+frpc+echo）      通过
e2e：96/134 通过（其余为 Windows 环境问题）
```

注：e2e 在本机剩余失败由两类环境因素导致：
1) 测试模板把 Windows 反斜杠路径写进无扩展名配置文件，TOML 解析失败后按 YAML 读取导致误读
2) 本机 HTTP_PROXY 环境变量让 frpc 走代理连 127.0.0.1（502）
均为既有问题，与本次改动无关（曾在 e2e 中发现并修复一处 default 分支误删的真实 bug）。
