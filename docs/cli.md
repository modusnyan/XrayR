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

## Generate

```bash
XrayR config init
```

For automation:

```bash
XrayR config init \
  --panel Xboard \
  --api-host https://panel.example.com \
  --api-key "$API_KEY" \
  --node-id 1 \
  --node-type Vless \
  --output /etc/XrayR/config.yml
```

## Diagnose

```bash
XrayR doctor -c config.yml --timeout 5s
```
