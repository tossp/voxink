# VoxInk（声墨）

[![CI](https://github.com/tossp/voxink/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/tossp/voxink/actions/workflows/ci.yml?query=branch%3Amain)

徽章仅显示 `main` 上 CI workflow 的运行状态；各分支结果以对应 Actions run 为准。CI 上传的是有保留期限的未签名验证构建，不是永久 release。

VoxInk 是计划中的 Windows 跨应用语音输入工具。

## 项目计划

未来目标是：用快捷键开始语音输入，显示识别中的文本，并在安全策略允许时将最终文本交付到当前应用。

这是完整产品目标；当前只实现下述自动化预览范围。

## 当前状态

`v0.1.0-alpha.1` 是准备发布的 Windows amd64 未签名自动化预览版。它是 prerelease，不是 stable 或正式可用产品，也尚未完成真实 Windows 设备、真实供应商账户和端到端验收。

当前代码已实现带 `SessionID` 门控的六态会话、500/600 ms 与 15/60 秒切段、火山 live、MiMo batch fallback、Windows WASAPI 采集、默认 `Ctrl+Shift+Space` 热键、no-activate overlay、脱敏诊断和 Linux/Windows CI。当前只有 overlay 展示，仍无文本注入、Clipboard、SQLite、tray、设置 UI、签名或安装器。

详细能力、隐私边界和已知限制见 [v0.1.0-alpha.1 发布说明](docs/releases/v0.1.0-alpha.1.md)与 [CHANGELOG](CHANGELOG.md)。

## 预览下载与运行前提

发布后可在 [GitHub Releases](https://github.com/tossp/voxink/releases) 获取未签名的 `voxink-windows-amd64.exe` 及其 `.sha256` 文件；不要把有保留期限的 Actions artifact 当作永久 release。当前没有安装器，请在 Windows x64 上直接运行 exe。

运行前需在本机设置环境变量：火山使用 `VOXINK_VOLC_API_KEY` 与 `VOXINK_VOLC_RESOURCE_ID`，MiMo 使用 `VOXINK_MIMO_API_KEY`；火山 legacy 认证可改用 `VOXINK_VOLC_APP_KEY`、`VOXINK_VOLC_ACCESS_KEY` 与 `VOXINK_VOLC_RESOURCE_ID`。不要将任何值写入仓库、issue 或日志。

## 开发进度

开发状态与后续计划见 [开发路线图](docs/roadmap.md)。

当前权威文档采用已确认的设计草案 v0.4；已实现范围和仍属后续目标的边界以正式文档、发布说明和代码为准。

## 参与贡献

提交 issue、实现、Pull Request 或发布前，请先阅读唯一入口规则文件 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 许可证

尚未选择许可证，仓库目前没有 `LICENSE`。预览版不能解读为已经授予任何特定开源许可证；本说明不提供法律结论。相关决策见 [#24](https://github.com/tossp/voxink/issues/24)。
