# CLI

## Validate

```bash
XrayR config check -c config.yml
XrayR config check -c config.yml --format json
```

## Display normalized configuration

```bash
XrayR config show -c config.yml --format yaml
```

## Migrate

```bash
XrayR config migrate --input old.yml --output new.yml
```

Migration never overwrites an existing file unless `--force` is provided.

## Create or edit configuration

```bash
XrayR config init -c /etc/XrayR/config.yml
```

When started from a terminal without all required automation flags, `config init` opens a full-screen terminal UI. If the target file already exists, its current values are loaded for editing.

The UI covers:

- global logging, connection policy, custom JSON paths, diagnostics and cache;
- multiple panel nodes with add, edit, clone and delete actions;
- panel credentials, node type, VLESS, local limits and rules;
- controller networking, DNS, traffic/rule reporting and protocol switches;
- none/file/HTTP/TLS/DNS certificate modes and DNS provider environment variables;
- local or panel-supplied REALITY configuration;
- automatic speed limiting, Redis global device limiting and multiple fallback entries.

Before writing, the UI shows a redacted YAML preview and redacted diff, runs static validation, and offers to run Doctor. API keys, Redis passwords, REALITY private keys, URL query strings and DNS provider secrets are never shown in the preview.

For automation, provide all required flags. This path never opens the terminal UI:

```bash
XrayR config init \
  --panel Xboard \
  --api-host https://panel.example.com \
  --api-key "$API_KEY" \
  --node-id 1 \
  --node-type Vless \
  --cert-mode none \
  --output /etc/XrayR/config.yml \
  --force
```

If required flags are missing while stdin/stdout are not terminals, the command exits instead of waiting for input. Use `XrayR config init --help` for advanced certificate, REALITY, Redis, fallback and controller flags.

## Diagnose

```bash
XrayR doctor -c config.yml --timeout 5s
```
