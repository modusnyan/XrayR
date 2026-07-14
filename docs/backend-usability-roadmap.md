# XrayR 后端易用性改进计划

## 1. 背景

XrayR 当前已经支持多种面板和协议，但用户在部署、配置和排查问题时仍然面临较高门槛。结合 Xboard 对接方式调整过程中暴露的问题，当前主要痛点不是缺少功能，而是用户很难确认以下事项：

1. 配置文件应该如何填写；
2. 配置内容是否有效、是否真正生效；
3. 故障发生在配置、网络、面板还是 XrayR 内部；
4. 不同面板名称、兼容接口和功能支持范围之间有什么区别；
5. 面板下发错误配置后，节点能否安全地继续运行。

本计划以“降低首次部署成本、缩短故障排查时间、减少历史兼容逻辑”为目标，优先完善配置、校验和诊断能力，再考虑管理界面。

---

## 2. 总体目标

最终希望用户通过以下流程完成部署：

```bash
XrayR config init
XrayR config check
XrayR doctor
systemctl enable --now XrayR
```

各命令职责如下：

- `config init`：生成配置；
- `config check`：执行本地静态校验；
- `doctor`：检查面板、网络、端口、证书和运行环境；
- `systemctl`：启动经过验证的服务。

设计原则：

- 配置错误应尽可能在启动前发现；
- 错误信息必须包含问题位置、原因和修复建议；
- 面板差异应通过适配器能力描述，不应散落为特殊判断；
- 配置更新失败时应继续使用上一份有效配置；
- 日志和诊断输出不得泄露 API Key、Token 等敏感信息；
- 文档、配置示例和实际代码必须保持同步。

---

## 3. 第一阶段：配置基础能力

第一阶段优先实施低成本、高收益的改进，为后续配置向导和诊断工具提供公共基础。

### 3.1 建立面板注册表

当前面板实例通过 `switch` 创建，历史名称会不断堆积，例如：

```go
case "NewV2board", "V2board", "Xboard":
    apiClient = newV2board.New(nodeConfig.ApiConfig)
```

建议改为集中注册：

```go
type PanelDefinition struct {
    Name         string
    Aliases      []string
    New          func(*api.Config) api.API
    Capabilities PanelCapabilities
}
```

示例：

```go
var panelDefinitions = []PanelDefinition{
    {
        Name:    "Xboard",
        Aliases: []string{"NewV2board", "V2board"},
        New:     newV2board.New,
        Capabilities: PanelCapabilities{
            NodeConfig:   true,
            UserSync:      true,
            TrafficReport: true,
            OnlineReport:  false,
            NodeStatus:    false,
        },
    },
}
```

#### 功能要求

- 面板名称大小写不敏感；
- 支持历史名称别名；
- 输入值归一化为规范名称；
- 能列出所有受支持面板；
- 输入拼写错误时提供候选名称；
- 面板支持能力集中声明；
- 控制器中不再出现 `panelType == "Xboard"` 一类特殊判断。

#### 验收标准

- `Xboard`、`NewV2board` 和 `V2board` 能创建同一种适配器；
- 不支持的名称返回包含可选值和拼写建议的错误；
- 现有配置保持向后兼容；
- 面板注册表具有单元测试。

### 3.2 实现统一配置校验器

增加独立配置校验包，确保 CLI 和服务启动流程使用同一套规则。

#### 校验范围

- YAML 是否能正确解析；
- 是否至少配置一个节点；
- `ApiHost` 是否为合法 HTTP/HTTPS 地址；
- `ApiKey` 是否为空；
- `NodeID` 是否大于 0；
- `NodeType` 是否受当前面板支持；
- 超时时间和更新周期是否合理；
- 监听端口是否在合法范围内；
- 多节点配置是否存在监听冲突；
- 证书和私钥文件是否存在、是否可读；
- Reality 必填字段是否完整；
- Redis 配置是否完整；
- 本地规则文件是否存在；
- 本地规则正则表达式是否有效；
- 未知字段和已废弃字段是否出现。

#### 校验结果结构

校验器不应直接打印日志，而应返回结构化结果：

```go
type ValidationIssue struct {
    Severity   Severity
    Path       string
    Message    string
    Suggestion string
}
```

严重程度至少包括：

- `error`：必须修复，否则禁止启动；
- `warning`：允许启动，但应提示风险；
- `info`：提供兼容性或默认值说明。

#### 错误输出示例

```text
错误：Nodes[0].ApiConfig.ApiHost 不能为空
修复：填写面板地址，例如 https://panel.example.com

警告：Nodes[1].ControllerConfig.UpdatePeriodic=5
建议：更新周期不应低于 30 秒，以避免对面板产生过多请求
```

#### 验收标准

- 启动前自动执行统一校验；
- 所有问题包含准确的字段路径；
- 单次校验能返回全部问题，而不是遇到第一个错误就退出；
- 校验器具有正常配置、边界值和错误配置测试。

### 3.3 实现 `XrayR config check`

命令示例：

```bash
XrayR config check -c /etc/XrayR/config.yml
```

预期输出：

```text
✓ YAML 格式正确
✓ 配置了 2 个节点
✓ 面板类型均受支持

错误：
  Nodes[0].ApiConfig.ApiHost 不能为空

警告：
  Nodes[1].ControllerConfig.UpdatePeriodic=5
  更新周期过短，建议不要低于 30 秒
```

#### 命令行为

- 只执行本地检查，不访问面板；
- 支持文本和 JSON 输出；
- 配置无错误时退出码为 `0`；
- 存在配置错误时退出码非 `0`；
- 仅有警告时退出码为 `0`；
- 支持 CI 和部署脚本调用。

#### 验收标准

- 可用于 systemd 的 `ExecStartPre`；
- JSON 输出字段稳定并有测试；
- 输出不得包含未脱敏的敏感信息。

### 3.4 改善错误信息

所有用户可操作的错误都应包含：

1. 发生了什么；
2. 问题所在位置；
3. 当前错误值；
4. 允许值或预期格式；
5. 如何修复。

示例：

```text
配置错误：Nodes[0].PanelType="Xbord" 不受支持。

你是否想输入：Xboard

支持的面板类型：
  Xboard
  SSPanel
  PMPanel
  ProxyPanel
  V2RaySocks
  GoV2Panel
  BunPanel
```

面板 API 错误示例：

```text
Xboard 节点配置获取失败：
  面板：https://panel.example.com
  节点：12
  状态码：401

可能原因：
  1. ApiKey 不正确；
  2. 节点 ID 与 ApiKey 不匹配；
  3. Xboard 节点未启用。

敏感参数已隐藏。
```

#### 安全要求

- 不在日志中打印完整带查询参数的请求 URL；
- 自动隐藏 `ApiKey`、`token`、密码和私钥；
- Debug 模式也不得默认输出认证信息；
- HTTP 响应体在输出前应检查并脱敏。

### 3.5 分离配置模板

建议将配置模板拆分为：

```text
release/config/
├── config.minimal.yml
├── config.yml.example
└── config.full.yml
```

#### `config.minimal.yml`

只包含首次运行必须修改的字段：

```yaml
Log:
  Level: info

Nodes:
  - PanelType: Xboard
    ApiConfig:
      ApiHost: https://panel.example.com
      ApiKey: CHANGE_ME
      NodeID: 1
      NodeType: Vless
```

#### `config.yml.example`

包含常用功能和简明注释，适合大部分用户。

#### `config.full.yml`

包含全部高级字段、可选值、单位和详细说明。

#### 验收标准

- 三份模板均可被当前版本解析；
- CI 自动运行配置校验；
- 模板中不存在已经移除的字段；
- 默认值由代码集中维护，避免不同模块分别设置。

---

## 4. 第二阶段：部署和故障诊断

### 4.1 实现 `XrayR config show`

用于显示解析、默认值填充和别名归一化后的最终配置：

```bash
XrayR config show -c /etc/XrayR/config.yml
```

示例：

```yaml
Nodes:
  - PanelType: Xboard
    EffectiveAdapter: NewV2board
    ApiConfig:
      ApiHost: https://panel.example.com
      ApiKey: "***REDACTED***"
      NodeID: 12
      NodeType: Vless
      Timeout: 30
    ControllerConfig:
      UpdatePeriodic: 60
```

#### 功能要求

- 显示最终默认值；
- 显示面板别名映射结果；
- 默认隐藏所有敏感信息；
- 支持 YAML 和 JSON 输出；
- 不执行网络请求。

### 4.2 实现 `XrayR doctor`

该命令负责实际检查完整运行链路：

```bash
XrayR doctor -c /etc/XrayR/config.yml
```

预期输出：

```text
XrayR Doctor

[配置]
✓ YAML 解析成功
✓ 节点配置完整

[网络]
✓ panel.example.com DNS 解析成功
✓ TCP 443 连接成功
✓ TLS 证书有效，有效期剩余 72 天

[面板]
✓ Xboard UniProxy API 可用
✓ API Token 验证成功
✓ 节点 12 存在
✓ 获取节点配置成功
✓ 获取到 138 个用户

[运行环境]
✓ 监听端口 443 未被占用
⚠ 证书将在 12 天后过期

诊断结果：可以启动
```

#### 检查项目

- 本地配置校验；
- DNS 解析；
- TCP 连通性；
- TLS 证书有效性和剩余时间；
- 面板 API 可用性；
- API Key 是否有效；
- 节点是否存在；
- 节点类型是否匹配；
- 节点配置是否能解析；
- 用户列表是否能获取；
- 监听端口是否被占用；
- 证书和私钥是否匹配；
- Redis 是否可连接；
- 文件和目录权限是否正确。

#### 设计要求

- 检查动作不得修改面板数据；
- 默认不执行流量上报等写操作；
- 每个检查项有单独超时；
- 支持仅检查指定节点；
- 支持 JSON 输出；
- 失败时给出下一步修复建议。

### 4.3 API 错误分类

建立统一错误类型：

```go
type APIErrorKind string

const (
    APIErrorNetwork        APIErrorKind = "network"
    APIErrorTimeout        APIErrorKind = "timeout"
    APIErrorAuthentication APIErrorKind = "authentication"
    APIErrorNotFound       APIErrorKind = "not_found"
    APIErrorRateLimited    APIErrorKind = "rate_limited"
    APIErrorInvalidPayload APIErrorKind = "invalid_payload"
    APIErrorServer         APIErrorKind = "server"
)
```

控制器、CLI 和日志根据错误类型输出不同建议，避免所有错误都退化为一个字符串。

### 4.4 启动前预检

推荐启动流程：

```text
加载配置
  ↓
配置结构校验
  ↓
文件权限、监听端口和证书检查
  ↓
连接面板并获取节点配置
  ↓
构建 Xray Core 配置
  ↓
验证 Core 配置
  ↓
原子启动或原子重载
```

#### 验收标准

- 无效配置不得替换当前有效配置；
- 预检失败时输出明确原因；
- 服务首次启动和热更新使用同一套验证流程；
- 预检结果可被 `doctor` 复用。

---

## 5. 第三阶段：长期维护能力

### 5.1 拆分 API 能力接口

当前统一接口要求面板实现所有方法，不支持的功能通常返回 `nil`。这使控制器无法区分“上报成功”和“该面板不支持”。

建议拆分接口：

```go
type NodeProvider interface {
    GetNodeInfo() (*NodeInfo, error)
    GetUserList() (*[]UserInfo, error)
}

type TrafficReporter interface {
    ReportUserTraffic(*[]UserTraffic) error
}

type OnlineUserReporter interface {
    ReportNodeOnlineUsers(*[]OnlineUser) error
}

type NodeStatusReporter interface {
    ReportNodeStatus(*NodeStatus) error
}
```

控制器按能力调用：

```go
if reporter, ok := client.(api.OnlineUserReporter); ok {
    err := reporter.ReportNodeOnlineUsers(users)
}
```

#### 预期收益

- 删除面板类型特殊判断；
- 不支持的功能不会被误认为调用成功；
- 面板注册表可以展示准确的能力矩阵；
- 新增面板时只实现真正支持的接口。

### 5.2 增加配置版本

在配置中加入：

```yaml
ConfigVersion: 1
```

提供迁移命令：

```bash
XrayR config migrate \
  --input old-config.yml \
  --output new-config.yml
```

#### 迁移范围

- 历史面板名称归一化；
- 已废弃字段替换；
- 字段重命名；
- 默认值变化；
- Reality 和旧 XTLS 配置转换；
- 对无法迁移的字段给出警告。

未知字段不应静默忽略。例如：

```text
警告：Nodes[0].ApiConfig.MachineID 已被移除，将被忽略。
Xboard 现在通过 UniProxy REST API 对接。
```

### 5.3 配置原子更新和失败回滚

面板下发的新配置必须先验证，再替换当前配置。

#### 行为要求

- 保留最后一份有效配置；
- 新配置验证失败时继续运行旧配置；
- 记录当前配置版本和更新时间；
- 记录最近一次更新失败原因；
- 重启后可以读取本地缓存，在面板暂时不可用时恢复服务；
- 缓存中不得明文写入不必要的敏感信息。

示例日志：

```text
ERROR 新配置验证失败，继续使用上一份有效配置
      node=12 reason="server port must be greater than 0"
```

### 5.4 可观测性

日志配置建议支持：

```yaml
Log:
  Level: info
  Format: text # text 或 json
```

结构化日志示例：

```text
INFO panel connected panel=Xboard node=12 protocol=Vless
INFO users updated node=12 added=3 removed=1 unchanged=134
INFO traffic reported node=12 users=48 upload=1.2GB download=8.7GB
```

增加可选诊断服务：

```yaml
Diagnostics:
  Enable: false
  Listen: 127.0.0.1:8080
```

端点：

- `/healthz`：进程存活；
- `/readyz`：节点是否完成初始化；
- `/metrics`：Prometheus 指标；
- `/status`：脱敏后的节点状态。

建议指标：

- 面板 API 请求数量和成功率；
- 最后一次成功同步时间；
- 当前用户数量；
- 流量上报失败次数；
- 节点配置重载次数；
- 当前有效配置版本；
- 证书剩余有效天数。

#### 安全要求

- 诊断服务默认关闭；
- 启用后默认仅监听 `127.0.0.1`；
- `/status` 不返回 Token、密码、私钥和完整用户身份信息；
- 非本机监听时必须明确警告并建议增加访问控制。

---

## 6. 第四阶段：配置向导和文档体系

### 6.1 实现 `XrayR config init`

交互式流程示例：

```text
$ XrayR config init

选择面板：
  1. Xboard
  2. SSPanel
  3. PMPanel
  4. ProxyPanel

面板地址: https://panel.example.com
API Key: ********
节点 ID: 12
节点协议:
  1. VLESS
  2. VMess
  3. Trojan
  4. Shadowsocks

正在检测面板连接……
✓ API 地址可访问
✓ API Key 有效
✓ 找到节点 12
✓ 节点协议：VLESS
✓ 配置已写入 /etc/XrayR/config.yml
```

支持非交互模式：

```bash
XrayR config init \
  --panel xboard \
  --api-host https://panel.example.com \
  --api-key "$API_KEY" \
  --node-id 12 \
  --node-type Vless \
  --output /etc/XrayR/config.yml
```

#### 功能要求

- API Key 使用隐藏输入；
- 输出文件已存在时不得直接覆盖；
- 覆盖前显示差异并要求确认；
- 支持只生成配置而不访问面板；
- 生成完成后自动执行配置校验；
- 非交互模式适合自动部署。

### 6.2 文档进入主仓库

所有用户文档统一维护在主仓库 `docs/` 中，避免代码和文档长期分离。

建议采用 MkDocs、VitePress 或 Docusaurus 构建文档站，并通过 CI 自动发布。

#### 文档要求

- 每个面板都有独立对接指南；
- 每个配置字段都有类型、默认值、单位和示例；
- 每个常见错误都有排查步骤；
- 配置示例由 CI 自动解析和校验；
- 文档中的字段必须能在代码中找到；
- Release 流程检查文档和示例是否与当前版本一致；
- 尽可能由 Go 配置结构生成参数表，减少手工同步。

### 6.3 Web 管理界面暂缓

在以下基础能力完成前，不建议优先开发 Web UI：

- 统一配置模型；
- 配置校验；
- 配置迁移；
- 诊断工具；
- 原子更新和回滚；
- 稳定的状态接口。

基础能力完成后，Web UI 可以安全地复用已有命令和内部服务，而不是重新实现一套配置逻辑。

---

## 7. 推荐实施顺序

### 里程碑 1：配置可验证

1. 面板注册表和别名归一化；
2. 统一配置校验器；
3. `XrayR config check`；
4. 友好错误信息；
5. 最小、常用和完整配置模板；
6. 配置示例 CI 校验。

完成标志：大部分配置错误可以在启动前一次性发现。

### 里程碑 2：问题可诊断

1. `XrayR config show`；
2. `XrayR doctor`；
3. API 错误分类；
4. 日志敏感信息脱敏；
5. 启动前环境预检。

完成标志：用户无需开启大量 Debug 日志即可定位常见故障。

### 里程碑 3：更新可恢复

1. API 能力接口拆分；
2. `ConfigVersion`；
3. `XrayR config migrate`；
4. 配置原子更新；
5. 最后有效配置缓存；
6. 更新失败自动回滚。

完成标志：面板错误配置或短暂不可用不会轻易导致节点整体下线。

### 里程碑 4：运维可观测

1. 结构化日志；
2. 健康检查；
3. 就绪检查；
4. Prometheus 指标；
5. 脱敏状态接口；
6. 文档站自动构建和发布。

完成标志：可以稳定接入 systemd、Docker、Kubernetes 和常见监控系统。

### 里程碑 5：配置更简单

1. `XrayR config init`；
2. 非交互式配置生成；
3. 覆盖前差异预览；
4. 配置向导与 `doctor` 联动；
5. 评估 Web 管理界面。

完成标志：新用户无需从完整示例中手工删改大量字段即可完成部署。

---

## 8. 第一项建议开发任务

建议首先实现：

> 面板注册表、统一配置校验包和 `XrayR config check`。

这项工作可以同时解决：

- 面板名称和历史别名混乱；
- 配置错误只能在启动后发现；
- 报错缺少字段位置和修复建议；
- 配置模板无法自动验证；
- 后续 `config init`、`config show` 和 `doctor` 缺少公共基础。

### 建议拆分

1. 定义面板注册信息和能力结构；
2. 将现有面板创建逻辑迁移到注册表；
3. 保持全部历史 `PanelType` 兼容；
4. 定义结构化校验结果；
5. 实现纯本地配置校验；
6. 添加 `config check` 子命令；
7. 为错误输出、JSON 输出和退出码增加测试；
8. 将示例配置加入 CI 校验。

### 完成定义

- 现有有效配置仍能正常加载；
- 错误配置能返回全部问题；
- 错误信息包含字段路径和建议；
- CLI 支持文本和 JSON 输出；
- CI 能验证所有仓库内配置示例；
- 不影响 `.github/ISSUE_TEMPLATE/` 及现有 Issue 模板。
