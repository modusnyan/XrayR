# XrayR Documentation

XrayR is a multi-panel backend for Xray-core. This documentation focuses on safe configuration, validation, diagnosis, recovery, and operations.

## Recommended deployment flow

```bash
XrayR config init
XrayR config check -c /etc/XrayR/config.yml
XrayR doctor -c /etc/XrayR/config.yml
systemctl enable --now XrayR
```

## Start here

- [Installation](installation.md)
- [Configuration](configuration.md)
- [CLI reference](cli.md)
- [Diagnostics](diagnostics.md)
- [Troubleshooting](troubleshooting.md)
