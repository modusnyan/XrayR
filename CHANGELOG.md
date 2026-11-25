# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [1.2.0] - 2026-11-26

The node now follows the panel when it is switched off, tells the panel what it
serves, and refuses configurations it cannot serve instead of running them with
parts silently missing. Release it together with the matching Xboard.

### Added

- Node status and online users are reported to Xboard (`UniProxy/status`,
  `UniProxy/alive`): CPU, memory, swap and disk usage, uptime, the users served
  and the synchronization state. The panel's node list and its user details show
  them. An empty online report is sent as well, so the panel's count drops to
  zero. A panel without these endpoints answers 404, and the report is switched
  off with a single warning.
- A node disabled in the panel stops serving. Xboard answers its config and user
  requests with HTTP 410 and `X-Node-State: disabled` (both are required); the
  node removes its inbound, deletes its snapshot so a restart during a panel
  outage cannot revive it, pushes the traffic counted before the shutdown, and
  is rebuilt from a fresh configuration once the panel enables it again. A node
  disabled at startup starts offline. Every other panel error keeps the node
  running as before.
- XHTTP `mode` and `extra` from the panel reach the inbound. The client-only
  `downloadSettings` and `xmux` keys are dropped, and empty values mean unset.
- REALITY `dest` and `xver` set in the panel are honoured.
- The panel's per-node `cert_config` is compared with the local `CertConfig`,
  and mismatches are logged once. A TLS node without a local certificate, which
  listens in plaintext, is flagged. Nothing from the panel's certificate config
  is applied yet.
- Shadowsocks simple-obfs is read from `plugin` / `plugin_opts`, the fields the
  subscriptions hand to clients; `obfs` / `obfs_settings` remain as the legacy
  form.

### Changed

- A Shadowsocks plugin XrayR cannot serve (v2ray-plugin, `obfs=tls`, ...) fails
  the node with an explicit error instead of being ignored while every client
  still uses it. Clean such nodes up in the panel before upgrading: a node
  without a snapshot will not start, and it takes the other nodes of the process
  with it.
- `VlessFlow` is no longer defaulted to `xtls-rprx-vision`. The Xboard adapter
  never read it: the flow always came from the panel, and a panel flow of `none`
  now means no flow. A local value is reported as ignored at startup.
- Routes other than `block` are logged as ignored instead of being dropped
  silently.
- A configuration the node rejects stays in diagnostics and in the status report
  until the panel sends a different one. The panel's next 304 for the same
  configuration used to clear it after one period. Panel outages still clear on
  the next good round.

### Fixed

- An empty user list from the panel was treated as an error, so banned and
  expired users kept their access. It is now applied once the next pull confirms
  it, which keeps a single bad answer from disconnecting everyone. Snapshots
  accept an empty list as well.
- The local `DeviceLimit` never forgot an address. The per-period IP set was
  only reset by the online-user report, which was switched off, so a user who
  had used `DeviceLimit` different addresses was refused on every new one until
  XrayR restarted.
- XHTTP nodes ignored their Host.
- The REALITY fallback target was `host:` when the panel's server port was empty
  (Xboard's default) and `host:443:443` when `dest` already had a port.
- A rejected node configuration echoed the panel response, which carries the
  SS-2022 server key and the REALITY private key, into the log and the
  diagnostics page. Rotate those keys if logs from earlier versions left the
  node.

## [1.1.0] - 2026-10-26

The panel now steers what it always could describe but the node ignored.

### Added

- The node follows the sync intervals the panel advertises in
  `base_config`: `pull_interval` drives the node monitor and the core
  heartbeat, `push_interval` drives traffic reporting (and the AutoSpeedLimit
  thresholds, which are per reporting period). Values the panel does not send
  fall back to the locally configured `UpdatePeriodic`. Changing the interval
  in the panel takes effect on the node's next sync without a restart.
- The per-user `device_limit` from the panel's user list is honoured when no
  local `DeviceLimit` is configured. A local value still wins.

### Fixed

- A VLESS node with `EnableVless: true` lost its transport parameters. The
  adapter overwrote the parsed top-level `networkSettings` with the separate
  `network_settings` block, which Xboard never sends, so the zero struct wiped
  the WebSocket path/host and the gRPC serviceName. The overwrite now happens
  only when the panel actually supplies that block.
- The VLESS decryption the panel sends when VLESS Encryption is enabled was
  discarded in favour of a hardcoded `none`. It is now passed through to the
  inbound; combining a real decryption with fallbacks is rejected with an
  explanatory error, which xray-core forbids anyway.

### Added (previously unreleased)

- `config check` warns when `ApiHost` or `ApiKey` still holds the installed
  template's placeholder. Both values are syntactically valid, so the check used
  to pass and the node only failed later with an unreachable host.

### Changed (previously unreleased)

- The installed configuration template now matches what the code does.
  `EnableDNS` was documented as switching on `dns.json`; it actually switches the
  outbound domain strategy from `Asis` to `DNSType`, while `DnsConfigPath` is
  applied regardless. The template also documents `EnableVless` (a VLESS node only
  reads `network_settings` when it is set), the REALITY fallback precedence,
  `Diagnostics`, and stops pointing at `release/config/config.full.yml`, which does
  not exist on an installed node.

## [1.0.2] - 2026-10-19

REALITY nodes now take their parameters from the panel, which is the only place
those parameters exist for an Xboard deployment.

### Fixed

- A REALITY node whose parameters come from the panel was built without REALITY
  at all. `InboundBuilder` only consulted the panel when
  `DisableLocalREALITYConfig` was set, so the default configuration — panel node
  with TLS type 2, nothing about REALITY in `config.yml` — produced a plain VLESS
  listener while the panel and every client expected a REALITY handshake. The
  panel now always wins when it describes the node as REALITY, because those are
  the parameters it hands to clients; `ControllerConfig.REALITYConfigs` is only a
  fallback for a node the panel does not describe as REALITY, and
  `DisableLocalREALITYConfig: true` still suppresses that fallback.
- Setting `DisableLocalREALITYConfig: true` without a `REALITYConfigs` block
  dereferenced a nil pointer and killed the process during startup. That is the
  configuration the Xboard guide recommended, so the crash was reachable by
  following the documentation.

## [1.0.1] - 2026-10-15

Operator-facing fixes for the 1.0.0 release: the node can find its configuration
again, the command name is case-insensitive, and release builds ship only the
architectures the installer can actually install.

### Changed

- Running the binary without `-c` now finds `/etc/XrayR/config.yml` before falling
  back to `./config.yml`. On an installed node `XrayR` used to fail with
  `open config config.yml: no such file or directory` because the working directory
  is not the configuration directory.
- The service binary moved to `/usr/local/lib/xrayr/XrayR`, and `/usr/local/bin`
  now holds the management command under **both** `xrayr` and `XrayR`. On a
  case-sensitive filesystem those were two different things — one opened the menu
  and the other started the service — so the name is now case-insensitive as users
  expect. Anything that is not a management subcommand (`xrayr config check`,
  `xrayr doctor`, `xrayr version`, `xrayr -c FILE`) is forwarded to the binary, and
  the installed copy has custom `--install-dir` / `--lib-dir` / `--config-dir`
  values baked in. `--lib-dir` is new; `--install-dir` now means the directory for
  the management command.
- Release builds target Linux only (amd64, 386, arm64, arm/v5-v7, riscv64, s390x,
  ppc64le) instead of twenty-three assets across six operating systems. That is
  exactly the set `release/install.sh` can fetch, and the Windows, macOS, BSD, Android
  and MIPS archives nobody could install are gone.

### Fixed

- The missing-configuration error read `open config config.yml: open config
  config.yml: no such file or directory`, repeating the path. It now says the file is
  missing and how to point at a different one.
- A Shadowsocks node using `2022-blake3-chacha20-poly1305` died at startup with
  `failed to build inbound handler for protocol shadowsocks > infra/conf:
  shadowsocks 2022 (multi-user): only blake3-aes-*-gcm methods are supported`,
  which names neither the cipher nor the replacement. XrayR always runs SS-2022 in
  xray-core's multi-user mode, because it adds and removes one account per panel
  user and only the multi-user inbound implements `proxy.UserManager`; that mode
  accepts blake3-aes-*-gcm only, so the chacha20 variant can never work here.
  `InboundBuilder` now rejects it up front and says which two ciphers do work.

## [1.0.0] - 2026-10-14

1.0.0 narrows the project to a single supported panel. Xboard is the only adapter
left, the data-plane hot paths were fixed, and the observability that had been
registered but never populated now reports real numbers.

### Added

- **Core liveness heartbeat.** xray-core exposes no liveness flag, so a wedged
  core keeps the process alive while serving nothing — invisible to both the
  node and the user monitor. A new `core heartbeat` periodic task dials the
  inbound listener every `UpdatePeriodic`; after three consecutive failures the
  process exits so the service manager restarts the node. A port move or a slow
  listener restart cannot trigger it. New metrics
  `xrayr_core_healthy` and `xrayr_core_probe_failures_total`, new option
  `DisableCoreHeartbeat`.

### Changed

- **`xrayr_panel_requests_total` is now populated.** Adapters record each API
  call exactly once through `api.ObservePanelRequest` / `api.ObservePanelError`,
  after the response has been fully handled, so the counter reports logical
  outcomes: a 200 whose body fails to parse is labelled `invalid_payload`, not
  `success`.
- **`xrayr_certificate_days_remaining` is now populated.** `certMonitor` reads
  the leaf certificate it is already renewing and publishes the remaining
  validity. `CertMode: file` nodes are covered too, which previously had no
  expiry visibility at all.
- **`errcheck` is enabled.** All 51 unchecked error returns are now either
  handled or explicitly discarded with `_ =`. Two were real defects: the
  unchecked `json.Unmarshal` calls in the Xboard adapter could silently produce a
  zero-valued node config or an empty user list, and `generatePrivateKey` could
  report success after failing to flush a private key to disk.
- `certMonitor` no longer dereferences a nil `LegoCMD` when `mylego.New` fails.
- **Xboard is now the only supported panel.** The `SSpanel`, `PMpanel`,
  `Proxypanel`, `V2RaySocks`, `GoV2Panel` and `BunPanel` adapters were removed
  (`api/sspanel`, `api/pmpanel`, `api/proxypanel`, `api/v2raysocks`,
  `api/gov2panel`, `api/bunpanel`). The names `Xboard`, `NewV2board` and
  `V2board` remain equivalent aliases; any other `PanelType` is rejected by
  both startup and `config check`, with the closest candidate suggested.
- The `Shadowsocks-Plugin` and `dokodemo-door` node types were removed along
  with SSpanel, the only adapter that could produce them. `dokodemo-door` is
  no longer accepted by `InboundBuilder`.
- The `DisableCustomConfig` API option was removed. Only SSpanel honoured it,
  and Xboard always derives the node from the UniProxy response. Existing
  configurations that still carry the key keep loading; the value is ignored.
- `api.BaseClient` and `api.ApplyETag` now hold the adapter identity fields,
  `Describe`, `Debug` and the ETag / 304 conditional-request flow that used to
  be copy-pasted per adapter.

### Fixed

- A missing snapshot no longer crash-loops the node. When `Cache` is enabled
  and the snapshot file does not exist, `snapshotStore.Load()` returned a raw
  `ENOENT` that propagated up to `Panel.Start()` and terminated the process,
  leaving systemd restarting forever. `os.IsNotExist` is now logged as a
  warning and the normal panel fetch proceeds.
- `cipherFromString` recognises `xchacha20-poly1305` instead of returning
  `UNKNOWN`, so Shadowsocks nodes using that cipher can start.

## [0.9.7] - 2026-10-09

### Added

- `xrayr` 管理菜单：`release/install.sh` 不带参数运行会打开菜单（安装 / Docker 安装 /
  启动 / 停止 / 重启 / 状态 / 日志 / 编辑配置 / 校验配置 / 更新 / 开机自启 / 卸载），
  安装后把自身装成 `/usr/local/bin/xrayr`，随时可以 `xrayr menu` 再打开，
  也支持同名子命令用于脚本化。
- `release/config/config.template.yml`：安装时生成的带注释配置模板。只需要改第 1 个
  节点块里的 4 个字段（ApiHost / ApiKey / NodeID / NodeType），可用的面板类型、协议
  和全部高级选项都作为注释列在同一个文件里。
- `tools/e2e/run-e2e.sh`: end-to-end test with no real panel. It stands up a stub
  Xboard panel, pushes real traffic through the node with a real xray-core client
  (`curl -> socks -> VMess/WS -> node -> freedom -> local HTTP server`), and checks
  that the node moves its listener when the panel changes the port and rolls back
  when a config cannot be applied. Entirely local, so it needs no secrets and no
  network. Now runs in CI.
- `release/install.sh`: one-click installer for Debian. Detects the architecture and
  Debian release, installs the required packages, builds from source (installing Go
  when missing) or unpacks a prebuilt release, creates `/etc/XrayR`, downloads the
  rule data, installs the systemd unit, validates the configuration and starts the
  service. Supports `--init`, `--release`, `--source-dir`, `--skip-rules`,
  `--no-start` and `--uninstall [--purge]`, and is idempotent on re-run.
- `release/download-rules-dat.sh`: downloads the latest `geoip.dat` / `geosite.dat`
  from [Loyalsoldier/v2ray-rules-dat](https://github.com/Loyalsoldier/v2ray-rules-dat)
  and verifies the published sha256 checksum.
- `release/systemd/XrayR.service`: systemd unit with `ExecStartPre` configuration
  validation, matching what `docs/installation.md` documents.
- `api.AppendRule`: compiles a panel-supplied audit rule pattern and skips it with a
  warning instead of panicking when the panel sends an invalid regular expression.
- `api.NewHTTPClient`, `api.ReadLocalRuleList`, `api.CheckResponse`: shared adapter
  helpers that replace seven near-identical copies.
- Offline unit tests using `httptest`: `api/client_test.go`,
  `api/sspanel/offline_test.go`, `internal/snapshot/snapshot_test.go`,
  `internal/redact/redact_test.go`, `panel/ruledata_test.go`,
  `service/controller/state_race_test.go`.
- CI now runs `go test -race ./...`, `golangci-lint run`, `shellcheck` on
  `release/*.sh`, and validates `release/config/route.json` against freshly
  downloaded rule data.
- `.golangci.yml`: conservative linter set (`govet`, `ineffassign`, `staticcheck`,
  `unused`, `gofmt`). `errcheck` is disabled with an explanation; enabling it is a
  follow-up.

### Changed

- 安装流程改为「生成带注释的配置 → 用户自己改 → 菜单里手动启动」。安装不再自动
  启用和启动服务，`xrayr start` 会在服务稳定运行后才报成功（systemd 单元带
  `Restart=always`，只看一次 `is-active` 会把崩溃循环误判成启动成功），
  启动失败时直接打印最近的日志。
- 安装时创建 `/etc/XrayR/cache`：`Cache` 默认开启，缺这个目录会让面板不可用时的
  快照回退直接失败。
- `Controller` runtime state (`nodeInfo`, `userList`, `Tag`) is published through an
  `atomic.Pointer[runtimeState]` snapshot instead of unsynchronized fields. The node
  monitor and the user monitor run on the same interval and previously raced on the
  user list.
- Periodic tasks are wrapped in `Controller.guardTask`, which recovers from panics.
  xray-core's `task.Periodic` neither recovers nor survives a non-nil error, so an
  unguarded panic used to terminate the whole process.
- Not-modified detection uses sentinel errors (`api.ErrNodeNotModified`,
  `api.ErrUserNotModified`, `api.ErrRuleNotModified`) compared with `errors.Is`,
  replacing `err.Error() == "..."` string comparisons.
- `parseConnectionConfig` returns an error instead of calling `log.Panicf`.
- `mydispatcher.Dispatch` returns an error for an invalid destination instead of
  panicking.
- `v2raysocks.GetNodeRule` returns the local rule list when the node configuration
  has not been fetched yet, instead of dereferencing a nil `ConfigResp`.
- Configuration reload records the attempt even when the new configuration is
  rejected, so a burst of invalid writes no longer re-runs validation on every event.
- Configuration reload exits with a non-zero status when neither the new nor the
  previous configuration can start, instead of staying alive while serving nothing.
- `defer p.Close()` on shutdown closes the panel that is actually running.

### Removed

- `release/config/geoip.dat` and `release/config/geosite.dat` are no longer committed.
  They are downloaded on every install; see `docs/installation.md`.
- Dead `assembleURL` helpers in six panel adapters.
- `api.NodeNotModified` / `api.UserNotModified` / `api.RuleNotModified` string
  constants, superseded by the sentinel errors above.
- Dead code reported by `golangci-lint`: `getConfig` in `cmd/root.go`,
  `LegoCMD.getPath` / `LegoCMD.getCertConfig`, `integerDescription` in
  `internal/configui`, and the unused `Writer.w` field in `common/limiter`.

### Fixed

- Misplaced configuration fields are now self-diagnosing. `CertConfig` written next to
  `ControllerConfig` instead of inside it produced
  `field CertConfig not found in type panel.NodesConfig`, which says what is wrong but
  not where the field belongs; the error now adds
  `hint: CertConfig is valid, but it does not belong under Nodes[]; nest it under
  Nodes[].ControllerConfig`.
- Decode errors now report the line number in the operator's own file. `Decode`
  re-encoded the YAML before parsing (to drop retired fields), which sorted the keys and
  made every reported line number point into the rewritten document. It only re-encodes
  when something was actually removed.
- The optional blocks in `config.template.yml` did not survive being uncommented: the
  top-level block used `# Key:` so removing the `#` left a one-space indent, and top-level
  keys must start at column 0 (`did not find expected key`); the two blocks were also in
  an order that broke the 6-space node block. The indentation inside the comments is now
  the real indentation, and `TestConfigTemplateOptionalBlocksStayValid` uncomments both
  blocks and asserts the fields land at the right level.
- `release/config/route.json` contained a rule with an empty `domain` list, which
  xray-core rejects with `this rule has no effective fields` when the instance is
  created. Users who enabled `RouteConfigPath` could not start the node at all.
- `ReadLocalRuleList` dereferenced a nil `*os.File` when the rule file could not be
  opened, and called `log.Fatalf` on a read error, terminating the process.
- `os.Kill` was passed to `signal.Notify`, which can never deliver it.
- A periodic task that stopped because `Execute` returned an error did so silently;
  the reason is now logged.
