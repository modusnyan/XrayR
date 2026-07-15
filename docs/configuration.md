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

## Terminal configuration UI

Run `XrayR config init -c /etc/XrayR/config.yml` to create a new configuration or edit an existing one. The UI maps directly to the complete Go configuration model rather than a reduced template, including global settings and multiple nodes.

Fields are displayed conditionally:

- panel selection controls the available node protocols;
- VLESS flow appears only for VLESS or V2ray with VLESS enabled;
- certificate fields change between none, existing files and ACME modes;
- local REALITY fields are hidden when panel-supplied REALITY is selected;
- fallback entries are available only for Trojan and VLESS;
- Redis, automatic speed limiting, diagnostics and cache details appear only when enabled.

The final review runs the same `config.Validate` rules used at startup. Static errors block saving. Doctor failures may be overridden only after an explicit confirmation, which supports deliberately saving an offline deployment.

Secrets shown by `config show` and the configuration UI are replaced by `***REDACTED***`. This includes API keys, Redis passwords, REALITY private keys and DNS provider environment values.
