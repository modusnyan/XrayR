# Configuration

XrayR supports versioned configuration. New files should start with:

```yaml
ConfigVersion: 1
```

Three templates are provided:

- `release/config/config.minimal.yml`: required fields only;
- `release/config/config.yml.example`: common options;
- `release/config/config.full.yml`: all advanced options.

Use `XrayR config check` after every edit. Unknown keys are rejected so spelling mistakes do not silently disappear. Legacy unversioned files remain supported and can be upgraded with `config migrate`.

Secrets shown by `config show` are replaced by `***REDACTED***`.
