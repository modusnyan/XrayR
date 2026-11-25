#!/usr/bin/env bash
#
# XrayR installer and management menu for Debian.
#
#   bash install.sh                 # 交互式菜单（推荐）
#   bash install.sh menu            # 同上；安装后可直接用 `xrayr menu`
#   bash install.sh install         # 直接安装（二进制 + systemd），不启动
#   bash install.sh docker          # Docker 安装
#   bash install.sh start|stop|restart|status|logs
#   bash install.sh update          # 更新到最新版本
#   bash install.sh enable|disable  # 开机自启 开 / 关
#   bash install.sh check|edit      # 校验 / 编辑配置
#   bash install.sh uninstall [--purge]
#
# 安装完成后会把自身装成 `xrayr` 命令，之后直接 `xrayr menu` 即可。
set -euo pipefail

REPO_SLUG="${XRAYR_REPO:-Activity163/XrayR}"
REF="${XRAYR_REF:-master}"
RAW_BASE="${XRAYR_RAW_BASE:-https://raw.githubusercontent.com/${REPO_SLUG}/${REF}}"
RELEASE_BASE="${XRAYR_RELEASE_BASE:-https://github.com/${REPO_SLUG}/releases}"
GO_VERSION="${XRAYR_GO_VERSION:-1.25.3}"
GO_ROOT="${XRAYR_GO_ROOT:-/usr/local/go}"

INSTALL_DIR="/usr/local/bin"
LIB_DIR="/usr/local/lib/xrayr"
CONFIG_DIR="/etc/XrayR"
SERVICE_PATH="/etc/systemd/system/XrayR.service"
SERVICE_NAME="XrayR"
# The service binary does not live on PATH: both `xrayr` and `XrayR` are the
# management command, so typing either spelling opens the menu on a
# case-sensitive filesystem.
SERVICE_BIN="${LIB_DIR}/XrayR"
MANAGER_NAMES="xrayr XrayR"
SELF_PATH="/usr/local/bin/xrayr"
CONTAINER_NAME="xrayr"
DOCKER_IMAGE="${XRAYR_IMAGE:-ghcr.io/$(printf '%s' "${REPO_SLUG%%/*}" | tr '[:upper:]' '[:lower:]')/xrayr:latest}"
CONFIG_FILE="${CONFIG_DIR}/config.yml"

MODE="auto"          # auto | release | source
RELEASE_TAG="latest"
SOURCE_DIR=""
SKIP_RULES=0
SKIP_SERVICE=0
NO_CONFIG=0
PURGE=0

WORKDIR=""
LOCAL_ROOT=""
DISTRO=""
ASSET=""
GO_ARCH=""
HAS_SYSTEMD=0
HAS_DOCKER=0
INSTALLED_MODE=""

if [ -t 1 ] && [ "${NO_COLOR:-}" = "" ]; then
    C_RESET=$'\033[0m'; C_BOLD=$'\033[1m'; C_DIM=$'\033[2m'
    C_RED=$'\033[31m'; C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'
    C_BLUE=$'\033[34m'; C_CYAN=$'\033[36m'
else
    C_RESET=""; C_BOLD=""; C_DIM=""
    C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_CYAN=""
fi

log()  { printf '%s==>%s %s\n' "$C_BLUE" "$C_RESET" "$*"; }
ok()   { printf '%s  ok%s %s\n' "$C_GREEN" "$C_RESET" "$*"; }
warn() { printf '%swarn%s %s\n' "$C_YELLOW" "$C_RESET" "$*" >&2; }
die()  { printf '%serror%s %s\n' "$C_RED" "$C_RESET" "$*" >&2; exit 1; }

cleanup() { [ -n "$WORKDIR" ] && [ -d "$WORKDIR" ] && rm -rf "$WORKDIR"; return 0; }
trap cleanup EXIT

usage() {
    cat <<'EOF'
XrayR 安装与管理脚本（Debian）

  bash install.sh                 交互式菜单（推荐）
  bash install.sh install         安装（二进制 + systemd），不启动
  bash install.sh docker          Docker 安装
  bash install.sh start|stop|restart|status|logs
  bash install.sh update          更新到最新版本
  bash install.sh enable|disable  开机自启 开 / 关
  bash install.sh check|edit      校验 / 编辑配置
  bash install.sh uninstall       卸载（--purge 同时删除配置目录）

非交互参数（自动化用）：
  --release [TAG]   使用预编译归档而不是从源码编译
  --build           强制从源码编译
  --source-dir DIR  用已有 checkout 编译
  --ref REF         克隆的 git ref（默认 master）
  --go-version VER  缺失时安装的 Go 版本（默认 1.25.3）
  --install-dir DIR 管理命令目录（默认 /usr/local/bin）
  --lib-dir DIR     服务二进制目录（默认 /usr/local/lib/xrayr）
  --config-dir DIR  配置目录（默认 /etc/XrayR）
  --skip-rules      不下载 geoip.dat / geosite.dat
  --skip-service    不安装 systemd 单元
  --purge           卸载时同时删除配置目录

不是管理子命令的参数会原样转交给服务二进制，所以
`xrayr config check`、`xrayr doctor`、`xrayr version`、`xrayr -c FILE` 都能直接用。
EOF
}

# ------------------------------------------------------------------- environment

detect_platform() {
    [ -f /etc/os-release ] || die "找不到 /etc/os-release；本脚本面向 Debian"
    # shellcheck disable=SC1091
    . /etc/os-release
    case "${ID:-}" in
        debian|ubuntu|raspbian) ;;
        *) warn "检测到 ${ID:-未知}，不是 Debian，仍会继续" ;;
    esac
    DISTRO="${PRETTY_NAME:-unknown}"

    case "$(uname -m)" in
        x86_64|amd64)   GO_ARCH="amd64";   ASSET="linux-64" ;;
        aarch64|arm64)  GO_ARCH="arm64";   ASSET="linux-arm64-v8a" ;;
        armv7l|armv7)   GO_ARCH="armv6l";  ASSET="linux-arm32-v7a" ;;
        armv6l|armv6)   GO_ARCH="armv6l";  ASSET="linux-arm32-v6" ;;
        i386|i686)      GO_ARCH="386";     ASSET="linux-32" ;;
        riscv64)        GO_ARCH="riscv64"; ASSET="linux-riscv64" ;;
        s390x)          GO_ARCH="s390x";   ASSET="linux-s390x" ;;
        ppc64le)        GO_ARCH="ppc64le"; ASSET="linux-ppc64le" ;;
        *) die "不支持的架构 $(uname -m)" ;;
    esac

    if [ -d /run/systemd/system ]; then HAS_SYSTEMD=1; fi
    if command -v docker >/dev/null 2>&1; then HAS_DOCKER=1; fi

    # 当前是怎么装的？决定 start/stop/logs 走 systemd 还是 docker
    if [ "$HAS_DOCKER" -eq 1 ] && docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx "$CONTAINER_NAME"; then
        INSTALLED_MODE="docker"
    elif [ -x "$SERVICE_BIN" ]; then
        INSTALLED_MODE="binary"
    else
        INSTALLED_MODE=""
    fi
}

apt_install() {
    local missing=() pkg
    for pkg in "$@"; do
        dpkg -s "$pkg" >/dev/null 2>&1 || missing+=("$pkg")
    done
    [ ${#missing[@]} -eq 0 ] && return 0
    command -v apt-get >/dev/null 2>&1 || die "缺少 ${missing[*]}，且没有 apt-get"
    log "安装依赖：${missing[*]}"
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq --no-install-recommends "${missing[@]}"
}

# ------------------------------------------------------------------ go toolchain

go_is_usable() {
    [ -x "$1" ] || return 1
    "$1" version 2>/dev/null | awk '{print $3}' | grep -q "^go${GO_VERSION}$"
}

ensure_go() {
    if go_is_usable "${GO_ROOT}/bin/go"; then
        export PATH="${GO_ROOT}/bin:${PATH}"; ok "使用已有的 $(go version)"; return 0
    fi
    if command -v go >/dev/null 2>&1 && go_is_usable "$(command -v go)"; then
        ok "使用已有的 $(go version)"; return 0
    fi
    apt_install curl ca-certificates tar
    local url="https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
    log "安装 Go ${GO_VERSION}"
    curl -fsSL --retry 3 --retry-delay 2 -o "${WORKDIR}/go.tar.gz" "$url" ||
        die "下载 Go 失败；可用 --go-version 指定其他版本"
    rm -rf "$GO_ROOT"
    mkdir -p "$(dirname "$GO_ROOT")"
    tar -C "$(dirname "$GO_ROOT")" -xzf "${WORKDIR}/go.tar.gz"
    cat >/etc/profile.d/go.sh <<EOF
export GOROOT=${GO_ROOT}
export GOPATH=/root/go
export PATH=\$PATH:${GO_ROOT}/bin:/root/go/bin
export GOPROXY=https://goproxy.cn,direct
EOF
    chmod 0644 /etc/profile.d/go.sh
    export PATH="${GO_ROOT}/bin:${PATH}"
    ok "已安装 $(go version)"
}

# ------------------------------------------------------------------- acquisition

fetch_repo_file() { # fetch_repo_file <relative-path> <destination>
    if [ -n "$LOCAL_ROOT" ] && [ -f "${LOCAL_ROOT}/${1}" ]; then
        cp "${LOCAL_ROOT}/${1}" "$2"
    else
        curl -fsSL --retry 3 --retry-delay 2 -o "$2" "${RAW_BASE}/${1}"
    fi
}

release_asset_url() {
    if [ "$1" = "latest" ]; then
        printf '%s/latest/download/XrayR-%s.zip' "$RELEASE_BASE" "$ASSET"
    else
        printf '%s/download/%s/XrayR-%s.zip' "$RELEASE_BASE" "$1" "$ASSET"
    fi
}

release_exists() { curl -fsSLI -o /dev/null --max-time 20 "$(release_asset_url "$RELEASE_TAG")" 2>/dev/null; }

install_binary_file() {
    mkdir -p "$LIB_DIR"
    install -m 0755 "$1" "$SERVICE_BIN"
    ok "已安装服务二进制 ${SERVICE_BIN}（$("$SERVICE_BIN" version 2>/dev/null || echo '版本检查失败')）"
}

copy_support_files() { # copy_support_files <source-dir>
    local dir="$1" file
    # cache/ 是必须的：Cache 默认开启，面板不可用时靠它恢复上一份有效配置
    mkdir -p "$CONFIG_DIR" "${CONFIG_DIR}/cache"
    chmod 0700 "$CONFIG_DIR"
    for file in dns.json route.json custom_inbound.json custom_outbound.json rulelist geoip.dat geosite.dat; do
        if [ -f "${dir}/${file}" ] && [ ! -f "${CONFIG_DIR}/${file}" ]; then
            install -m 0644 "${dir}/${file}" "${CONFIG_DIR}/${file}"
        fi
    done
}

install_from_release() {
    apt_install curl ca-certificates unzip
    local url archive
    url="$(release_asset_url "$RELEASE_TAG")"
    archive="${WORKDIR}/XrayR.zip"
    log "下载 ${url}"
    curl -fsSL --retry 3 --retry-delay 2 -o "$archive" "$url" ||
        die "下载 release 归档失败（tag=${RELEASE_TAG}）"
    mkdir -p "${WORKDIR}/archive"
    unzip -q -o "$archive" -d "${WORKDIR}/archive"
    [ -f "${WORKDIR}/archive/XrayR" ] || die "归档里没有 XrayR 二进制"
    install_binary_file "${WORKDIR}/archive/XrayR"
    copy_support_files "${WORKDIR}/archive"
}

install_from_source() {
    apt_install curl ca-certificates tar git
    ensure_go
    local src="$SOURCE_DIR"
    if [ -z "$src" ]; then
        src="${WORKDIR}/src"
        log "克隆 ${REPO_SLUG}（${REF}）"
        git clone --quiet --depth 1 --branch "$REF" "https://github.com/${REPO_SLUG}.git" "$src" ||
            die "克隆失败；可用 --source-dir 指定已有 checkout"
    else
        [ -f "${src}/go.mod" ] || die "${src} 不像 XrayR 仓库（没有 go.mod）"
        log "从 ${src} 编译"
    fi
    ( cd "$src" && GOFLAGS="-trimpath" CGO_ENABLED=0 go build -ldflags "-s -w" -o "${WORKDIR}/XrayR" . ) ||
        die "go build 失败"
    install_binary_file "${WORKDIR}/XrayR"
    copy_support_files "${src}/release/config"
    LOCAL_ROOT="$src"
}

# ------------------------------------------------------------------- config file

write_config_template() { # 不覆盖已有配置
    if [ -f "$CONFIG_FILE" ]; then
        ok "保留已有配置 ${CONFIG_FILE}"
        return 0
    fi
    fetch_repo_file "release/config/config.template.yml" "${WORKDIR}/config.template.yml" ||
        die "获取配置模板失败"
    install -m 0600 "${WORKDIR}/config.template.yml" "$CONFIG_FILE"
    ok "已生成带注释的配置模板 ${CONFIG_FILE}"
}

config_is_placeholder() {
    [ -f "$CONFIG_FILE" ] && grep -qE 'panel\.example\.com|CHANGE_ME' "$CONFIG_FILE"
}

# ----------------------------------------------------------------------- install

install_rules() {
    local script="${WORKDIR}/download-rules-dat.sh"
    log "下载 geoip.dat / geosite.dat"
    fetch_repo_file "release/download-rules-dat.sh" "$script" || die "获取规则下载脚本失败"
    bash "$script" "$CONFIG_DIR"
    ok "规则数据已就位（${CONFIG_DIR}）"
}

install_service_unit() {
    local unit="${WORKDIR}/XrayR.service"
    fetch_repo_file "release/systemd/XrayR.service" "$unit" || die "获取 systemd 单元失败"
    if [ "$CONFIG_DIR" != "/etc/XrayR" ] || [ "$SERVICE_BIN" != "/usr/local/lib/xrayr/XrayR" ]; then
        sed -i -e "s#/etc/XrayR#${CONFIG_DIR}#g" -e "s#/usr/local/lib/xrayr/XrayR#${SERVICE_BIN}#g" "$unit"
    fi
    install -m 0644 "$unit" "$SERVICE_PATH"
    if [ "$HAS_SYSTEMD" -eq 1 ]; then
        systemctl daemon-reload
        ok "已安装 ${SERVICE_PATH}"
    else
        warn "已写入 ${SERVICE_PATH}，但 systemd 未运行"
    fi
}

# install_manager installs the management command under every name it answers to.
# When any directory was overridden the defaults are baked out of the copy, so the
# installed script keeps pointing at the right places instead of falling back to
# /usr/local/lib/xrayr.
install_manager() { # install_manager <script>
    local src="$1" name
    mkdir -p "$INSTALL_DIR"
    for name in $MANAGER_NAMES; do
        if [ "$INSTALL_DIR" = "/usr/local/bin" ] && [ "$LIB_DIR" = "/usr/local/lib/xrayr" ] && [ "$CONFIG_DIR" = "/etc/XrayR" ]; then
            install -m 0755 "$src" "${INSTALL_DIR}/${name}"
        else
            sed -e "s#^LIB_DIR=\"/usr/local/lib/xrayr\"#LIB_DIR=\"${LIB_DIR}\"#" \
                -e "s#^INSTALL_DIR=\"/usr/local/bin\"#INSTALL_DIR=\"${INSTALL_DIR}\"#" \
                -e "s#^CONFIG_DIR=\"/etc/XrayR\"#CONFIG_DIR=\"${CONFIG_DIR}\"#" \
                -e "s#^SELF_PATH=\"/usr/local/bin/xrayr\"#SELF_PATH=\"${INSTALL_DIR}/xrayr\"#" \
                "$src" >"${INSTALL_DIR}/${name}"
            chmod 0755 "${INSTALL_DIR}/${name}"
        fi
    done
    ok "已安装管理命令 ${MANAGER_NAMES}（之后可直接运行 xrayr menu，大小写均可）"
}

install_self() {
    local src="${BASH_SOURCE[0]}"
    if [ -f "$src" ] && [ "$(cd -- "$(dirname -- "$src")" && pwd)/$(basename -- "$src")" != "$SELF_PATH" ]; then
        install_manager "$src"
    elif [ ! -f "$SELF_PATH" ]; then
        if fetch_repo_file "release/install.sh" "${WORKDIR}/self.sh" 2>/dev/null; then
            install_manager "${WORKDIR}/self.sh"
        fi
    fi
}

do_install() { # do_install [binary|docker]
    local target="${1:-binary}"
    detect_platform

    if [ "$target" = "docker" ]; then
        do_install_docker
        return
    fi

    if [ "$MODE" = "auto" ]; then
        if release_exists; then MODE="release"; log "找到 release 归档，直接使用"
        else MODE="source"; log "没有可用的 release 归档，改为从源码编译"
        fi
    fi
    if [ "$MODE" = "release" ]; then install_from_release; else install_from_source; fi

    if [ "$NO_CONFIG" -eq 0 ]; then write_config_template; fi
    if [ "$SKIP_RULES" -eq 0 ] && { [ ! -f "${CONFIG_DIR}/geoip.dat" ] || [ ! -f "${CONFIG_DIR}/geosite.dat" ]; }; then
        install_rules
    fi
    if [ "$SKIP_SERVICE" -eq 0 ]; then install_service_unit; fi
    install_self

    printf '\n'
    if config_is_placeholder; then
        warn "${CONFIG_FILE} 里还是示例值，启动前请先改成你自己的面板信息"
    fi
    cat <<EOF

${C_BOLD}安装完成，服务尚未启动${C_RESET}

  服务二进制  ${SERVICE_BIN}
  管理命令    ${INSTALL_DIR}/xrayr, ${INSTALL_DIR}/XrayR
  配置        ${CONFIG_FILE}
  规则        ${CONFIG_DIR}/geoip.dat, ${CONFIG_DIR}/geosite.dat
  单元        ${SERVICE_PATH}

下一步：
  xrayr edit     修改配置里的 ApiHost / ApiKey / NodeID / NodeType
  xrayr check    校验配置
  xrayr start    启动
  xrayr menu     打开管理菜单（XrayR 大小写均可）
EOF
}

do_install_docker() {
    if [ "$HAS_DOCKER" -eq 0 ]; then
        die "没有找到 docker。先安装：apt-get install -y docker.io"
    fi

    mkdir -p "$CONFIG_DIR" "${CONFIG_DIR}/cache"
    chmod 0700 "$CONFIG_DIR"
    write_config_template

    if [ "$SKIP_RULES" -eq 0 ] && { [ ! -f "${CONFIG_DIR}/geoip.dat" ] || [ ! -f "${CONFIG_DIR}/geosite.dat" ]; }; then
        install_rules
    fi

    log "拉取镜像 ${DOCKER_IMAGE}"
    if ! docker pull "$DOCKER_IMAGE"; then
        warn "拉取失败。可以改用二进制安装（菜单第 1 项），"
        warn "或先自行构建镜像：docker build -t ${DOCKER_IMAGE} ."
        return 1
    fi

    docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
    log "创建容器 ${CONTAINER_NAME}"
    docker run -d \
        --name "$CONTAINER_NAME" \
        --restart unless-stopped \
        --network host \
        -v "${CONFIG_FILE}:/etc/XrayR/config.yml:ro" \
        -v "${CONFIG_DIR}/cache:/etc/XrayR/cache" \
        -v "${CONFIG_DIR}/geoip.dat:/etc/XrayR/geoip.dat:ro" \
        -v "${CONFIG_DIR}/geosite.dat:/etc/XrayR/geosite.dat:ro" \
        "$DOCKER_IMAGE" >/dev/null
    ok "容器已创建（restart=unless-stopped，即开机自启）"

    printf '\n'
    if config_is_placeholder; then
        warn "${CONFIG_FILE} 里还是示例值，容器会反复重启，请先改配置再 xrayr restart"
    fi
    cat <<EOF

${C_BOLD}Docker 安装完成${C_RESET}

  容器   ${CONTAINER_NAME}（镜像 ${DOCKER_IMAGE}）
  配置   ${CONFIG_FILE}

下一步：
  xrayr edit     修改配置
  xrayr restart  重启容器使配置生效
  xrayr logs     查看日志
EOF
}

# ------------------------------------------------------------------------ manage

# manage_mode 只读 INSTALLED_MODE：detect_platform 在 main 里已经跑过。
# 放在子 shell 里调用 detect_platform 会让它设置的变量丢失。
manage_mode() {
    if [ "$INSTALLED_MODE" = "docker" ]; then printf 'docker'; else printf 'binary'; fi
}

# systemctl is-active / is-enabled 在失败时既输出状态又返回非零，
# 直接 `|| echo unknown` 会打印两行。用 show 取单一值。
service_state() { systemctl show -p ActiveState --value "$SERVICE_NAME" 2>/dev/null || true; }
service_enabled() { systemctl show -p UnitFileState --value "$SERVICE_NAME" 2>/dev/null || true; }

do_start() {
    case "$(manage_mode)" in
        docker)
            docker start "$CONTAINER_NAME" >/dev/null && ok "容器已启动" ;;
        binary)
            [ -f "$SERVICE_PATH" ] || die "没有找到 ${SERVICE_PATH}，请先安装"
            [ "$HAS_SYSTEMD" -eq 1 ] || die "systemd 未运行，无法启动服务"
            if config_is_placeholder; then
                warn "${CONFIG_FILE} 里还是示例值，服务可能连不上面板"
            fi
            local stable=0
            systemctl reset-failed "$SERVICE_NAME" 2>/dev/null || true
            systemctl start "$SERVICE_NAME" || return 1
            # unit 里有 Restart=always：配置不对时进程会退出并被反复拉起，
            # 只采样一次 is-active 会把崩溃循环误判成启动成功。要求连续 5 秒
            # 处于 active/running —— 崩溃循环每 5 秒（RestartSec）会掉出这个状态。
            for _ in $(seq 1 15); do
                sleep 1
                if [ "$(service_state)" = "active" ] &&
                   [ "$(systemctl show -p SubState --value "$SERVICE_NAME" 2>/dev/null)" = "running" ]; then
                    stable=$((stable + 1))
                    [ "$stable" -ge 5 ] && break
                else
                    stable=0
                fi
            done
            if [ "$stable" -lt 5 ]; then
                warn "服务未能稳定运行，最近的日志："
                journalctl -u "$SERVICE_NAME" --no-pager -n 25 >&2
                return 1
            fi
            ok "服务已启动并稳定运行" ;;
        *) die "尚未安装 XrayR，请先安装" ;;
    esac
}

do_stop() {
    case "$(manage_mode)" in
        docker) docker stop "$CONTAINER_NAME" >/dev/null && ok "容器已停止" ;;
        binary)
            [ "$HAS_SYSTEMD" -eq 1 ] || die "systemd 未运行"
            systemctl stop "$SERVICE_NAME" && ok "服务已停止" ;;
        *) die "尚未安装 XrayR" ;;
    esac
}

do_restart() { do_stop || true; do_start; }

do_status() {
    local mode state enabled
    mode="$(manage_mode)"
    printf '安装方式 : %s\n' "${mode:-未安装}"
    printf '发行版   : %s\n' "${DISTRO:-未知}"
    printf '架构     : %s\n' "${ASSET:-未知}"
    if [ "$mode" = "binary" ]; then
        if [ -x "$SERVICE_BIN" ]; then
            printf '二进制   : %s\n' "$("$SERVICE_BIN" version)"
        else
            printf '二进制   : 未安装\n'
        fi
        state="$(service_state)"; [ -n "$state" ] || state="unknown"
        enabled="$(service_enabled)"; [ -n "$enabled" ] || enabled="unknown"
        printf '运行状态 : %s\n' "$state"
        printf '开机自启 : %s\n' "$enabled"
    elif [ "$mode" = "docker" ]; then
        printf '容器     : %s\n' "$(docker inspect -f '{{.State.Status}} (restart={{.HostConfig.RestartPolicy.Name}})' "$CONTAINER_NAME" 2>/dev/null || echo unknown)"
        printf '镜像     : %s\n' "$(docker inspect -f '{{.Config.Image}}' "$CONTAINER_NAME" 2>/dev/null || echo unknown)"
    fi
    printf '配置文件 : %s\n' "$CONFIG_FILE"
    if config_is_placeholder; then
        printf '           ← 还是示例值，需要修改\n'
    fi
}

do_logs() {
    local lines="${1:-80}"
    case "$(manage_mode)" in
        docker) docker logs --tail "$lines" -f "$CONTAINER_NAME" ;;
        binary)
            [ "$HAS_SYSTEMD" -eq 1 ] || die "systemd 未运行"
            journalctl -u "$SERVICE_NAME" -n "$lines" -f ;;
        *) die "尚未安装 XrayR" ;;
    esac
}

do_edit() {
    [ -f "$CONFIG_FILE" ] || die "没有找到 ${CONFIG_FILE}，请先安装"
    local editor="${EDITOR:-}" candidate
    if [ -z "$editor" ]; then
        for candidate in nano vim vi; do
            if command -v "$candidate" >/dev/null 2>&1; then editor="$candidate"; break; fi
        done
    fi
    [ -n "$editor" ] || die "找不到编辑器；请设置 EDITOR 环境变量"
    "$editor" "$CONFIG_FILE"
    printf '\n'
    do_check || true
}

do_check() {
    [ -f "$CONFIG_FILE" ] || die "没有找到 ${CONFIG_FILE}"
    if [ "$(manage_mode)" = "docker" ]; then
        docker run --rm -v "${CONFIG_FILE}:/etc/XrayR/config.yml:ro" --entrypoint XrayR \
            "$DOCKER_IMAGE" config check -c /etc/XrayR/config.yml
    else
        [ -x "$SERVICE_BIN" ] || die "没有找到 ${SERVICE_BIN}"
        "$SERVICE_BIN" config check -c "$CONFIG_FILE"
    fi
    if config_is_placeholder; then
        printf '\n'
        warn "配置里还有示例值（panel.example.com / CHANGE_ME），记得改成你自己的面板信息"
    fi
}

do_enable() {
    case "$(manage_mode)" in
        docker) docker update --restart unless-stopped "$CONTAINER_NAME" >/dev/null && ok "已设置容器开机自启" ;;
        binary)
            [ "$HAS_SYSTEMD" -eq 1 ] || die "systemd 未运行"
            systemctl enable "$SERVICE_NAME" && ok "已设置开机自启" ;;
        *) die "尚未安装 XrayR" ;;
    esac
}

do_disable() {
    case "$(manage_mode)" in
        docker) docker update --restart no "$CONTAINER_NAME" >/dev/null && ok "已关闭容器开机自启" ;;
        binary)
            [ "$HAS_SYSTEMD" -eq 1 ] || die "systemd 未运行"
            systemctl disable "$SERVICE_NAME" && ok "已关闭开机自启" ;;
        *) die "尚未安装 XrayR" ;;
    esac
}

do_update() {
    local mode was_active=0
    mode="$(manage_mode)"
    [ -n "$mode" ] || die "尚未安装 XrayR，请先安装"

    if [ "$mode" = "docker" ]; then
        do_install_docker
        return
    fi

    log "更新 ${SERVICE_BIN}"
    if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
        was_active=1
        do_stop
    fi
    if [ "$MODE" = "release" ] || { [ "$MODE" = "auto" ] && release_exists; }; then
        install_from_release
    else
        install_from_source
    fi
    install_service_unit
    install_self
    if [ "$was_active" -eq 1 ]; then do_start; fi
    ok "更新完成"
}

do_uninstall() {
    local mode; mode="$(manage_mode)"
    log "卸载 XrayR"
    if [ "$mode" = "docker" ]; then
        docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
        ok "已删除容器"
    fi
    if [ "$HAS_SYSTEMD" -eq 1 ]; then
        systemctl disable --now "$SERVICE_NAME" >/dev/null 2>&1 || true
    fi
    rm -f "$SERVICE_PATH"
    [ "$HAS_SYSTEMD" -eq 1 ] && systemctl daemon-reload
    rm -f "$SERVICE_BIN"
    rm -rf "$LIB_DIR"
    ok "已删除服务二进制与 systemd 单元"
    if [ "$PURGE" -eq 1 ]; then
        rm -rf "$CONFIG_DIR"
        ok "已删除 ${CONFIG_DIR}"
    else
        printf '  %s\n' "配置保留在 ${CONFIG_DIR}（加 --purge 可一并删除）"
    fi
    local name removed=0
    for name in $MANAGER_NAMES; do
        if [ -f "${INSTALL_DIR}/${name}" ]; then
            rm -f "${INSTALL_DIR}/${name}"
            removed=1
        fi
    done
    if [ "$removed" -eq 1 ]; then
        printf '  %s\n' "已删除管理命令 ${MANAGER_NAMES}"
    fi
}

# -------------------------------------------------------------------------- menu

# ------------------------------------------------------------ status helpers

# service_badge prints a coloured dot plus the state in words, so the menu reads
# the same whether or not the terminal supports colour.
service_badge() {
    local state
    if [ "$(manage_mode)" = "docker" ]; then
        state="$(docker inspect -f '{{.State.Status}}' "$CONTAINER_NAME" 2>/dev/null || true)"
        case "$state" in
            running)   printf '%s●%s 运行中' "$C_GREEN" "$C_RESET" ;;
            "")        printf '%s●%s 未安装' "$C_DIM" "$C_RESET" ;;
            exited|created|paused) printf '%s●%s 已停止' "$C_YELLOW" "$C_RESET" ;;
            *)         printf '%s●%s %s' "$C_YELLOW" "$C_RESET" "$state" ;;
        esac
        return 0
    fi
    state="$(service_state)"
    case "$state" in
        active)    printf '%s●%s 运行中' "$C_GREEN" "$C_RESET" ;;
        failed)    printf '%s●%s 启动失败' "$C_RED" "$C_RESET" ;;
        inactive)  printf '%s●%s 已停止' "$C_YELLOW" "$C_RESET" ;;
        activating|deactivating|reloading)
                   printf '%s●%s %s' "$C_YELLOW" "$C_RESET" "$state" ;;
        "")        printf '%s●%s 未安装' "$C_DIM" "$C_RESET" ;;
        *)         printf '%s●%s %s' "$C_YELLOW" "$C_RESET" "$state" ;;
    esac
}

mode_label() {
    case "$(manage_mode)" in
        docker) printf 'Docker' ;;
        *) if [ -x "$SERVICE_BIN" ]; then printf 'systemd'; else printf '未安装'; fi ;;
    esac
}

installed_version() {
    if [ "$(manage_mode)" = "docker" ]; then
        docker inspect -f '{{.Config.Image}}' "$CONTAINER_NAME" 2>/dev/null | sed 's#.*:##' || true
    elif [ -x "$SERVICE_BIN" ]; then
        "$SERVICE_BIN" version 2>/dev/null | awk '{print $2}'
    fi
}

autostart_label() {
    local state
    if [ "$(manage_mode)" = "docker" ]; then
        state="$(docker inspect -f '{{.HostConfig.RestartPolicy.Name}}' "$CONTAINER_NAME" 2>/dev/null || true)"
    else
        state="$(service_enabled)"
    fi
    case "$state" in
        enabled|always|unless-stopped) printf '已开启' ;;
        "") printf '未安装' ;;
        *) printf '已关闭' ;;
    esac
}

hr() { printf '%s────────────────────────────────────────────────────────────%s\n' "$C_DIM" "$C_RESET"; }

print_menu() {
    local version
    version="$(installed_version)"

    printf '\n  %s%sXrayR 管理面板%s   %s·%s %s\n' \
        "$C_BOLD" "$C_BLUE" "$C_RESET" "$C_DIM" "$C_RESET" "$(mode_label)"
    hr
    printf '   %s状态%s     %s\n' "$C_DIM" "$C_RESET" "$(service_badge)"
    printf '   %s版本%s     %s\n' "$C_DIM" "$C_RESET" "${version:-未知}"
    printf '   %s开机自启%s %s\n' "$C_DIM" "$C_RESET" "$(autostart_label)"
    printf '   %s配置%s     %s\n' "$C_DIM" "$C_RESET" "$CONFIG_FILE"
    hr

    printf '\n   %s安装%s\n' "$C_BOLD" "$C_RESET"
    printf '     %s1%s  安装 / 重装（二进制 + systemd）\n' "$C_CYAN" "$C_RESET"
    printf '     %s2%s  安装 / 重装（Docker）\n' "$C_CYAN" "$C_RESET"

    printf '\n   %s服务%s\n' "$C_BOLD" "$C_RESET"
    printf '     %s3%s  启动            %s4%s  停止            %s5%s  重启\n' \
        "$C_CYAN" "$C_RESET" "$C_CYAN" "$C_RESET" "$C_CYAN" "$C_RESET"
    printf '     %s6%s  查看状态        %s7%s  查看日志\n' \
        "$C_CYAN" "$C_RESET" "$C_CYAN" "$C_RESET"

    printf '\n   %s配置与升级%s\n' "$C_BOLD" "$C_RESET"
    printf '     %s8%s  编辑配置        %s9%s  校验配置\n' \
        "$C_CYAN" "$C_RESET" "$C_CYAN" "$C_RESET"
    printf '    %s10%s  更新到最新版本  %s11%s  开机自启 开 / 关\n' \
        "$C_CYAN" "$C_RESET" "$C_CYAN" "$C_RESET"

    printf '\n   %s其他%s\n' "$C_BOLD" "$C_RESET"
    printf '    %s12%s  卸载\n' "$C_CYAN" "$C_RESET"

    printf '\n'
    hr
    printf '     %s0%s  退出\n\n' "$C_CYAN" "$C_RESET"
}

menu_pause() { printf '\n按回车继续...'; read -r _ || true; }

toggle_autostart() {
    local current
    if [ "$(manage_mode)" = "docker" ]; then
        current="$(docker inspect -f '{{.HostConfig.RestartPolicy.Name}}' "$CONTAINER_NAME" 2>/dev/null || echo none)"
    else
        current="$(service_enabled)"; [ -n "$current" ] || current="disabled"
    fi
    case "$current" in
        unless-stopped|always|enabled) do_disable ;;
        *) do_enable ;;
    esac
}

menu_loop() {
    local choice
    while true; do
        print_menu
        if ! read -r -p "请选择: " choice; then
            printf '\n'
            return 0
        fi
        case "$choice" in
            1)  do_install binary ;;
            2)  do_install docker ;;
            3)  do_start ;;
            4)  do_stop ;;
            5)  do_restart ;;
            6)  do_status ;;
            7)  do_logs 80 ;;
            8)  do_edit ;;
            9)  do_check ;;
            10) do_update ;;
            11) toggle_autostart ;;
            12) do_uninstall ;;
            0|q|Q) return 0 ;;
            "") ;;
            *) warn "无效选择：${choice}" ;;
        esac
        menu_pause
    done
}

# -------------------------------------------------------------------------- main

parse_flags() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --release)
                MODE="release"
                if [ $# -gt 1 ] && [ "${2#--}" = "$2" ]; then RELEASE_TAG="$2"; shift; fi ;;
            --build)        MODE="source" ;;
            --source-dir)   SOURCE_DIR="${2:?--source-dir 需要路径}"; shift ;;
            --ref)          REF="${2:?--ref 需要值}"; shift ;;
            --go-version)   GO_VERSION="${2:?--go-version 需要值}"; shift ;;
            --install-dir)  INSTALL_DIR="${2:?--install-dir 需要路径}"; shift ;;
            --lib-dir)      LIB_DIR="${2:?--lib-dir 需要路径}"; shift ;;
            --config-dir)   CONFIG_DIR="${2:?--config-dir 需要路径}"; shift ;;
            --skip-rules)   SKIP_RULES=1 ;;
            --skip-service) SKIP_SERVICE=1 ;;
            --no-start)     : ;;  # 现在默认就不启动，保留参数以兼容旧调用
            --no-config)    NO_CONFIG=1 ;;
            --purge)        PURGE=1 ;;
            -h|--help)      usage; exit 0 ;;
            *) die "未知参数 '$1'（--help 查看用法）" ;;
        esac
        shift
    done
    CONFIG_FILE="${CONFIG_DIR}/config.yml"
    SERVICE_BIN="${LIB_DIR}/XrayR"
}

main() {
    local script_dir
    script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
    if [ -f "${script_dir}/download-rules-dat.sh" ] && [ -f "${script_dir}/../go.mod" ]; then
        LOCAL_ROOT="$(cd -- "${script_dir}/.." && pwd)"
    fi

    # 先探测环境：DISTRO/ASSET/INSTALLED_MODE 要留在当前 shell 里供各命令使用
    detect_platform

    case "${1:-}" in
        menu)      shift; parse_flags "$@"; menu_loop; return 0 ;;
        install)   shift; parse_flags "$@"; do_install binary; return 0 ;;
        docker)    shift; parse_flags "$@"; do_install docker; return 0 ;;
        start)     shift; parse_flags "$@"; do_start; return 0 ;;
        stop)      shift; parse_flags "$@"; do_stop; return 0 ;;
        restart)   shift; parse_flags "$@"; do_restart; return 0 ;;
        status)    shift; parse_flags "$@"; do_status; return 0 ;;
        logs)      shift; lines="${1:-80}"; shift || true; parse_flags "$@"; do_logs "$lines"; return 0 ;;
        edit)      shift; parse_flags "$@"; do_edit; return 0 ;;
        check)     shift; parse_flags "$@"; do_check; return 0 ;;
        update)    shift; parse_flags "$@"; do_update; return 0 ;;
        enable)    shift; parse_flags "$@"; do_enable; return 0 ;;
        disable)   shift; parse_flags "$@"; do_disable; return 0 ;;
        uninstall) shift; parse_flags "$@"; do_uninstall; return 0 ;;
        help|-h|--help) usage; return 0 ;;
        "") ;;  # 没有参数：交给下面的默认分支
        -*) ;;  # 只有标志：交给下面的默认分支（安装）
        *)
            # 不是管理子命令就转发给服务二进制，这样 `xrayr config check`、
            # `xrayr doctor`、`xrayr version`、`xrayr -c FILE` 都能用，
            # 而且 xrayr / XrayR 两个名字的行为完全一致。
            if [ -x "$SERVICE_BIN" ]; then
                exec "$SERVICE_BIN" "$@"
            fi
            die "未知子命令 '$1'，且没找到服务二进制 ${SERVICE_BIN}；用 ${0##*/} help 查看用法"
            ;;
    esac

    local argc=$#
    parse_flags "$@"
    detect_platform

    if [ "$argc" -eq 0 ] && [ -t 0 ] && [ -t 1 ]; then
        menu_loop
    else
        do_install binary
    fi
}

main "$@"
