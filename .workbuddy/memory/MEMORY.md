# frp2 项目长期备忘

- **本仓库是 frp 的精简分支：只支持 `type=tcp` 代理**（2026-10-19 起）。udp/http/https/tcpmux/stcp/xtcp/sudp、visitor、vhost、tcpmux、nathole、UDPPacket/wire UDP codec 均已删除，不要再引用这些类型或为它们补功能。
- **传输协议也已裁剪：frpc↔frps 仅支持 tcp / websocket / wss**；quic/kcp、Prometheus /metrics、OIDC 认证、sshTunnelGateway、virtualNet/featureGates 全部删除。auth 只剩 token（含 tokenSource）。
- 保留的通用能力：客户端插件（socks5/http_proxy/static_file/unix_domain_socket/https2http/http2https/http2http/https2https/tls2raw）、健康检查（tcp/http 探测本地服务）、负载均衡 group、带宽限制、加密压缩、proxy protocol、webServer/dashboard、store 配置源。
- 体积参考（-s -w + 嵌入 dashboard）：frps ≈ 12.8MB，frpc ≈ 14.2MB；kcp-go 经 fatedier/golib/net 间接引入、k8s.io/apimachinery 仅用于 annotation 校验，是剩余可挤的水分。
- 配置 strict 模式下旧的 vhost*/tcpmux*/subDomainHost/custom404Page/udpPacketSize/nathole*/visitors 字段会直接报 unknown field。
- 工作区为 CRLF（`core.autocrlf=true`）；e2e 需先 `make build` 出 `bin/frps|frpc`。
