# Other panel integrations

XrayR also supports the following panel adapters through the same `Nodes` configuration structure:

| `PanelType` | Adapter | Typical protocols |
|---|---|---|
| `SSpanel` | SSPanel UIM API | Vmess, Vless, Trojan, Shadowsocks, Shadowsocks-Plugin |
| `PMpanel` | PMPanel API | Vmess, Trojan, Shadowsocks |
| `Proxypanel` | ProxyPanel API | Vmess, Vless, Trojan, Shadowsocks |
| `V2RaySocks` | V2RaySocks API | Vmess, Vless, Trojan, Shadowsocks |
| `GoV2Panel` | GoV2Panel API | Vmess, Vless, Trojan, Shadowsocks |
| `BunPanel` | BunPanel API | Vmess, Vless, Trojan, Shadowsocks |

Use `XrayR config check` to verify the exact protocol accepted by the selected adapter. Unsupported capabilities—such as online-user or node-status reporting—are skipped according to the registry capability matrix rather than treated as successful no-op requests.

A minimal node block is:

```yaml
Nodes:
  - PanelType: SSpanel
    ApiConfig:
      ApiHost: https://panel.example.com
      ApiKey: CHANGE_ME
      NodeID: 1
      NodeType: Vless
```

For panel-specific fields, follow the panel project's node API documentation and run `XrayR doctor` before starting the service.
