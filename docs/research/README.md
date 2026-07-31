# 研究证据索引

## 目的与使用方式

本目录保存为 VoxInk 设计决策准备的**详细、可复核**研究证据；产品、架构和 Provider 摘要仍以 `docs/` 根目录中的正式文档为准。会话标识只用于追溯当时的调研上下文，不能替代下面列出的官方页面、固定 GitHub commit 或正式结论。实现前应先读对应正式文档，再按表中的“复核”项重新确认会变化的协议、许可证和维护状态。

**日期：** 2026-07-26（统一检索/整理日期）  
**问题/范围：** 补齐文档审计识别的 P0/P1 证据定位，不接入任何服务。  
**来源：** 本目录所列官方文档与固定 GitHub 快照。  
**关键证据/定位：** 下表链接到逐项证据和来源。  
**结论：** 研究层是摘要文档的可复核支撑，而不是新的产品承诺。  
**适用边界：** 不含密钥、价格、SLA、实际账户能力或运行时探测结果。

**会话 ID：** 用于追溯可读取的调研上下文，不替代本目录列出的官方页面、固定 GitHub commit 或正式结论。

| 主题 | 状态 | 正式文档 | 证据记录 | 会话 ID | 子代理类型 | 主要来源 | 实现前复核 |
| --- | --- | --- | --- | --- | --- | --- |
| Windows 开源前端 | 已整理 | [架构](../architecture.md) | [windows-frontends](windows-frontends.md) | `ses_063cf0a78ffeGg0eNJsIrZ8ccZ` | scout | 项目 README、GitHub 仓库 | 是：活跃度、许可证、实际注入路径 |
| Windows 文本输入 API | 已整理 | [Windows 文本输入边界](../windows-text-input.md) | [windows-apis](windows-apis.md) | `ses_063c8de7fffex4qpgSsnrzZRfU` | explorer | Microsoft Learn | 是：目标应用/权限兼容性 |
| Go 生态与架构判断 | 已整理 | [架构](../architecture.md) | [go-ecosystem](go-ecosystem.md) | `ses_063c8de7fffex4qpgSsnrzZRfU`（库事实）；`ses_063ca9428ffeExHxVeDp8CCiAD`（架构研判） | explorer（库事实）；oracle（架构研判） | GitHub 仓库、工程研判 | 是：库 API、Windows 支持和维护状态 |
| MiMo 模型分类 | 已纠偏 | [Provider 能力](../providers.md) | [providers/mimo](providers/mimo.md) | `ses_063a7faaaffeVXEClX7wii4H26` | explorer | MiMo 官方文档、BiBi 固定 commit | 是：模型能力、请求格式、等价性 |
| 火山 V3 流式协议 | 已整理 | [Provider 能力](../providers.md) | [providers/volcengine](providers/volcengine.md) | `ses_063c65e43ffe8fV3cak11ijJ7W` | explorer | 火山官方文档、BiBi 固定 commit | 是：协议字段和限额 |
| MOSI 转写协议 | 已整理 | [Provider 能力](../providers.md) | [providers/mosi](providers/mosi.md) | `ses_063c65e43ffe8fV3cak11ijJ7W` | explorer | MOSI 官方文档 | 是：版本、限制、错误码 |
| BiBi Keyboard 参考实现 | 已整理 | [BiBi Keyboard 设计参考](../bibi-keyboard-reference.md) | [bibi-keyboard](bibi-keyboard.md) | `ses_063bcc616ffemur9MKP2TnPHrH` | explorer | 固定 commit `741fbb15df7041d11122e43ef5053ff8ff6642b8` | 是：上游变更和许可证义务 |
| 文档缺口审计 | 已闭环 | 本索引及上述页面 | 本页“覆盖”段 | `ses_063a4fdffffecVz8Cc5zN4AOod` | auditor | 既有 VoxInk 文档 | 否；新增实现时再审计 |

## 覆盖

- P0：将 true streaming audio ingress 与完整音频后的 result streaming 分开记录；将 Windows partial 的正确语义和权限边界落到 Microsoft 官方资料。
- P1：固定 BiBi 参考的 commit/行号，补足前端选择、Go 库风险、MiMo/MOSI/火山协议定位和许可证边界。
- 本轮不改变 MVP 范围：没有真实 API、录音、GUI、数据库、第三方依赖或 Windows 注入实现。
