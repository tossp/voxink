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
