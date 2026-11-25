# Build go
FROM golang:1.25.3-alpine AS builder
WORKDIR /app
COPY . .
ENV CGO_ENABLED=0
RUN go mod download
RUN go test ./...
RUN go run . config check -c release/config/config.minimal.yml
RUN go run . config check -c release/config/config.yml.example
RUN go run . config check -c release/config/config.full.yml
RUN go build -v -o XrayR -trimpath -ldflags "-s -w -buildid="

# Rule data (geoip.dat / geosite.dat) is not stored in the repository: fetch the
# latest release on every build and verify the published checksum.
RUN apk --update --no-cache add curl bash \
    && bash release/download-rules-dat.sh /app/rules

# Release
FROM  alpine
# 安装必要的工具包
RUN  apk --update --no-cache add tzdata ca-certificates \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
RUN mkdir /etc/XrayR/
COPY --from=builder /app/XrayR /usr/local/bin
# xray-core resolves geoip:/geosite: rules through XRAY_LOCATION_ASSET, which the
# binary points at the directory holding config.yml.
COPY --from=builder /app/rules/geoip.dat /app/rules/geosite.dat /etc/XrayR/

ENTRYPOINT [ "XrayR", "--config", "/etc/XrayR/config.yml"]
