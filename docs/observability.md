# Observability

Enable the local diagnostics server explicitly:

```yaml
Diagnostics:
  Enable: true
  Listen: 127.0.0.1:8080
```

Endpoints:

- `/healthz`: process liveness;
- `/readyz`: all configured nodes are initialized;
- `/status`: sanitized node state;
- `/metrics`: Prometheus metrics.

For structured logs:

```yaml
Log:
  Level: info
  Format: json
```

Metrics use only low-cardinality labels. User IDs, email addresses, IP addresses, tokens, passwords, and private keys are not labels.
