# Migration and rollback

## Configuration migration

```bash
XrayR config migrate --input old.yml --output migrated.yml
```

Historical panel names are normalized and removed fields are reported as warnings.

## File reload rollback

A changed configuration is fully parsed and validated before the active instance is stopped. If the replacement cannot start, XrayR starts the previous validated configuration again and records a rollback metric.

## Panel snapshot cache

Validated node information, users, and audit rules are saved under the configured cache directory. API keys, passwords, and private keys are never written to snapshots. On restart, a fresh snapshot may be used when the panel is unavailable.

A node without users is saved with an empty user list and restored as such. When the panel disables a node, its snapshot is deleted, so a restart during a panel outage cannot bring a switched-off node back.

## Upgrading to 1.2.0

Upgrade together with the matching Xboard release, which adds `UniProxy/alive`, `UniProxy/status` and the disabled-node signal. Before upgrading:

1. Remove Shadowsocks plugins other than simple-obfs (http) from the panel. Such a node now fails with an explicit error, and a node that cannot start takes the other nodes of the process with it.
2. Make sure a CDN or reverse proxy in front of the panel passes the `X-Node-State` response header through.
3. `VlessFlow` in the local configuration is ignored; the panel's flow has always been the one in use, so nothing changes at runtime.

An older Xboard keeps working: the node logs one warning per missing report endpoint and continues.
