# Changelog

本文件记录 VoxInk 的版本化变更，格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)。

## [Unreleased]

## [0.1.0-alpha.1] - 2026-08-01

### Added

- 首个 Windows amd64 未签名自动化预览；这是 prerelease，不是 stable 或正式可用产品。
- 实现带 `SessionID` 迟到事件门控的六态会话，以及 500 ms 最短语音、600 ms 静音、15 秒连续语音和 60 秒会话上限切段。
- 实现火山 live 主路径、MiMo batch fallback、Windows WASAPI 采集、默认 `Ctrl+Shift+Space` 全局热键和 no-activate overlay。
- 实现不记录正文、音频、凭据和原始错误的有界结构化脱敏诊断。
- 增加 [CI workflow](https://github.com/tossp/voxink/actions/workflows/ci.yml)：Linux test/race/vet，以及 Windows 2025、`CGO_ENABLED=1` 的 test/vet/未签名 build。

### Known limitations

- 尚未完成真实 Windows 设备与交互、火山账户、MiMo 账户和端到端验证，见 [#9](https://github.com/tossp/voxink/issues/9)、[#11](https://github.com/tossp/voxink/issues/11)、[#12](https://github.com/tossp/voxink/issues/12)、[#13](https://github.com/tossp/voxink/issues/13)。
- 不含文本注入、Clipboard、SQLite、tray、设置 UI、代码签名或安装器；许可证与正式发布自动化仍未决，见 [#24](https://github.com/tossp/voxink/issues/24)、[#25](https://github.com/tossp/voxink/issues/25)。仓库目前没有 `LICENSE`。
- 发布附件将包含 `.sha256` 文件；实际摘要由最终 CI artifact 生成后加入 GitHub release body/附件，本文件不预写占位摘要。

[Unreleased]: https://github.com/tossp/voxink/compare/v0.1.0-alpha.1...HEAD
[0.1.0-alpha.1]: https://github.com/tossp/voxink/releases/tag/v0.1.0-alpha.1
