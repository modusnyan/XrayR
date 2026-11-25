# 对接 Xboard

## 概述

Xboard 使用与 V2board 兼容的 UniProxy API，XrayR 通过 **NewV2board** 面板类型即可对接 Xboard。

从 XrayR v0.10.0 起，`PanelType` 支持直接填写 `"Xboard"`，它是 `"NewV2board"` 的别名，两者完全等价。

## 基本对接配置

1. 在 `config.yml` 中配置 `PanelType:` 填写 `"Xboard"` 或 `"NewV2board"`。
2. Xboard 需要启用 `EnableVless: true` 时请在配置文件中手动开启，面板端不支持在线配置 vless。
3. Xboard 支持审计规则下发（路由规则），**只有动作为 `block`（阻断）的规则生效**；`dns` / `direct` / `proxy` 规则会被忽略并在日志里提示一次。
4. 面板与 XrayR 请使用同一批次的版本（XrayR 1.2.0 起需要面板提供 `UniProxy/alive`、`UniProxy/status` 和节点禁用信号），旧面板仍可对接，只是缺少状态回传。

> **说明**
>
> Xboard 兼容 V2board UniProxy API，因此选择 `"Xboard"` 或 `"NewV2board"` 均可正常对接。

配置文件详见：[配置文件说明](../configuration.md)

### 基础配置示例

```yaml
Nodes:
  -
    PanelType: "Xboard" # Panel type: Xboard, NewV2board
    ApiConfig:
      ApiHost: "https://your-xboard-domain.com"
      ApiKey: "your-api-key"
      NodeID: 1
      NodeType: V2ray # Node type: V2ray, Vmess, Vless, Trojan, Shadowsocks
      Timeout: 30
      EnableVless: false
      SpeedLimit: 0
      DeviceLimit: 0
      RuleListPath: # /etc/XrayR/rulelist
    ControllerConfig:
      ListenIP: 0.0.0.0
      SendIP: 0.0.0.0
      UpdatePeriodic: 60
      EnableDNS: false
      DNSType: AsIs
      EnableProxyProtocol: false
      DisableSniffing: false
      CertConfig:
        CertMode: none # none, file, http, dns
        CertDomain: "node.example.com"
        CertFile: /etc/XrayR/cert/node.example.com.cert
        KeyFile: /etc/XrayR/cert/node.example.com.key
```

### 对接方式说明

XrayR 通过 REST API 周期性拉取 Xboard 面板配置，默认每 60 秒（`UpdatePeriodic`）同步一次节点信息和用户列表，并通过 POST 请求上报用户流量。

| 操作 | API 端点 | 方法 |
|------|----------|------|
| 获取节点配置 | `/api/v1/server/UniProxy/config` | GET |
| 获取用户列表 | `/api/v1/server/UniProxy/user` | GET |
| 上报用户流量 | `/api/v1/server/UniProxy/push` | POST |
| 上报在线设备 | `/api/v1/server/UniProxy/alive` | POST |
| 上报负载与同步状态 | `/api/v1/server/UniProxy/status` | POST |

> **说明**
>
> XrayR 会使用 ETag 机制进行增量更新，避免不必要的数据传输。当面板配置未变更时返回 HTTP 304，XrayR 将跳过本次更新。
>
> 拉取间隔以面板 `base_config.pull_interval` 为准，上报间隔以 `push_interval` 为准；面板没给时用本地 `UpdatePeriodic`。

### 节点启停与状态回传

- **面板关闭节点**：Xboard 对 `config` / `user` 返回 HTTP 410 + 响应头 `X-Node-State: disabled`。
  XrayR 立即移除入站、删除本地快照（面板故障时重启也不会把节点拉起来），把下线前已统计的流量报完，
  之后继续轮询；面板重新启用后下一轮自动恢复。只有这个完整信号会让节点下线，网络错误、5xx 等仍按
  「面板暂不可用」处理并保持服务。**面板前的 CDN / 反代必须透传 `X-Node-State` 响应头。**
- **用户全部移除**：面板返回空用户列表时，XrayR 等下一轮确认后再清空节点上的用户，避免一次异常响应
  让所有人断线。
- **状态回传**：每个上报周期发送在线设备（`{"uid": ["ip", ...]}`）和负载 / 同步状态，后台节点列表的
  「在线人数」列和状态悬浮框、用户详情的「在线设备数 / 最后在线时间」都来自这里。面板没有这两个接口
  （返回 404）时只告警一次，之后不再发送。
- **配置被拒**：面板下发的配置 XrayR 无法应用时，节点继续用上一份有效配置运行，错误保留在诊断页
  `last_error` 并上报给面板（节点状态变黄、「内核状态」显示 OFF），直到面板换了配置为止。

## 对接 Shadowsocks2022

确保 Xboard 面板支持 Shadowsocks2022 协议，XrayR 需要配置：

```yaml
ApiConfig:
  NodeType: Shadowsocks
```

面板端需在节点配置中设置 `cipher` 和 `server_key` 字段。

可用的加密方式：

| 加密方式 | 说明 |
|---|---|
| `aes-128-gcm` / `aes-256-gcm` / `chacha20-ietf-poly1305` | 普通 Shadowsocks |
| `2022-blake3-aes-128-gcm` / `2022-blake3-aes-256-gcm` | Shadowsocks 2022 |
| `2022-blake3-chacha20-poly1305` | **不可用**：XrayR 按面板用户增删账号需要多用户模式，该模式只接受 `blake3-aes-*-gcm` |

### 混淆插件

只支持 simple-obfs 的 **http** 模式。面板里这样配（订阅端也按这两个字段生成客户端参数）：

| 字段 | 值 |
|---|---|
| 插件 `plugin` | `obfs` |
| 插件参数 `plugin_opts` | `obfs=http;obfs-host=www.bing.com`（可选 `path=/xxx`） |

`v2ray-plugin` 等其他插件、`obfs=tls` 都无法由 xray-core 实现：XrayR 会**明确报错、节点不启动**，
而不是不带插件运行（那样所有客户端都连不上）。升级前请先在面板里清理这类节点。
旧字段 `obfs` / `obfs_settings` 仍然兼容。

## 对接 Vmess + WebSocket

Xboard 面板需要在节点传输协议配置中增加以下内容，配置 ws 的路径：

```json
{
  "path": "/your-path"
}
```

其中 `"your-path"` 换成任意字符串，可用于 nginx 等反代分流。

## 对接 Vmess + WebSocket + TLS

Xboard 面板需要在节点传输协议配置中增加以下内容：

```json
{
  "path": "/",
  "headers": {
    "Host": "your-domain.com"
  }
}
```

`"Host"` 后面的域名更改为自己的伪装域名。

同时 XrayR 配置中需要启用证书：

```yaml
ControllerConfig:
  CertConfig:
    CertMode: dns # 或 file、http
    CertDomain: "your-domain.com"
    Provider: alidns
    Email: your-email@example.com
    DNSEnv:
      ALICLOUD_ACCESS_KEY: aaa
      ALICLOUD_SECRET_KEY: bbb
```

## 对接 Vmess + gRPC

Xboard 面板需要在节点传输协议配置中增加如下内容：

```json
{
  "serviceName": "your-service-name"
}
```

其中 `"your-service-name"` 换成任意字符串，可用于 nginx 等反代分流。

## 对接 Vmess + TCP + HTTP

> **警告**
>
> Xboard 面板本身可能不支持 TCP+HTTP 订阅下发，请自行确保客户端配置正确。

Xboard 面板需要在节点传输协议配置中增加如下内容：

```json
{
  "header": {
    "type": "http",
    "request": {},
    "response": {}
  }
}
```

其中 `request` 和 `response` 中的内容请参照 [Xray-core 文档](https://xtls.github.io/config/transports/tcp.html#httpheaderobject) 设置。

## 对接 VLESS / Vmess + XHTTP

面板传输协议选 `xhttp`，传输配置示例：

```json
{
  "host": "cdn.example.com",
  "path": "/yourpath",
  "mode": "auto",
  "extra": {
    "xPaddingBytes": "100-1000",
    "scMaxEachPostBytes": 1000000
  }
}
```

- `host` 会用于服务端 Host 校验（1.2.0 之前 xhttp 节点的 Host 被忽略）。
- `mode` 可选 `auto` / `packet-up` / `stream-up` / `stream-one`，填错时节点保留旧配置并报错。
- `extra` 原样传给 xray-core；其中只有客户端使用的 `downloadSettings`、`xmux` 会被去掉
  （面板默认模板里 `downloadSettings` 的空地址不能交给服务端构建）。

## 对接 Reality

**面板配好就够了，XrayR 的配置文件不需要写任何 REALITY 字段。**

在 Xboard 的节点配置里把 TLS 类型选成 **2（Reality）**，填好 `dest` / `server_name` / `private_key` / `short_id`，
XrayR 拉取节点时会一并取回这些参数并直接用于构建 inbound：

```yaml
Nodes:
  - PanelType: "Xboard"
    ApiConfig:
      ApiHost: "https://panel.example.com"
      ApiKey: "YOUR_KEY"
      NodeID: 7
      NodeType: "Vless"
      EnableVless: true      # VLESS 节点建议开启，否则读不到 network_settings 里的 path / serviceName
    ControllerConfig:
      ListenIP: 0.0.0.0
      UpdatePeriodic: 60
```

面板 `tls_settings` 与 inbound 的对应关系：

| 面板字段 | 作用 |
|---|---|
| `dest` | REALITY 回落目标，可选。可写 `host`、`host:port`、`[IPv6]:port`、本机端口号或 unix socket（`/path`、`@name`） |
| `server_port` | 回落端口，只在 `dest` 是裸主机名或留空时使用，空则为 443 |
| `server_name` | 客户端可用的 SNI；`dest` 留空时回落到 `server_name:server_port` |
| `private_key` | 服务端私钥 |
| `short_id` | 允许的 short id |
| `xver` | PROXY protocol 版本（0 / 1 / 2） |

> Xboard 后台表单没有 `dest` / `xver` 输入框，需要通过后台 API 保存节点时传
> `protocol_settings.reality_settings.dest` / `.xver`。不填时行为与之前一致。

REALITY 节点不需要 `CertConfig`，证书相关的申请与续签会被自动跳过。

### 本地 REALITY 配置（仅兜底）

REALITY 参数以面板为准。只有当**面板没有把节点描述成 REALITY**（`tls` 不是 2）时，
本地的 `REALITYConfigs` 才会被用来补一个 REALITY 服务端：

```yaml
ControllerConfig:
  EnableREALITY: true
  REALITYConfigs:
    Show: true
    Dest: www.microsoft.com:443
    ProxyProtocolVer: 0
    ServerNames:
      - www.microsoft.com
    PrivateKey: "your-private-key"
    ShortIds:
      - ""
      - "abcdef0123456789"
```

**优先级**：

1. 面板说这个节点是 REALITY → **用面板的 dest / server_name / private_key / short_id / xver**，
   本地 `REALITYConfigs` 一律忽略（只保留 `Show` 这个调试开关）。
2. 面板没说 REALITY，但本地配了 → 用本地的。
3. 两边都没有 → 不启用 REALITY。

> 为什么面板优先：面板下发给客户端的就是它自己那份参数。如果服务端用了本地另一套，
> 客户端握手会直接失败，而且日志里看不出原因。
>
> `DisableLocalREALITYConfig: true` 仍然有效，作用是**关掉第 2 条兜底**（永远不用本地 REALITY 参数）。

## 证书：面板 cert_config 与本地 CertConfig

TLS 节点（非 REALITY）**只使用本地 `ControllerConfig.CertConfig`**。面板节点里的证书配置
（`cert_config`）目前不会被应用，XrayR 只把它和本地配置比对，在启动和每次同步后告警一次：

| 告警 | 含义 |
|---|---|
| `listens WITHOUT TLS` | 面板说节点是 TLS，本地却没配证书：入站会以明文监听，客户端连不上 |
| `not applied by XrayR` | 面板配了证书、本地没配 |
| `cert mode ... differs` / `cert domain ... differs` / `DNS provider ... differs` | 两边不一致，以本地为准 |
| `not supported by XrayR` | 面板选了自签（`self`）或证书推送（`content`）模式 |

DNS 凭据、推送的证书和私钥内容不会被 XrayR 读取或记录。

## 面板能选、但 XrayR 不支持的选项

Xboard 后台前端编译自上游完整版，下面这些选项节点后端用不了。配套版本的 Xboard 会在保存节点时拦截，
订阅侧也不会再把多路复用 / ECH 下发给客户端：

| 选项 | 原因 |
|---|---|
| 传输 `h2` / `kcp` / `quic` | xray-core 已移除或 XrayR 未实现 |
| SS 加密 `aes-192-gcm`、`2022-blake3-chacha20-poly1305` | xray-core / 多用户模式不支持 |
| flow `xtls-rprx-direct` / `xtls-rprx-splice` | 已废弃；vision 只能配 `tcp` 且需要 TLS / REALITY |
| REALITY + `ws` / `httpupgrade` | xray-core 不支持该组合 |
| 多路复用（multiplex）、ECH | xray-core 不支持 sing-box 多路复用，XrayR 未接入 ECH |
| 自定义出站 / 自定义路由 | XrayR 不读取 |
| 路由 `dns` / `direct` / `proxy` | 只执行 `block` |

## 配置参数说明

### ApiConfig

| 参数 | 类型 | 说明 |
|------|------|------|
| `ApiHost` | string | Xboard 面板地址，示例 `https://panel.example.com` |
| `ApiKey` | string | 面板通讯密钥（对应 Xboard 节点配置中的 `token`） |
| `NodeID` | int | 节点 ID |
| `NodeType` | string | 节点类型：`V2ray`, `Vmess`, `Vless`, `Trojan`, `Shadowsocks` |
| `Timeout` | int | API 请求超时时间（秒），默认 5 秒 |
| `EnableVless` | bool | 为 V2ray 类型启用 VLESS 协议 |
| `VlessFlow` | string | **Xboard 下不生效**：节点使用面板中配置的 flow，本地填写只会在启动时提示被忽略 |
| `SpeedLimit` | float | 本地限速（Mbps），会覆盖远程设置，0 为不启用 |
| `DeviceLimit` | int | 本地设备限制，会覆盖远程设置，0 为不启用；按每个上报周期内见到的 IP 数计算 |
| `RuleListPath` | string | 本地审计规则文件路径 |

### ControllerConfig

| 参数 | 类型 | 说明 |
|------|------|------|
| `ListenIP` | string | 监听 IP 地址，`0.0.0.0` 同时监听 IPv4 和 IPv6 |
| `SendIP` | string | 发送数据使用的 IP 地址 |
| `UpdatePeriodic` | int | 从面板同步节点和用户信息的间隔（秒），默认 60 |
| `EnableDNS` | bool | 是否启用自定义 DNS |
| `DNSType` | string | DNS 策略：`AsIs`, `UseIP`, `UseIPv4`, `UseIPv6` |
| `DisableUploadTraffic` | bool | 禁止上报流量到面板 |
| `DisableGetRule` | bool | 禁止从面板获取审计规则 |
| `DisableSniffing` | bool | 关闭域名嗅探 |
| `DisableCoreHeartbeat` | bool | 关闭内核心跳探测（默认开启，探测连续失败 3 次会退出进程交由服务管理器重启） |
| `EnableProxyProtocol` | bool | 启用 ProxyProtocol 获取中转 IP |
| `EnableFallback` | bool | 启用 Fallback（仅对 VLESS 和 Trojan 有效） |
| `EnableREALITY` | bool | 启用 REALITY |
| `DisableLocalREALITYConfig` | bool | 禁用本地 REALITY 配置，优先使用面板下发 |
| `CertConfig` | object | 证书申请配置。TLS 节点只认这里，面板的 `cert_config` 仅用于比对告警 |

## 常见问题

### Xboard 与 V2board 有什么区别？

Xboard 是 V2board 的现代化分支，后端基于 Laravel 11 + Octane，前后端分离架构。在 API 层面，Xboard 兼容 V2board 的 UniProxy 接口，因此 XrayR 使用 `NewV2board` 或 `Xboard` 类型均可对接。

### 面板配置变更后多久生效？

默认 `UpdatePeriodic` 为 60 秒，即 XrayR 每 60 秒向面板拉取一次最新配置。如需加快同步，可以调小此值（建议不低于 30 秒以避免过多请求）。

### 如何验证对接成功？

1. 检查 XrayR 日志，确认节点信息获取成功（`Added N new users`）
2. 在 Xboard 面板中查看节点状态：绿点为正常；悬浮框里能看到 CPU / 内存 / 磁盘与运行指标
3. 客户端连接后，面板应显示用户流量统计，「在线人数」列随之变化
4. 节点状态为黄点且「内核状态」为 OFF：节点拒绝了面板最新配置，原因见诊断页 `last_error` 或面板日志

### 节点在面板关闭后多久停止服务？

下一次拉取时（默认 60 秒内）。关闭期间节点保持轮询，重新启用后下一轮自动恢复。

### 日志里出现 "The panel has no node status endpoint"？

面板是没有 `UniProxy/status`、`UniProxy/alive` 的旧版本 Xboard。状态回传已自动关闭，流量上报不受影响。
