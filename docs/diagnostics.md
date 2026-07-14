# Diagnostics

`XrayR doctor` runs read-only checks for:

- strict configuration validation;
- DNS, TCP, and TLS connectivity;
- certificate and key files;
- Redis when global device limiting is enabled;
- panel node and user endpoints.

The command does not report traffic, node status, online users, or audit events.

JSON output is suitable for automation:

```bash
XrayR doctor -c config.yml --format json
```

To check one node, use its zero-based index:

```bash
XrayR doctor -c config.yml --node 0
```
