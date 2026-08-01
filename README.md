# VoxInk（声墨）

[![CI](https://github.com/tossp/voxink/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/tossp/voxink/actions/workflows/ci.yml?query=branch%3Amain)

徽章仅显示 `main` 上 CI workflow 的运行状态；各分支结果以对应 Actions run 为准。CI 上传的是保留 14 天的未签名验证构建，不是 release。

VoxInk 是计划中的 Windows 跨应用语音输入工具。

## 项目计划

未来目标是：用快捷键开始语音输入，显示识别中的文本，并在安全策略允许时将最终文本交付到当前应用。

这是项目目标，不是当前已实现的功能。

## 现在能用吗？

**目前没有可用版本。** 项目仍处于设计与早期脚手架阶段。
当前无法安装、配置或使用 VoxInk；录音、语音识别、文本输入、托盘和界面均未实现。

## 开发进度

开发状态与后续计划见 [开发路线图](docs/roadmap.md)。

当前权威文档采用已确认的设计草案 v0.4：未来以 `AsrVendor` 供应商中心承载配置、能力和引擎选择，语音转文字只配置不同的主、备用供应商；非流式路径使用会话内存音频、停顿渐进切段和单一 FIFO 识别。以上均为实现目标，当前代码尚未提供这些能力。

## 参与贡献

提交 issue、实现、Pull Request 或发布前，请先阅读唯一入口规则文件 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 许可证

尚未选择许可证；在作出选择前，用户的使用权利尚未明确。
