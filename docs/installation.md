# Installation

## 一键安装（Debian）

```bash
curl -fsSL https://raw.githubusercontent.com/Activity163/XrayR/master/release/install.sh -o install.sh
sudo bash install.sh
```

不带参数运行会打开**管理菜单**：

```
XrayR 管理菜单
  安装方式: 未安装 | 运行状态: 未安装
  配置文件: /etc/XrayR/config.yml
----------------------------------------------------------------
  1) 安装 / 重装（二进制 + systemd）
  2) 安装 / 重装（Docker）
  3) 启动
  4) 停止
  5) 重启
  6) 查看状态
  7) 查看日志
  8) 编辑配置
  9) 校验配置
 10) 更新到最新版本
 11) 开机自启 开 / 关
 12) 卸载
  0) 退出
----------------------------------------------------------------
```

安装完成后脚本会把自身装成 `xrayr` 命令，之后随时 `xrayr menu` 打开菜单，
也可以直接用子命令：

```bash
xrayr install          # 安装（二进制 + systemd）
xrayr docker           # Docker 安装
xrayr start|stop|restart|status
xrayr logs [行数]      # 跟随日志，Ctrl-C 退出
xrayr edit             # 编辑配置
xrayr check            # 校验配置
xrayr update           # 更新到最新版本
xrayr enable|disable   # 开机自启 开 / 关
xrayr uninstall [--purge]
```

**安装不会自动启动服务。** 装完后先改配置、再校验、最后启动：

```bash
xrayr edit      # 改配置里的 ApiHost / ApiKey / NodeID / NodeType
xrayr check
xrayr start
```

`xrayr start` 会等服务稳定运行后才报成功：systemd 单元带 `Restart=always`，
配置不对时进程会退出并被反复拉起，只看一次 `is-active` 会把崩溃循环误判成启动成功。
启动失败时会直接把最近的日志打出来。

## 配置

安装时生成 `/etc/XrayR/config.yml`（权限 `0600`），**是一份带完整注释的模板**。
只需要改第 1 个节点块里的 4 个字段：

```yaml
  - PanelType: "Xboard"                     # 面板类型
    ApiConfig:
      ApiHost: "https://panel.example.com"  # ← 改这里：面板地址
      ApiKey: "CHANGE_ME"                   # ← 改这里：节点密钥
      NodeID: 1                             # ← 改这里：节点 ID
      NodeType: Vmess                       # ← 改这里：协议
```

这 4 个值都在面板后台的「节点」页面里。文件里同时列出了可用的面板类型、协议，
以及常用的可选配置（默认注释掉，需要时去掉 `#` 即可）。

`xrayr check` 会校验配置；如果还是示例值（`panel.example.com` / `CHANGE_ME`），
会给出警告并指出是哪个字段。完整字段说明见
[config.full.yml](https://github.com/Activity163/XrayR/blob/master/release/config/config.full.yml)。

> 也保留交互式生成：`XrayR config init`。默认流程是手改这份带注释的模板 ——
> 字段含义都写在文件里，不需要来回查文档。

## Docker 安装

菜单第 2 项（或 `xrayr docker`）会：

1. 生成 `/etc/XrayR/config.yml` 和规则数据；
2. 拉取镜像（默认 `ghcr.io/activity163/xrayr:latest`，可用 `XRAYR_IMAGE` 覆盖）；
3. 以 `--restart unless-stopped`（即开机自启）、`--network host` 启动容器，
   挂载配置、规则数据和 `cache/` 目录。

之后 `xrayr start/stop/logs/status` 会自动走 `docker` 而不是 systemd。

拉取镜像失败时（例如镜像还没发布）会提示改用二进制安装，或自行构建：

```bash
docker build -t ghcr.io/activity163/xrayr:latest .
```

## 更新

`xrayr update`：

- 二进制安装：重新下载（或编译）最新版本，替换二进制和 systemd 单元，
  服务原本在运行会自动停掉再拉起来；
- Docker 安装：重新拉取镜像并重建容器。

## 规则数据（geoip.dat / geosite.dat）

`geoip:` / `geosite:` 路由规则依赖这两个文件。它们**不在仓库里**（合计约 26 MB 且持续增长），
安装时会从
[Loyalsoldier/v2ray-rules-dat](https://github.com/Loyalsoldier/v2ray-rules-dat)
的最新 release 下载并校验 sha256：

```bash
bash release/download-rules-dat.sh /etc/XrayR
```

必须和 `config.yml` 放在同一目录（XrayR 通过 `XRAY_LOCATION_ASSET` 在该目录查找）。
单独刷新规则数据后 `xrayr restart` 即可生效，不用动二进制。

## 手动安装

不想用脚本时：

```bash
sudo install -d /usr/local/lib/xrayr
sudo install -m 0755 XrayR /usr/local/lib/xrayr/XrayR
sudo install -d /usr/local/bin
sudo install -m 0755 release/install.sh /usr/local/bin/xrayr
sudo install -m 0755 release/install.sh /usr/local/bin/XrayR
sudo mkdir -p /etc/XrayR/cache
sudo install -m 0644 release/systemd/XrayR.service /etc/systemd/system/XrayR.service
sudo install -m 0600 release/config/config.template.yml /etc/XrayR/config.yml
sudo bash release/download-rules-dat.sh /etc/XrayR
# 改配置
xrayr check
sudo systemctl enable --now XrayR
```

服务二进制放在 `/usr/local/lib/xrayr/`，不在 PATH 上：`/usr/local/bin` 里的 `xrayr` 与
`XrayR` 都是管理脚本，两个名字大小写不敏感，都会把非管理子命令转发给真正的二进制。

`/etc/XrayR/cache` 是必需的：`Cache` 默认开启，面板临时不可用时靠它恢复上一份有效配置。

`ExecStartPre` 会在每次启动前跑一次 `config check`，配置有错就拒绝启动，
而不是留下一个半死不活的节点。

## 非交互参数

自动化场景可以完全绕过菜单：

| 参数 | 作用 |
|------|------|
| `--release [TAG]` | 使用预编译归档而不是从源码编译 |
| `--build` | 强制从源码编译 |
| `--source-dir DIR` | 用已有 checkout 编译 |
| `--ref REF` | 克隆的 git ref（默认 master） |
| `--go-version VER` | 缺失时安装的 Go 版本（默认 1.25.3） |
| `--install-dir DIR` / `--config-dir DIR` | 自定义安装 / 配置目录 |
| `--skip-rules` | 不下载规则数据 |
| `--skip-service` | 不安装 systemd 单元 |
| `--purge` | 卸载时同时删除配置目录 |

例如 CI 或无人值守部署：

```bash
sudo bash install.sh install --source-dir /root/XrayR --skip-rules
```
