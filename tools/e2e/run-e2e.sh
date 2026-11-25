#!/usr/bin/env bash
#
# End-to-end test for XrayR without a real panel.
#
# Stands up a stub Xboard (UniProxy) panel, starts XrayR against it, and pushes real
# traffic through the node with a real xray-core client:
#
#   curl -> socks -> xray client -> VMess/WS -> XrayR node -> freedom -> local HTTP server
#
# Then it asserts the parts of the controller that are easiest to get wrong:
#   - the node listens on the port the panel announced
#   - the user from the panel is installed and per-user counters are created
#   - counted traffic is reported back to the panel
#   - a node config change moves the listener without restarting the process
#   - a config that cannot be applied is rolled back and the old node keeps serving
#
# Everything is local, so the test needs no internet access and is deterministic.
# It binds only high ports and does not touch systemd, so it does not need root.
#
# Requirements: python3, curl, go (unless XRAYR_BINARY is given).
#
# Usage:  bash tools/e2e/run-e2e.sh
#         XRAYR_BINARY=/usr/local/bin/XrayR bash tools/e2e/run-e2e.sh
set -euo pipefail

REPO_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"

PANEL_PORT=16670
NODE_PORT=18443
NODE_PORT_ALT=18444
SOCKS_PORT=11080
HTTP_PORT=18080
PAYLOAD="xrayr-e2e-payload-ok"
UUID="b831381d-6324-4d53-ad4f-8cda48b30811"
WS_PATH="/ws"

WORKDIR=""
PANEL_PID=""
HTTP_PID=""
XRAYR_PID=""
CLIENT_PID=""
FAILURES=0
KEEP_LOGS=0

if [ -t 1 ]; then G=$'\033[32m'; R=$'\033[31m'; B=$'\033[34m'; N=$'\033[0m'; else G=""; R=""; B=""; N=""; fi
step() { printf '\n%s==>%s %s\n' "$B" "$N" "$*"; }
pass() { printf '  %sPASS%s %s\n' "$G" "$N" "$*"; }
fail() { printf '  %sFAIL%s %s\n' "$R" "$N" "$*"; FAILURES=$((FAILURES + 1)); }
die()  { printf '%sfatal%s %s\n' "$R" "$N" "$*" >&2; exit 1; }

# Assertions take the command rather than its exit status: under `set -e` a bare
# `grep -q ...` would abort the script before the check could report it.
expect_ok()   { local d="$1"; shift; if "$@" >/dev/null 2>&1; then pass "$d"; else fail "$d"; fi; }
expect_fail() { local d="$1"; shift; if "$@" >/dev/null 2>&1; then fail "$d"; else pass "$d"; fi; }

cleanup() {
    [ -n "$CLIENT_PID" ] && kill "$CLIENT_PID" 2>/dev/null || true
    [ -n "$XRAYR_PID" ] && kill "$XRAYR_PID" 2>/dev/null || true
    [ -n "$PANEL_PID" ] && kill "$PANEL_PID" 2>/dev/null || true
    [ -n "$HTTP_PID" ] && kill "$HTTP_PID" 2>/dev/null || true
    sleep 1
    if [ "$KEEP_LOGS" -eq 1 ]; then
        printf '\nlogs kept in %s\n' "$WORKDIR" >&2
    elif [ -n "$WORKDIR" ] && [ -d "$WORKDIR" ]; then
        rm -rf "$WORKDIR"
    fi
    return 0
}
trap cleanup EXIT

listening() { ss -lntn 2>/dev/null | grep -q ":$1 "; }
wait_for_port() { # wait_for_port <port> <seconds>
    local i=0
    while [ "$i" -lt "$2" ]; do
        if listening "$1"; then return 0; fi
        sleep 1
        i=$((i + 1))
    done
    return 1
}

# ------------------------------------------------------------------ prerequisites

command -v python3 >/dev/null || die "python3 is required"
command -v curl >/dev/null || die "curl is required"

WORKDIR="$(mktemp -d /tmp/xrayr-e2e.XXXXXX)"
PANEL_LOG="${WORKDIR}/panel.log"
# Truncate once, here: the panel is restarted between steps and must not wipe the
# evidence the later assertions look for.
: > "$PANEL_LOG"

# -------------------------------------------------------------------- the fixture

write_panel() {
    local node_port="$1"
    cat > "${WORKDIR}/fake_panel.py" <<PYEOF
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

NODE_PORT = ${node_port}
LOG = "${PANEL_LOG}"


def log(message):
    with open(LOG, "a") as handle:
        handle.write(message + "\n")


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def _send(self, payload, code=200, etag=None):
        body = json.dumps(payload).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        if etag:
            self.send_header("Etag", etag)
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        path = self.path.split("?")[0]
        if path.endswith("/UniProxy/config"):
            log("GET  " + self.path)
            self._send({
                "server_port": NODE_PORT,
                "network": "ws",
                "tls": 0,
                "networkSettings": {"path": "${WS_PATH}", "host": ""},
                "base_config": {"push_interval": 5, "pull_interval": 5},
                "routes": [],
            }, etag='"cfg-%d"' % NODE_PORT)
        elif path.endswith("/UniProxy/user"):
            log("GET  " + self.path)
            self._send({"users": [{"id": 1, "uuid": "${UUID}", "speed_limit": 0}]}, etag='"usr-1"')
        else:
            self._send({"message": "not found"}, 404)

    def do_POST(self):
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length).decode("utf-8", "replace") if length else ""
        log("POST " + self.path + " " + raw)
        self._send({})

    def log_message(self, *args):
        pass


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", ${PANEL_PORT}), Handler).serve_forever()
PYEOF
}

write_xrayr_config() {
    cat > "${WORKDIR}/config.yml" <<EOF
ConfigVersion: 1
Log:
  Level: debug
  Format: text
Nodes:
  - PanelType: Xboard
    ApiConfig:
      ApiHost: http://127.0.0.1:${PANEL_PORT}
      ApiKey: e2e-token
      NodeID: 1
      NodeType: Vmess
      Timeout: 10
    ControllerConfig:
      ListenIP: 127.0.0.1
      UpdatePeriodic: 5
EOF
}

write_client_config() {
    cat > "${WORKDIR}/client.json" <<EOF
{
  "log": { "loglevel": "warning" },
  "inbounds": [
    { "tag": "socks", "listen": "127.0.0.1", "port": ${SOCKS_PORT}, "protocol": "socks",
      "settings": { "udp": false, "auth": "noauth" } }
  ],
  "outbounds": [
    { "tag": "proxy", "protocol": "vmess",
      "settings": { "vnext": [ { "address": "127.0.0.1", "port": ${NODE_PORT},
        "users": [ { "id": "${UUID}", "alterId": 0, "security": "auto" } ] } ] },
      "streamSettings": { "network": "ws", "wsSettings": { "path": "${WS_PATH}" } } }
  ]
}
EOF
}

restart_panel() { # restart_panel <node_port>
    if [ -n "$PANEL_PID" ]; then kill "$PANEL_PID" 2>/dev/null || true; fi
    sleep 1
    write_panel "$1"
    python3 "${WORKDIR}/fake_panel.py" > "${WORKDIR}/panel.out" 2>&1 &
    PANEL_PID=$!
    wait_for_port "$PANEL_PORT" 10 || die "stub panel did not start"
}

# ------------------------------------------------------------------------- build

build_xrayr() {
    if [ -n "${XRAYR_BINARY:-}" ]; then
        [ -x "$XRAYR_BINARY" ] || die "XRAYR_BINARY=$XRAYR_BINARY is not executable"
        printf '%s\n' "$XRAYR_BINARY"
        return
    fi
    command -v go >/dev/null || die "go is required (or set XRAYR_BINARY)"
    ( cd "$REPO_ROOT" && go build -trimpath -ldflags "-s -w" -o "${WORKDIR}/XrayR" . ) ||
        die "go build failed"
    printf '%s\n' "${WORKDIR}/XrayR"
}

build_xray_client() {
    command -v go >/dev/null || die "go is required to build the xray client"
    ( cd "$REPO_ROOT" && go build -o "${WORKDIR}/xray" github.com/xtls/xray-core/main ) ||
        die "building the xray client failed"
}

# -------------------------------------------------------------------------- run

step "preparing the fixture"
restart_panel "$NODE_PORT"
write_xrayr_config
write_client_config

# The traffic check targets a local HTTP server rather than a real site: it keeps the
# test deterministic and offline, and asserting on the body proves the payload really
# travelled through the tunnel instead of just that a connection was accepted.
printf '%s' "$PAYLOAD" > "${WORKDIR}/payload.txt"
python3 -m http.server "$HTTP_PORT" --bind 127.0.0.1 --directory "$WORKDIR" \
    > "${WORKDIR}/http.log" 2>&1 &
HTTP_PID=$!
wait_for_port "$HTTP_PORT" 10 || die "local HTTP server did not start"

XRAYR_BIN="$(build_xrayr)"
step "XrayR binary: ${XRAYR_BIN} ($("$XRAYR_BIN" version))"
"$XRAYR_BIN" config check -c "${WORKDIR}/config.yml" >/dev/null || die "config check failed"

step "starting the node"
"$XRAYR_BIN" -c "${WORKDIR}/config.yml" > "${WORKDIR}/xrayr.log" 2>&1 &
XRAYR_PID=$!

wait_for_port "$NODE_PORT" 20
expect_ok "node listens on the port the panel announced (${NODE_PORT})" listening "$NODE_PORT"
expect_ok "node process is alive" kill -0 "$XRAYR_PID"
expect_ok "user from the panel was installed" grep -q "Added 1 new users" "${WORKDIR}/xrayr.log"
expect_ok "per-user traffic counters were created" grep -q "traffic>>>uplink" "${WORKDIR}/xrayr.log"

step "pushing real traffic through the node"
build_xray_client
"${WORKDIR}/xray" run -c "${WORKDIR}/client.json" > "${WORKDIR}/client.log" 2>&1 &
CLIENT_PID=$!
wait_for_port "$SOCKS_PORT" 20 || die "the xray client did not start"

HTTP_CODE="$(curl -s -o "${WORKDIR}/body.txt" -w '%{http_code}' --max-time 25 \
    -x "socks5h://127.0.0.1:${SOCKS_PORT}" "http://127.0.0.1:${HTTP_PORT}/payload.txt" || true)"
HTTP_CODE="${HTTP_CODE:-000}"
if [ "$HTTP_CODE" = "200" ]; then
    pass "curl through the proxy returned HTTP 200"
else
    fail "curl through the proxy returned HTTP ${HTTP_CODE} (expected 200)"
fi
if [ "$(cat "${WORKDIR}/body.txt" 2>/dev/null)" = "$PAYLOAD" ]; then
    pass "the response body came back intact through the tunnel"
else
    fail "response body mismatch (got: $(head -c 80 "${WORKDIR}/body.txt" 2>/dev/null))"
fi

step "waiting for the traffic report"
PUSH_LINE=""
for _ in $(seq 1 20); do
    PUSH_LINE="$(grep -m1 '^POST .*UniProxy/push' "$PANEL_LOG" || true)"
    if [ -n "$PUSH_LINE" ]; then break; fi
    sleep 2
done
if [ -z "$PUSH_LINE" ]; then
    fail "panel received no traffic report"
elif printf '%s' "$PUSH_LINE" | grep -q '\[0,0\]'; then
    fail "traffic report carried zero counters: ${PUSH_LINE##* }"
else
    pass "panel received a traffic report with non-zero counters"
    printf '      %s\n' "$PUSH_LINE"
fi

step "changing the node port from the panel"
restart_panel "$NODE_PORT_ALT"
wait_for_port "$NODE_PORT_ALT" 25
expect_ok "listener moved to ${NODE_PORT_ALT}" listening "$NODE_PORT_ALT"
expect_fail "old port ${NODE_PORT} was released" listening "$NODE_PORT"
expect_ok "process was not restarted" kill -0 "$XRAYR_PID"

step "pushing a config that cannot be applied (port already in use)"
restart_panel "$PANEL_PORT"
sleep 20
expect_ok "node still listens on ${NODE_PORT_ALT} after the rejected config" listening "$NODE_PORT_ALT"
expect_ok "rollback was logged" grep -q "rejected and rolled back" "${WORKDIR}/xrayr.log"
expect_ok "process survived the rollback" kill -0 "$XRAYR_PID"

printf '\n'
if [ "$FAILURES" -eq 0 ]; then
    printf '%sall checks passed%s\n' "$G" "$N"
else
    printf '%s%d check(s) failed%s\n' "$R" "$FAILURES" "$N" >&2
    KEEP_LOGS=1
fi
exit "$FAILURES"
