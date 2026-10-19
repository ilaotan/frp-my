# HTTP 代理访问控制：鉴权头替换、登录页与 IP 白名单

> 本文档描述本仓库对 `type = "http"` 代理新增的三个访问控制能力。它们都只在
> `type = "http"` 上生效，`https`、`tcpmux` 等其他类型不受影响。

## 1. 解决什么问题

frp 原本用 `httpUser` / `httpPassword` 给 Web 服务加一层 HTTP Basic Auth，但有两个硬伤：

1. **凭据只能放在 `Authorization` 头里**，而一条 HTTP 请求只有一个这样的槽位。源站如果自己也用
   `Authorization`（Basic / Bearer 都算），两套凭据只能二选一。
2. **frps 校验完不会摘掉这个头**，会原样转发给源站。于是源站要么读到 frp 的账号密码（可被
   base64 还原），要么因为拿到错误凭据而判 401，浏览器在两层 realm 之间反复弹窗，形成登录死循环。

本仓库给出的解法：

| 能力 | 配置字段 | 解决的问题 |
| --- | --- | --- |
| 换一个鉴权头 | `httpAuthHeader` | `Authorization` 完全留给源站 |
| frps 自带登录页 + Cookie | `httpAuthPagePath` | 浏览器无法发送自定义头，改用表单登录 |
| 来源 IP 白名单 | `allowIPs` | 只允许指定网段访问，不依赖密码 |

## 2. 字段一览

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `httpUser` | string | 空 | 用户名，为空且 `httpPassword` 也为空时不启用鉴权 |
| `httpPassword` | string | 空 | 密码 |
| `httpAuthHeader` | string | 空 | 承载 Basic 凭据的头名。为空时用 `Authorization`（代理模式用 `Proxy-Authorization`） |
| `httpAuthPagePath` | string | `/.auth` | 登录页基路径，实际暴露为 `<base>/login`、`<base>/logout`。仅在 `httpAuthHeader` 非空时可用 |
| `allowIPs` | []string | 空 | 允许访问的来源 IP 或 CIDR，空表示不限制 |

## 3. 完整示例

```toml
# frpc.toml
serverAddr = "x.x.x.x"
serverPort = 7000

[[proxies]]
name = "web01"
type = "http"
localIP = "127.0.0.1"
localPort = 80
customDomains = ["web01.example.com"]

# 1) 鉴权开关与凭据
httpUser = "alice"
httpPassword = "s3cret"

# 2) 改用自定义头，把 Authorization 让给源站
httpAuthHeader = "X-FRP-Auth"

# 3) 登录页位置（浏览器走这条路；可随便改，避免和业务路由冲突）
httpAuthPagePath = "/_o7k2/entry"

# 4) 来源 IP 白名单，支持单 IP 与 CIDR，IPv4/IPv6 均可
allowIPs = ["192.168.1.0/24", "203.0.113.7", "2001:db8::/32"]
```

`frps.toml` 不需要任何改动：

```toml
bindPort = 7000
vhostHTTPPort = 8080
```

## 4. 鉴权头替换：`httpAuthHeader`

### 4.1 读哪个头

| 场景 | `httpAuthHeader` 为空（兼容旧行为） | `httpAuthHeader = "X-FRP-Auth"` |
| --- | --- | --- |
| 浏览器/直接访问 vhost 域名 | `Authorization` | `X-FRP-Auth` |
| 把 frps 当 HTTP 代理用（绝对 URI） | `Proxy-Authorization` | `X-FRP-Auth` |
| CONNECT 隧道 | `Proxy-Authorization` | `X-FRP-Auth` |

配置了自定义头之后，frps **不再读取** `Authorization`，源站可以放心用它做自己的鉴权。

### 4.2 凭据格式

两种写法都接受：

```
X-FRP-Auth: Basic YWxpY2U6czNjcmV0      # 标准写法，推荐
X-FRP-Auth: YWxpY2U6czNjcmV0            # 省略 Basic 前缀，方便手工调用
```

生成方式：

```bash
printf 'alice:s3cret' | base64
```

### 4.3 转发前剥离

校验通过的请求在转发给源站之前，frps 会删掉这个鉴权头，源站和它的日志都不会看到 frp 的密码。

顺序上先剥离再应用 `requestHeaders.set`，所以如果你显式配置了同名头，仍以自己的配置为准：

```toml
requestHeaders.set.X-FRP-Auth = "fixed-value"
```

### 4.4 客户端调用示例

```bash
curl -H 'X-FRP-Auth: Basic YWxpY2U6czNjcmV0' http://web01.example.com/api/health
```

Go：

```go
req.Header.Set("X-FRP-Auth", "Basic "+base64.StdEncoding.EncodeToString([]byte("alice:s3cret")))
```

## 5. 浏览器登录页与 Cookie

浏览器无法发送自定义请求头，因此当 `httpAuthHeader` 非空时，frps 会自己提供一个表单登录页，
并用签名 Cookie 记住登录状态。

### 5.1 判断规则

| 请求特征 | 响应 |
| --- | --- |
| 有合法 Cookie 或有效的鉴权头 | 正常转发 |
| 无凭据，`GET`/`HEAD` 且 `Accept` 含 `text/html` | `302` 跳转到 `<base>/login?redirect=<原路径>` |
| 无凭据，把 frps 当 HTTP 代理用（绝对 URI / CONNECT） | `407` + `Proxy-Authenticate` |
| 无凭据，其他情况（`curl`、`fetch`、`POST` 等） | `401`，且**不带** `WWW-Authenticate`，不会弹浏览器登录框 |

这样浏览器可以正常登录，而 API 客户端拿到的是标准状态码，不会收到一坨 HTML。

### 5.2 流程

```
浏览器 GET /page
  │  无 cookie / 无鉴权头
  ├─ 302 → /<base>/login?redirect=%2Fpage
  │
  ├─ GET  /<base>/login        → 200 登录表单
  ├─ POST /<base>/login        → 账号密码正确：Set-Cookie + 302 回 /page
  │                             账号密码错误：401 + 页面提示
  ├─ GET  /page  (带 Cookie)   → 200，转发到源站
  └─ GET  /<base>/logout       → 清除 Cookie，回到登录页
```

### 5.3 路径配置

- 默认 `<base>` 为 `/.auth`，因此默认登录页是 `/.auth/login`，退出是 `/.auth/logout`。
- 用 `httpAuthPagePath` 改成任意基路径，例如 `/_o7k2/entry` → `/_o7k2/entry/login`。
- 结尾的 `/` 会自动去掉：`/_o7k2/entry/` 与 `/_o7k2/entry` 等价。
- **这两个路径是该代理的保留路径**，不会再转发给源站。改成不冲突的值即可避免业务路由被遮蔽。
- 改了路径之后，原来的 `/.auth/login` 就是普通业务路径了（未登录访问会被 302 到新登录页）。

校验规则（配置加载阶段报错）：必须以 `/` 开头且不能只有 `/`，不能包含空白、`?`、`#`、`..`、`//`，
并且必须同时设置 `httpAuthHeader`。

### 5.4 Cookie 细节

| 项 | 值 |
| --- | --- |
| 名称 | `frp_auth_<16 位十六进制>`，由 `domain + location + routeByHTTPUser` 的哈希得到，同一域名下多个代理互不覆盖 |
| 值 | `v1.<过期时间戳>.<HMAC-SHA256>` |
| 签名密钥 | 由该代理的 `domain/location/routeByHTTPUser/httpUser/httpPassword` 派生，**改密码后旧 Cookie 自动失效**，无需额外配置密钥 |
| 有效期 | 12 小时 |
| 属性 | `HttpOnly`、`SameSite=Lax`、`Path=/`，明文端口下不设置 `Secure` |
| 转发 | 转发给源站前会从 `Cookie` 头中摘掉这一条，源站自己的 Cookie 保持原样 |

退出登录只需要访问 `<base>/logout`（GET 即可），也可以手动删除该 Cookie。

### 5.5 与默认模式的区别

`httpAuthHeader` 为空时**不会**启用登录页，行为与官方 frp 一致：`401` + `WWW-Authenticate`，
由浏览器弹出原生 Basic 登录框。

### 5.6 用 nginx 统一注入凭据

如果 frps 前面有 nginx，可以让 nginx 写死 `X-FRP-Auth`：客户端完全无感，登录页和 Cookie 都不再需要。

```nginx
set $frp_auth "Basic YWxpY2U6czNjcmV0";   # printf 'alice:s3cret' | base64

location /api/ {
    proxy_set_header X-FRP-Auth $frp_auth;  # 覆盖语义，客户端伪造无效
    proxy_set_header Host $host;            # frps 靠 Host 选路由
    proxy_pass http://frp_vhost;
}
```

完整示例（按路径分流、SSE、WebSocket、放行内网直连）见
[`nginx_frp_auth.example.conf`](./nginx_frp_auth.example.conf)，分场景说明见 [第 11 节](#11-前置-nginx统一注入凭据与按路径分流)。

要点：
- frpc 侧若配置了 `allowIPs`，必须把 nginx 的出口 IP 加进白名单。
- 此时 nginx 是唯一入口，它自己需要做访问控制（`allow/deny`、`auth_basic`、mTLS 等）。
- frps 转发前会删掉 `X-FRP-Auth`，源站看不到该头。

## 6. IP 白名单：`allowIPs`

```toml
allowIPs = ["192.168.1.0/24", "203.0.113.7", "2001:db8::/32"]
```

- 支持单个 IPv4/IPv6 地址和 CIDR 网段；IPv4-mapped IPv6（`::ffff:1.2.3.4`）会被归一化后比较。
- 为空表示不限制；写错（如 `10.0.0.0/33`）会在 frpc 加载配置时报错，不会静默放行。
- **判定在鉴权之前**：不在白名单的请求直接 `403`，不会看到登录页，也探测不到密码是否正确。
- 无法解析对端地址时按"拒绝"处理（fail closed）。

### ⚠️ 比较的是 TCP 对端地址

`allowIPs` 取的是与 frps 建立连接的来源 IP：

- 直连 frps：就是客户端真实 IP。
- 前面有 CDN / SLB / Nginx 反向代理：拿到的是**代理机器的 IP**，白名单需要写代理网段，
  真实客户端 IP 要靠 `X-Forwarded-For`，而该头可被伪造，不适合做安全边界。
- 通过 HTTP 代理模式（CONNECT）接入时同样是 TCP 对端地址。

## 7. 与既有功能的关系

| 功能 | 与本组字段的关系 |
| --- | --- |
| `routeByHTTPUser` | 依然从 `Authorization`（代理模式为 `Proxy-Authorization`）中取用户名做路由，与 `httpAuthHeader` 无关。用 Cookie 登录的浏览器请求没有 `Authorization`，因此不会命中按用户分流的路由 |
| `[proxies.plugin] type = "http_proxy"` 的 `httpUser/httpPassword` | 校验的是 `Proxy-Authorization`，返回 `407`，和 vhost 鉴权是两条独立链路 |
| `[proxies.plugin] type = "static_file"` 的 `httpUser/httpPassword` | 插件自己做 Basic Auth，返回 `401` |
| `[webServer] user/password`（frps 面板） | Dashboard 登录，和代理鉴权无关 |
| `type = "https"` | **不支持**以上字段。TLS 在 frps 终止后走 Muxer，不经过 HTTP 反向代理层 |
| `type = "tcpmux"` | 仍只支持 `Proxy-Authorization` 方式 |

## 8. 安全说明

- **Basic 凭据只是 base64，不是加密**。走明文 `vhostHTTPPort` 时，`X-FRP-Auth` 可被抓包读取；
  公网场景建议配合 `vhostHTTPSPort` 或在前面加 TLS 终结。
- **Cookie 只含过期时间和签名，不含密码**，但仍是可重放的会话凭据，同样建议跑在 TLS 之上。
- **登录页路径不是安全边界**。未登录访问任意页面时，`302` 的 `Location` 就会暴露真实登录路径。
  隐藏路径的价值仅在于避免路由冲突、不暴露中间件身份。
- 登录失败时日志会记录来源 IP 和用户名（不含密码），便于排查暴力破解。
- 用户名/密码比较使用常量时间比较，避免时序侧信道。

## 9. 兼容性与限制

- **frpc 与 frps 必须使用同一份新代码**。新字段通过 `NewProxy` 控制消息下发，旧 frps 会忽略
  `http_auth_header` / `http_auth_page_path` / `allow_ips`，表现为"配置没生效"（静默退回读 `Authorization`）。
- 只覆盖 `type = "http"`。
- `web/frpc` 前端表单尚未包含这三个字段，用 Web UI 编辑代理会丢掉它们，请直接编辑 TOML。
- legacy INI 配置未新增对应键，只支持 v1 TOML/JSON。
- `https` 类型的证书与 Muxer 逻辑未改动。

## 10. 排错

| 现象 | 排查方向 |
| --- | --- |
| 页面直接 401，没有登录框 | 这是预期行为（自定义头模式且非浏览器导航）。用 `curl -H 'Accept: text/html'` 复现，或直接用 `X-FRP-Auth` 调用 |
| 改成自定义头后仍要求旧密码 | 旧 frps 不认新字段，两端都要升级 |
| 一直 302 循环 | `httpAuthPagePath` 与源站路由冲突，或 Cookie 未带上（检查浏览器是否禁用了 Cookie、是否跨域） |
| 登录成功但立即又跳登录页 | 检查是否同时配置了多个代理共用同一域名，Cookie 名按路由隔离，跨代理不通用属正常 |
| 全部请求 403 | `allowIPs` 写的是真实客户端网段吗？经过 CDN 时这里是 CDN 的 IP |
| 401 但页面提示密码错误 | 密码含特殊字符时注意 TOML 转义；登录表单提交上限 64KB |

## 11. 前置 nginx：统一注入凭据与按路径分流

### 11.1 适用场景

- 客户端不方便带自定义头（浏览器、老旧系统、第三方回调）。
- 想按接口路径把流量分给不同的后端。
- 想把 frp 凭据集中在网关，外部客户端完全无感。

职责划分：**nginx 负责注入凭据和分流，frp 只负责认头放行**。客户端不需要知道 frp 的存在。

### 11.2 关键配置

```nginx
set $frp_auth "Basic YWxpY2U6czNjcmV0";   # printf 'alice:s3cret' | base64

location /api/ {
    proxy_set_header X-FRP-Auth $frp_auth;   # 覆盖语义，客户端伪造无效
    proxy_set_header Host $host;             # frps 靠 Host 选路由
    proxy_pass http://frp_vhost;
}
```

`proxy_set_header` 是覆盖而不是追加：客户端自己塞的 `X-FRP-Auth` 会被彻底替换，既伪造不了凭据，
也不会把假值透传给源站。

### 11.3 完整示例

```nginx
# frps 的 vhostHTTPPort（不是 bindPort）
upstream frp_vhost {
    server 127.0.0.1:8180;
    keepalive 16;
}

server {
    listen 80;
    server_name web01.example.com;

    set $frp_auth "Basic YWxpY2U6czNjcmV0";

    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_connect_timeout 5s;
    proxy_read_timeout 60s;

    # 走 frp，由 nginx 注入凭据
    location /api/ {
        proxy_set_header X-FRP-Auth $frp_auth;
        proxy_pass http://frp_vhost;
    }

    # SSE / 长轮询
    location /api/stream {
        proxy_set_header X-FRP-Auth $frp_auth;
        proxy_buffering off;
        proxy_read_timeout 3600s;
        proxy_pass http://frp_vhost;
    }

    # WebSocket
    location /ws/ {
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header X-FRP-Auth $frp_auth;
        proxy_read_timeout 3600s;
        proxy_pass http://frp_vhost;
    }

    # 不走 frp，直连内网；空值表示不向后端发送该头
    location /internal/ {
        proxy_set_header X-FRP-Auth "";
        proxy_set_header Host "internal.example.com";
        proxy_pass http://10.0.0.20:8080;
    }

    # 其余流量仍然走 frp
    location / {
        proxy_set_header X-FRP-Auth $frp_auth;
        proxy_pass http://frp_vhost;
    }
}
```

可直接复制使用：`doc/nginx_frp_auth.example.conf`（含多账号、nginx 自身鉴权的注释）。语法请本地
`nginx -t` 校验后再 reload。

### 11.4 四个注意点

| 注意点 | 说明 |
| --- | --- |
| `allowIPs` 要放行 nginx | 同机部署写 `127.0.0.1`，跨机写 nginx 的内网地址。否则 nginx 转过去的请求会直接 `403` |
| `Host` 必须对 | frps 按 Host 匹配 `customDomains`。对外域名与 frpc 配置不一致时，改成 `proxy_set_header Host "web01.example.com";` 写死 |
| nginx 成为唯一入口 | 客户端不再需要任何 frp 凭据，公网暴露时必须在 nginx 上做 `allow/deny`、`auth_basic` 或 mTLS |
| 凭据是静态的 | 改密码必须同步改 `$frp_auth`，否则 frps 直接 401。多套账号定义多个变量分别绑定 |

### 11.5 排错

| 现象 | 原因 |
| --- | --- |
| `404` 且页面是 frp 的 not found | `Host` 没传对，或路径没匹配到该代理的 `locations` |
| `401` | `$frp_auth` 值不对：base64 算错、漏了 `Basic ` 后的空格、或密码改过 |
| `403` | frpc 侧配了 `allowIPs`，但没放行 nginx 的 IP |
| SSE 响应卡住 | 缺少 `proxy_buffering off` / `proxy_read_timeout` 太短 |
| 客户端伪造头无效 | 预期行为，`proxy_set_header` 覆盖客户端传入值 |
| 登录页仍然出现 | 某个 location 忘了注入头；浏览器导航会走到 frp 的登录页 |

### 11.6 不需要 nginx 的替代方案：`locations` 分流

如果只是想把同一域名下的不同路径分给不同后端，frp 自身就支持，同一域名可以注册多个 http 代理：

```toml
[[proxies]]
name = "api"
type = "http"
localPort = 8080
customDomains = ["web01.example.com"]
locations = ["/api"]        # 只接管 /api 开头的请求

[[proxies]]
name = "web"
type = "http"
localPort = 80
customDomains = ["web01.example.com"]
locations = ["/"]           # 其余全部
```

匹配规则（`pkg/util/vhost/router.go`）：按 `location` 字典序降序比较，**更具体的路径优先**，
与 proxy 的注册顺序无关。实测 `/api/users` 稳定命中 `locations = ["/api"]` 的代理，
`/other` 命中 `locations = ["/"]` 的代理。

局限：只能按路径前缀区分，不能改路径、不能做负载均衡或健康检查、不能对单个接口加额外鉴权。
需要这些能力时，用 nginx 更合适；两者也可以叠加——nginx 负责分流和注入头，frp 侧只留一个
`locations = ["/"]` 的代理。

## 12. 代码位置

| 文件 | 内容 |
| --- | --- |
| `pkg/util/vhost/auth_page.go` | 登录页、Cookie 签名与校验、路径解析、Cookie 剥离 |
| `pkg/util/vhost/ip_filter.go` | `IPAllowList`，IP/CIDR 解析与匹配 |
| `pkg/util/vhost/http.go` | 请求入口：IP → 登录页 → 鉴权 → 转发；鉴权头与 Cookie 剥离 |
| `pkg/util/vhost/vhost.go` | `RouteConfig` 新增 `AuthHeader`、`AuthPagePath`、`AllowList` |
| `pkg/config/v1/proxy.go` | `HTTPProxyConfig` 新增 `httpAuthHeader`、`httpAuthPagePath`、`allowIPs` |
| `pkg/msg/msg.go` | 控制消息字段 `http_auth_header`、`http_auth_page_path`、`allow_ips` |
| `pkg/config/v1/validation/proxy.go` | 配置加载期校验 |
| `server/proxy/http.go` | 组装 `RouteConfig`，非法配置导致代理启动失败 |

测试：`go test ./pkg/util/vhost/... ./pkg/config/...`，覆盖鉴权头切换、Cookie 签名/过期/篡改/跨路由、
登录页流程、开放重定向防护、Cookie 剥离、IP 白名单匹配与 fail-closed。
