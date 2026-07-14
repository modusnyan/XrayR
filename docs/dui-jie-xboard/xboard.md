# 对接 Xboard

## 概述

Xboard 使用与 V2board 兼容的 UniProxy API，XrayR 通过 **NewV2board** 面板类型即可对接 Xboard。

从 XrayR v0.10.0 起，`PanelType` 支持直接填写 `"Xboard"`，它是 `"NewV2board"` 的别名，两者完全等价。

## 基本对接配置

1. 在 `config.yml` 中配置 `PanelType:` 填写 `"Xboard"` 或 `"NewV2board"`。
2. Xboard 需要启用 `EnableVless: true` 时请在配置文件中手动开启，面板端不支持在线配置 vless。
3. Xboard 支持审计规则下发（路由规则），如需使用审计功能请确保 Xboard 面板已配置相应规则。

{% hint style="info" %}
Xboard 兼容 V2board UniProxy API，因此选择 `"Xboard"` 或 `"NewV2board"` 均可正常对接。
{% endhint %}

配置文件详见：[配置文件说明](../xrayr-pei-zhi-wen-jian-shuo-ming/config.md)

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
      VlessFlow: "xtls-rprx-vision"
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

{% hint style="info" %}
XrayR 会使用 ETag 机制进行增量更新，避免不必要的数据传输。当面板配置未变更时返回 HTTP 304，XrayR 将跳过本次更新。
{% endhint %}

## 对接 Shadowsocks2022

确保 Xboard 面板支持 Shadowsocks2022 协议，XrayR 需要配置：

```yaml
ApiConfig:
  NodeType: Shadowsocks
```

面板端需在节点配置中设置 `cipher` 和 `server_key` 字段。

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

{% hint style="warning" %}
Xboard 面板本身可能不支持 TCP+HTTP 订阅下发，请自行确保客户端配置正确。
{% endhint %}

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

## 对接 Reality

Xboard 面板需在节点配置中启用 Reality（TLS 类型选择 2），XrayR 配置中启用：

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

{% hint style="info" %}
如果已在 Xboard 面板侧配置了 Reality 参数，建议设置 `DisableLocalREALITYConfig: true` 以优先使用面板下发的配置。
{% endhint %}

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
| `VlessFlow` | string | VLESS flow 模式，如 `xtls-rprx-vision` |
| `SpeedLimit` | float | 本地限速（Mbps），会覆盖远程设置，0 为不启用 |
| `DeviceLimit` | int | 本地设备限制，会覆盖远程设置，0 为不启用 |
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
| `EnableProxyProtocol` | bool | 启用 ProxyProtocol 获取中转 IP |
| `EnableFallback` | bool | 启用 Fallback（仅对 VLESS 和 Trojan 有效） |
| `EnableREALITY` | bool | 启用 REALITY |
| `DisableLocalREALITYConfig` | bool | 禁用本地 REALITY 配置，优先使用面板下发 |
| `CertConfig` | object | 证书申请配置 |

## 常见问题

### Xboard 与 V2board 有什么区别？

Xboard 是 V2board 的现代化分支，后端基于 Laravel 11 + Octane，前后端分离架构。在 API 层面，Xboard 兼容 V2board 的 UniProxy 接口，因此 XrayR 使用 `NewV2board` 或 `Xboard` 类型均可对接。

### 面板配置变更后多久生效？

默认 `UpdatePeriodic` 为 60 秒，即 XrayR 每 60 秒向面板拉取一次最新配置。如需加快同步，可以调小此值（建议不低于 30 秒以避免过多请求）。

### 如何验证对接成功？

1. 检查 XrayR 日志，确认节点信息获取成功
2. 在 Xboard 面板中查看节点状态，应显示为"在线"
3. 客户端连接后，面板应显示用户流量统计
