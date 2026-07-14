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
