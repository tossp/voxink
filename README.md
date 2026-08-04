# VoxInk（声墨）

[![CI](https://github.com/tossp/voxink/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/tossp/voxink/actions/workflows/ci.yml?query=branch%3Amain)

徽章仅显示 `main` 上 CI workflow 的运行状态；各分支结果以对应 Actions run 为准。CI 上传的是有保留期限的未签名验证构建，不是永久 release。

VoxInk 是计划中的 Windows 跨应用语音输入工具。

## 项目计划

未来目标是：用快捷键开始语音输入，显示识别中的文本，并在安全策略允许时将最终文本交付到当前应用。

这是完整产品目标；当前只实现下述自动化预览范围。

## 当前状态

最新发布状态及下载资产以 [GitHub Releases](https://github.com/tossp/voxink/releases) 为准。[`v0.1.0-alpha.1`](https://github.com/tossp/voxink/releases/tag/v0.1.0-alpha.1) 可作为已发布历史预览查阅。后续预览构建可提供 Windows amd64 未签名安装器和 portable ZIP，均包含托盘程序、CLI、自检与第三方 notices；本段不表示尚未列入 Release 的资产已经发布。所有预览都是 prerelease，不是 stable 或正式可用产品。

当前代码已实现带 `SessionID` 门控的六态会话、500/600 ms 与 15/60 秒切段、火山 live、MiMo batch fallback、Windows WASAPI 采集、默认 `Ctrl+Shift+Space` 热键、no-activate overlay、托盘控制、脱敏诊断、Windows per-user 安装器和 Linux/Windows CI。仍无 Clipboard、SQLite、设置 UI、签名或自动更新。

`v0.1.0-alpha.2` 的计划能力、隐私边界和已知限制见 [release notes 源](docs/releases/v0.1.0-alpha.2.md)；历史说明见 [v0.1.0-alpha.1](docs/releases/v0.1.0-alpha.1.md)，版本摘要见 [CHANGELOG](CHANGELOG.md)。

## 预览下载、安装与运行前提

资产可用时，选择 `voxink-windows-amd64-setup.exe` 进行当前用户安装，或选择 `voxink-windows-amd64.zip` 解压后 portable 运行；不要把有保留期限的 Actions artifact 当作永久 release。两种形式均未签名，Windows SmartScreen 可能显示警告或阻止运行。无参数运行 `voxink.exe` 启动托盘应用；`config`、`self-check`、`smoke` 等需要控制台输出的子命令必须使用 `voxink-cli.exe`。

安装器默认安装到 `%LocalAppData%\Programs\VoxInk`，不会要求管理员权限；桌面快捷方式是可选项且默认不创建。卸载只删除程序文件和快捷方式，刻意保留 `%AppData%\VoxInk` 用户设置及 Windows Credential Manager 中的 `VoxInk/*` 凭据，避免数据损失；如需清理凭据，请在卸载前使用 CLI 或之后通过 Windows 凭据管理器手动删除。

运行前需在本机设置环境变量：火山使用 `VOXINK_VOLC_API_KEY` 与 `VOXINK_VOLC_RESOURCE_ID`，MiMo 使用 `VOXINK_MIMO_API_KEY`；火山 legacy 认证可改用 `VOXINK_VOLC_APP_KEY`、`VOXINK_VOLC_ACCESS_KEY` 与 `VOXINK_VOLC_RESOURCE_ID`。不要将任何值写入仓库、issue 或日志。

## 分层自检

`self-check` 子命令不读取 Provider 凭据，也不调用 Provider。静态报告可在任意开发平台生成；音频与交互探针必须在相应的 Windows 环境运行：

```text
voxink-cli.exe self-check --mode=static --json
voxink-cli.exe self-check --mode=audio --duration=3s --json
voxink-cli.exe self-check --mode=interactive --timeout=15s --json
```

JSON 只写入 stdout，交互提示与参数错误写入 stderr。报告不会包含 PCM、设备名、窗口标题、路径、环境变量值、识别文本或原始错误；interactive 报告中的 `focus_confirmation_required` 仍须人工确认，不能由程序自动判定。完整说明见[分层自检](docs/self-check.md)。

## Provider 受控冒烟

只有用户明确提供已授权 WAV 并确认发送时，才会运行单一 Provider 冒烟；命令不采集麦克风、不触发 fallback，也不会在普通启动或 CI 中自动执行：

```text
voxink-cli.exe smoke volc --audio <authorized.wav> --confirm-send --json
voxink-cli.exe smoke mimo --audio <authorized.wav> --confirm-send --json
```

输入限制为 2 MiB、最长 60 秒的 PCM16 LE / 16 kHz / mono WAV。报告不包含路径、音频、识别文本、响应正文、endpoint、凭据或原始错误。真实运行前必须复核账户权限及当期供应商政策；mock 结果不能替代真实账户证据。完整说明见 [Provider 受控冒烟命令](docs/provider-smoke.md)。

## 开发进度

开发状态与后续计划见 [开发路线图](docs/roadmap.md)。

当前权威文档采用已确认的设计草案 v0.4；已实现范围和仍属后续目标的边界以正式文档、发布说明和代码为准。

## 参与贡献

提交 issue、实现、Pull Request 或发布前，请先阅读唯一入口规则文件 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 许可证

尚未选择许可证，仓库目前没有 `LICENSE`。预览版不能解读为已经授予任何特定开源许可证；本说明不提供法律结论。第三方组件的归属文本见 [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md)，技术审计见 [`docs/license-audit.md`](docs/license-audit.md)；二者都不是 VoxInk 自身的许可证。相关决策仍由 [#24](https://github.com/tossp/voxink/issues/24) 跟踪。
