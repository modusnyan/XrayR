# Troubleshooting

## Unsupported panel type

Run `config check`. Panel names are case-insensitive and spelling suggestions are shown. `Xboard`, `NewV2board`, and `V2board` resolve to the same UniProxy adapter.

## Authentication failure

Verify `ApiKey`, `NodeID`, and `NodeType`. `doctor` classifies HTTP 401/403 separately from network failures.

## Configuration reload failed

The previous validated configuration remains active or is restarted. Inspect structured logs and `xrayr_config_reloads_total{result="rollback"}`.

## Panel unavailable during restart

If cache is enabled and the snapshot is valid, recent, and matches the configured panel identity, XrayR starts from it. Expired, corrupted, or mismatched snapshots are rejected.

## Node stopped serving and logs "Panel disabled the node"

The node is switched off in Xboard. It keeps polling and comes back on the next
pull after the node is enabled again. If the node was not switched off, check
that nothing between the node and the panel answers 410 with an
`X-Node-State: disabled` header.

## The panel node list shows a yellow dot, `last_error` is set

The node rejected the configuration the panel sends and is still serving the
previous one. `last_error` and the panel log (`Node rejected its configuration`)
give the reason. Typical causes:

- `unsupported shadowsocks plugin` / `unsupported shadowsocks obfs mode`: only
  simple-obfs in http mode is supported. Remove the plugin or set
  `plugin=obfs`, `plugin_opts=obfs=http;obfs-host=...`.
- an unknown XHTTP `mode`: use `auto`, `packet-up`, `stream-up` or `stream-one`.
- a transport or cipher xray-core cannot build (`h2`, `kcp`, `aes-192-gcm`,
  `2022-blake3-chacha20-poly1305`).

A Shadowsocks node that cannot start at all, with no snapshot to fall back on,
fails the whole process. Fix the node in the panel first.

## "Panel routes ignored"

Only routes with action `block` are enforced. `dns`, `direct` and `proxy` routes
have no effect on the node; remove them from the node in the panel.

## Certificate configuration warnings

XrayR builds TLS from the local `CertConfig` and only compares it with the
panel's `cert_config`. "listens WITHOUT TLS" means the node is TLS in the panel
but has no local certificate: clients cannot connect until `CertConfig` is set.
The other warnings say which value is used; align the two to silence them.

## Users refused after changing networks (before 1.2.0)

The local `DeviceLimit` used to remember every address a user had connected from
until XrayR restarted. 1.2.0 resets the count every push period. Upgrade, or
restart the service as a workaround.

## "The panel has no node status endpoint" / "no online user endpoint"

The panel is an Xboard build without `UniProxy/status` and `UniProxy/alive`.
Reporting is switched off until XrayR restarts; traffic reporting is not
affected. Upgrade Xboard to receive the reports.

## Diagnostics endpoint unavailable

Diagnostics are disabled by default. Enable them and use a loopback listen address unless an authenticated reverse proxy protects the endpoint.
