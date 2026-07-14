# Installation

## Binary

Place the binary at `/usr/local/bin/XrayR`, create `/etc/XrayR/config.yml`, then install the provided systemd unit from `release/systemd/XrayR.service`.

```bash
XrayR config check -c /etc/XrayR/config.yml
sudo systemctl enable --now XrayR
```

## Docker

Mount the configuration read-only and persist the cache directory:

```yaml
services:
  xrayr:
    image: ghcr.io/xrayr-project/xrayr:latest
    restart: unless-stopped
    volumes:
      - ./config.yml:/etc/XrayR/config.yml:ro
      - ./cache:/etc/XrayR/cache
    network_mode: host
```

Run `XrayR config check` before starting a replacement container.
