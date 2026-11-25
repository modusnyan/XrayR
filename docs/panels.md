# Supported panel

XrayR talks to exactly one panel backend: **Xboard**.

| `PanelType` | Adapter | Accepted node types |
|---|---|---|
| `Xboard` | `NewV2board` | `V2ray`, `Vmess`, `Vless`, `Trojan`, `Shadowsocks` |

`NewV2board` and `V2board` are accepted as historical aliases of `Xboard`, so
existing configurations keep working. Write `Xboard` in new configurations.

A minimal node block is:

```yaml
Nodes:
  - PanelType: Xboard
    ApiConfig:
      ApiHost: https://panel.example.com
      ApiKey: CHANGE_ME
      NodeID: 1
      NodeType: Vless
      Timeout: 30
```

Anything else in `PanelType` is rejected at startup with a message naming the
closest supported value:

```
unsupported panel type "Xbord"; did you mean "Xboard"? supported panels: Xboard
```

Run `XrayR config check` to validate the panel type and node type before
starting the service, and `XrayR doctor` for a full preflight report.

## What the node exchanges with Xboard

| Endpoint | Direction | Purpose |
|---|---|---|
| `GET /api/v1/server/UniProxy/config` | pull | Node configuration (ETag / 304) |
| `GET /api/v1/server/UniProxy/user` | pull | Users of the node (ETag / 304) |
| `POST /api/v1/server/UniProxy/push` | push | Per-user traffic |
| `POST /api/v1/server/UniProxy/alive` | push | Online devices per user, `{"uid": ["ip", ...]}` |
| `POST /api/v1/server/UniProxy/status` | push | Host load, users served and synchronization state |

`alive` and `status` need Xboard with the matching release. An older panel
answers 404; the node then logs one warning and stops sending that report.

A node switched off in the panel gets HTTP 410 with `X-Node-State: disabled` on
`config` and `user`. The node removes its inbound, deletes its snapshot and keeps
polling; it comes back when the panel enables it again. Only that exact signal
takes the node down: network errors, 5xx and other 4xx keep it running on the
last valid configuration. A CDN or reverse proxy in front of the panel must pass
the `X-Node-State` header through.

An empty user list is applied after the next pull confirms it, so the node stops
serving users who were all banned, expired or removed from its groups, while a
single bad answer cannot disconnect everyone.

## What the node does not take from the panel

| Panel setting | Behaviour |
|---|---|
| Routes with action `dns`, `direct` or `proxy` | Ignored with a warning; only `block` routes become audit rules |
| `cert_config` | Compared with the local `CertConfig` and logged when they differ; TLS always uses the local `CertConfig` |
| `multiplex`, ECH, custom outbounds and routes | Not supported; Xboard refuses to save them |
| Shadowsocks plugins other than simple-obfs (http) | The node fails with an explicit error |
| `VlessFlow` in the local configuration | Ignored; the panel's flow is used |

See [Xboard](dui-jie-xboard/xboard.md) for the panel-specific setup steps.
