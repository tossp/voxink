# Provider 凭据

Windows cgo 构建可将当前 VoxInk 使用的 Provider 凭据保存到当前用户的 Windows Credential Manager。凭据优先于同名 `VOXINK_*` 环境变量；未保存对应项时仍兼容环境变量回退。

固定名称为：`volc-api-key`、`volc-resource-id`、`volc-app-key`、`volc-access-key`、`mimo-api-key`。Credential Manager target 使用 `VoxInk/<name>`，target 不包含凭据值。

```text
voxink config credential set <name>
voxink config credential delete <name>
voxink config credential list
voxink config credential list --json
```

`set` 只从 stdin 读取值，不接受命令行 value 参数；Windows 交互控制台会关闭输入回显。`list` 只显示固定名称及是否已配置，不显示值、长度或摘要。非 Windows 或 no-cgo 构建不支持这些管理命令，但现有环境变量回退与 `self-check` 不受影响。

不要将凭据粘贴到 Issue、Pull Request、日志、命令行参数或仓库文件中。

## 非敏感运行设置

VoxInk 只将当前程序已实际支持的五项非敏感设置保存到 `%AppData%/VoxInk/config.json`：`hotkey`、`volc-endpoint`、`volc-read-limit-bytes`、`mimo-endpoint`、`mimo-auth-mode`。该文件不保存任何凭据；运行时优先级为 settings 文件、环境变量、内置默认值，凭据优先级仍为 Credential Manager、环境变量。

```text
voxink config settings set <key> <value>
voxink config settings delete <key>
voxink config settings list
voxink config settings list --json
```

`delete` 删除文件中的对应覆盖并恢复环境变量或内置默认值。`hotkey` 支持 Ctrl/Alt/Shift/Win 修饰键加 A-Z、0-9、F1-F24 或 Space；默认仍为 `Ctrl+Shift+Space`。`volc-read-limit-bytes` 接受 65536–67108864，默认 1048576。Volcengine endpoint 必须使用 `wss`，MiMo endpoint 必须使用 `https`，且均不得包含 userinfo、query 或 fragment。`mimo-auth-mode` 只接受 `api-key` 或 `bearer`。设置 endpoint 时不会发起网络请求。

`list` 可在本机显示上述非敏感值；在 Issue、Pull Request 或其他公开反馈中粘贴输出前，仍应审阅自定义 endpoint 是否包含不应公开的内部主机名或路径。
