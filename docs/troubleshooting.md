# Troubleshooting

## Unsupported panel type

Run `config check`. Panel names are case-insensitive and spelling suggestions are shown. `Xboard`, `NewV2board`, and `V2board` resolve to the same UniProxy adapter.

## Authentication failure

Verify `ApiKey`, `NodeID`, and `NodeType`. `doctor` classifies HTTP 401/403 separately from network failures.

## Configuration reload failed

The previous validated configuration remains active or is restarted. Inspect structured logs and `xrayr_config_reloads_total{result="rollback"}`.

## Panel unavailable during restart

If cache is enabled and the snapshot is valid, recent, and matches the configured panel identity, XrayR starts from it. Expired, corrupted, or mismatched snapshots are rejected.

## Diagnostics endpoint unavailable

Diagnostics are disabled by default. Enable them and use a loopback listen address unless an authenticated reverse proxy protects the endpoint.
