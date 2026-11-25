# XrayR Documentation

XrayR is an Xboard backend for Xray-core. This documentation focuses on safe configuration, validation, diagnosis, recovery, and operations.

## Recommended deployment flow

```bash
# 1. 安装（生成带注释的配置模板，不启动）
curl -fsSL https://raw.githubusercontent.com/Activity163/XrayR/master/release/install.sh -o install.sh
sudo bash install.sh install

# 2. 改配置里的 4 个字段：ApiHost / ApiKey / NodeID / NodeType
xrayr edit

# 3. 校验并启动
xrayr check
xrayr start

# 4. 需要时打开管理菜单
xrayr menu
```

`xrayr menu` 里可以启动 / 停止 / 查看日志 / 更新 / 开关开机自启 / 卸载。

> 不用脚本时，`XrayR config init` 仍然可以交互式生成配置，
> 之后用 `XrayR config check` 与 `XrayR doctor` 校验。

## Start here

- [Installation](installation.md)
- [Configuration](configuration.md)
- [CLI reference](cli.md)
- [Diagnostics](diagnostics.md)
- [Troubleshooting](troubleshooting.md)
