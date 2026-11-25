# XrayR

一个基于 [Xray-core](https://github.com/XTLS/Xray-core) 的后端节点，对接 **Xboard** 面板。

> 本版本只支持 Xboard 一个面板。`Xboard` / `NewV2board` / `V2board` 三个名称等价，写其他面板名会在启动和 `config check` 时直接报错并提示最接近的候选名。

> 维护说明：因个人习惯使用 XrayR，我会继续维护此项目，但不保证更新频率。使用中遇到问题请联系 <ModusNyan@gmail.com>，我会尽力修复。

## 快速开始

Debian 12+ 一键安装（**不会自动启动服务**）：

```bash
curl -fsSL https://raw.githubusercontent.com/Activity163/XrayR/master/release/install.sh -o install.sh
sudo bash install.sh
```

不带参数运行会打开管理菜单：

```
  1) 安装 / 重装（二进制 + systemd）      7) 查看日志
  2) 安装 / 重装（Docker）                8) 编辑配置
  3) 启动                                9) 校验配置
  4) 停止                               10) 更新到最新版本
  5) 重启                               11) 开机自启 开 / 关
  6) 查看状态                           12) 卸载
```

装完只需要四步：

```bash
xrayr edit      # 改 4 个字段：ApiHost / ApiKey / NodeID / NodeType
xrayr check     # 校验配置
xrayr start     # 启动（会等服务稳定运行，失败时直接打印日志）
xrayr status    # 查看状态
```

安装时生成的 `/etc/XrayR/config.yml` 是一份**带完整注释的模板**：只有 4 个字段必填，其余全部注释掉，不用来回查文档。

## 命令

`/usr/local/bin` 下会装两个名字：**`xrayr` 和 `XrayR`，它们是同一个管理脚本，大小写不敏感**。

```bash
xrayr menu        # 打开管理面板，XrayR menu 效果一样
```

管理子命令：`menu` `install` `docker` `start` `stop` `restart` `status` `logs` `edit` `check` `update` `enable` `disable` `uninstall`。

**不是管理子命令的参数会原样转交给服务二进制**，所以这些也能直接用：

```bash
xrayr config check -c /etc/XrayR/config.yml
xrayr config show
xrayr doctor
xrayr version
```

服务二进制本体放在 `/usr/local/lib/xrayr/XrayR`（**不在 PATH 上**，否则会和上面两个名字冲突），systemd 就是直接调它：

```
ExecStart=/usr/local/lib/xrayr/XrayR -c /etc/XrayR/config.yml
```

它的子命令：不带子命令即启动服务；`config check` / `config show` / `config migrate` / `config init` / `doctor` / `version` / `x25519`。

> 不带 `-c` 运行时，会先找 `/etc/XrayR/config.yml`，找不到才退回当前目录的 `config.yml`。

## 系统要求

- **系统**：Debian 12 (bookworm) 或更新。一键脚本面向 Debian，其他发行版可从源码编译
- **架构**：amd64 / arm64 / armv7 / armv6 / 386 / riscv64 / s390x / ppc64le
- **源码编译**：Go 1.25 或更高（一键脚本会自行安装 1.25.3）

## 功能

| 功能             | 说明                                                  |
| -------------- | --------------------------------------------------- |
| 节点 / 用户同步      | 周期性从面板拉取节点配置与用户列表；用 ETag 做增量，面板返回 304 时跳过           |
| 流量统计与上报        | 按用户统计上下行并回传面板                                       |
| 在线设备统计与限制      | 按用户限制同时在线 IP 数（每个上报周期重新计数）；支持 Redis 全局设备限制          |
| **状态回传面板**     | 负载（CPU / 内存 / 磁盘）、在线用户、同步状态上报到 Xboard 后台节点列表              |
| **跟随面板启停**     | 节点在面板被关闭后立即停止服务，重新启用后自动恢复                           |
| 限速             | 节点级 / 用户级限速；支持超速自动限速（可设警告次数与限速时长）                   |
| 审计规则           | 面板下发（仅 `block` 动作）或本地规则文件；命中即拒绝连接                     |
| TLS 证书         | ACME `http` / `tls` / `dns` 自动申请与续签；证书剩余天数有指标；与面板证书配置不一致时告警 |
| 自定义 DNS / 路由   | `dns.json` / `route.json` / 自定义 inbound-outbound    |
| **配置启动前完整校验**  | 一次返回全部问题，错误信息带字段路径与修复建议                             |
| **配置热重载**      | 修改配置文件自动生效；新配置无效或启动失败时保留上一份可用配置                     |
| **面板不可用时仍可启动** | 本地快照缓存恢复上一份有效配置                                     |
| **核心心跳自愈**     | 探测节点监听端口；连续 3 次失败即退出，交给 systemd 重启                  |
| **内置可观测性**     | Prometheus 指标 + `/healthz` `/readyz` `/status` 诊断端点 |

## 支持的协议与面板

| `PanelType`                           | 协议                                             |
| ------------------------------------- | ---------------------------------------------- |
| `Xboard`（别名 `NewV2board` / `V2board`） | `V2ray`、`Vmess`、`Vless`、`Trojan`、`Shadowsocks` |

- **传输层**：`tcp`、`websocket`、`grpc`、`httpupgrade`、`splithttp` / `xhttp`（含面板下发的 `mode` / `extra`）
- **安全层**：`none`、`tls`、`reality`（REALITY 的 `ws` / `httpupgrade` 组合不可用）
- **Shadowsocks**：`aes-128-gcm`、`aes-256-gcm`、`chacha20-ietf-poly1305`、`2022-blake3-aes-128-gcm`、`2022-blake3-aes-256-gcm`；
  插件只支持 simple-obfs 的 `http` 模式，其他插件或 `obfs=tls` 会让节点启动失败
- **VLESS flow**：留空或 `xtls-rprx-vision`（仅 `tcp`，且需要 TLS / REALITY），以面板配置为准
- 不支持：`h2` / `kcp` / `quic` 传输、sing-box 多路复用、ECH。对应的 Xboard 已在保存节点时拦截
- 面板名与协议名大小写不敏感。详见 [docs/panels.md](docs/panels.md) 与 [Xboard 对接文档](docs/dui-jie-xboard/xboard.md)

## 安装方式

### 一键安装（Debian）

见上方 [快速开始](#快速开始)。脚本支持非交互参数（自动化用）：

```bash
bash install.sh install          # 安装（二进制 + systemd），不启动
bash install.sh docker           # Docker 安装
bash install.sh update           # 更新到最新版本
bash install.sh uninstall        # 卸载（--purge 同时删除配置目录）

--release [TAG]   使用预编译归档而不是从源码编译
--build           强制从源码编译
--source-dir DIR  用已有 checkout 编译
--ref REF         克隆的 git ref（默认 master）
--skip-rules      不下载 geoip.dat / geosite.dat
--skip-service    不安装 systemd 单元
--purge           卸载时同时删除配置目录
```

`bash install.sh help` 可查看完整说明。

### Docker

菜单第 2 项（或 `xrayr docker`）会自动拉镜像、生成配置、挂载规则数据并启动容器：

```bash
docker run -d --name xrayr --network host --restart unless-stopped \
  -v /etc/XrayR/config.yml:/etc/XrayR/config.yml:ro \
  -v /etc/XrayR/cache:/etc/XrayR/cache \
  ghcr.io/activity163/xrayr:latest
```

镜像内已内置 `geoip.dat` / `geosite.dat`，无需额外下载。

### 从源码编译

```bash
git clone https://github.com/Activity163/XrayR.git
cd XrayR
go build -trimpath -ldflags "-s -w" -o XrayR .
```

### 规则数据（geoip.dat / geosite.dat）

`geoip:` / `geosite:` 路由规则依赖这两个文件。它们**不在本仓库**（合计约 26 MB 且持续增长），每次安装都会从  
[Loyalsoldier/v2ray-rules-dat](https://github.com/Loyalsoldier/v2ray-rules-dat) 的最新 release 下载并校验 sha256：

```bash
bash release/download-rules-dat.sh /etc/XrayR
```

这两个文件**必须与 `config.yml` 放在同一目录**（XrayR 通过 `XRAY_LOCATION_ASSET` 在该目录查找规则数据）。

## 配置

配置是带版本号的 YAML（`ConfigVersion: 1`）。**未知字段会被拒绝，而不是静默忽略** —— 拼错的字段会在启动时直接报错。

| 模板                                  | 用途                                             |
| ----------------------------------- | ---------------------------------------------- |
| `/etc/XrayR/config.yml`             | 安装时由 `config.template.yml` 生成，带完整注释，改 4 个字段即可跑 |
| `release/config/config.minimal.yml` | 首次运行必须修改的最小字段                                  |
| `release/config/config.yml.example` | 常用功能 + 注释，适合大多数用户                              |
| `release/config/config.full.yml`    | 全部高级字段与可选值                                     |

不想手改也可以交互式生成：`XrayR config init`。

### 校验与体检

```bash
xrayr check              # 等价于 XrayR config check -c /etc/XrayR/config.yml
xrayr edit               # 编辑配置后会自动校验
XrayR doctor             # 只读体检：配置 / DNS / TCP / TLS / 面板 API / Redis / 端口占用
XrayR config show        # 显示归一化后的最终配置（敏感字段已脱敏）
XrayR config migrate     # 迁移旧版配置到当前版本
```

`config check` 是纯本地静态校验，已被 systemd 单元用作 `ExecStartPre`，所以配置有问题时服务根本起不来，而不是起来后崩。

## 运维

### 常用命令

```bash
xrayr status             # 运行状态
xrayr logs 200           # 查看最近 200 行日志
xrayr restart
xrayr update             # 更新到最新版本
xrayr enable|disable     # 开机自启 开 / 关
xrayr menu               # 重新打开管理菜单
```

### 可观测性

诊断端点默认关闭，需要显式开启：

```yaml
Diagnostics:
  Enable: true
  Listen: 127.0.0.1:8080
```

- `/healthz` 进程存活
- `/readyz` 所有节点已初始化
- `/status` 脱敏后的节点状态
- `/metrics` Prometheus 指标

指标包括面板请求成功率、用户数、流量上报失败数、配置重载结果、**证书剩余天数**、**核心健康状态**。  
完整清单见 [docs/observability.md](docs/observability.md)。

`/status` 的 `last_error` 显示最近一次同步问题：面板下发的配置被拒绝时，节点继续用上一份有效配置运行，
这条错误会一直保留到面板换了配置为止；节点被面板关闭时为 `node disabled by panel`。
同样的信息会通过 `UniProxy/status` 上报，在 Xboard 节点列表里显示为黄点。

### 核心心跳与自愈

xray-core 没有暴露存活标志，所以 XrayR 每个 `UpdatePeriodic` 会拨号一次自己的监听端口来确认核心仍在服务。  
连续 3 次失败即退出进程，交给 systemd（`Restart=always`）重启 —— 这是唯一安全的自愈方式，  
因为进程内重建 core 会让所有 feature 句柄失效。

如果不想让它在核心无响应时退出，设 `DisableCoreHeartbeat: true`。

### 卸载

```bash
xrayr uninstall            # 保留 /etc/XrayR
xrayr uninstall --purge    # 同时删除配置目录
```

## 开发

```bash
gofmt -l .                                  # 必须为空
go build ./... && go vet ./...
golangci-lint run --max-issues-per-linter 0 --max-same-issues 0
go test -race ./...
go test -run '^$' -tags=integration ./...   # integration 测试只编译不执行
bash tools/e2e/run-e2e.sh                   # 端到端：假面板 → 节点 → 真客户端 → 真流量
```

`tools/e2e/run-e2e.sh` 不需要真实面板，也不需要外网：它起一个 Xboard 桩面板，用真 xray-core 客户端把流量打穿节点  
（`curl → socks → VMess/WS → 节点 → freedom → 本地 HTTP 服务`），并验证面板改端口时节点会平滑迁移、  
下发无法应用的配置时会回滚。CI 每次都会跑。

`integration` 标签的测试会打真实面板并硬编码凭据，**只编译不执行**。

## 文档

| 文档                                                 | 内容        |
| -------------------------------------------------- | --------- |
| [docs/index.md](docs/index.md)                     | 文档索引      |
| [docs/installation.md](docs/installation.md)       | 安装        |
| [docs/configuration.md](docs/configuration.md)     | 配置        |
| [docs/cli.md](docs/cli.md)                         | 命令行       |
| [docs/diagnostics.md](docs/diagnostics.md)         | 诊断        |
| [docs/observability.md](docs/observability.md)     | 可观测性      |
| [docs/migration.md](docs/migration.md)             | 迁移与回滚     |
| [docs/troubleshooting.md](docs/troubleshooting.md) | 故障排查      |
| [docs/panels.md](docs/panels.md)                   | 面板与协议支持范围 |

## 免责声明

本项目只是个人学习开发并维护，不保证任何可用性，也不对使用本软件造成的任何后果负责。

## Thanks

- [Project X](https://github.com/XTLS/)
- [V2Fly](https://github.com/v2fly)
- [VNet-V2ray](https://github.com/ProxyPanel/VNet-V2ray)
- [Air-Universe](https://github.com/crossfw/Air-Universe)
- [Loyalsoldier/v2ray-rules-dat](https://github.com/Loyalsoldier/v2ray-rules-dat)

## Licence

[Mozilla Public License Version 2.0](LICENSE)
