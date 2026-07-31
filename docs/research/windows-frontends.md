# Windows 开源语音输入前端调研

## 研究记录

- **问题/范围：** 哪些开源桌面前端可参考 Windows 全局语音输入、Provider 配置和文本交付；不将模型推理库误当作前端。
- **日期：** 2026-07-26；阶段 1 技术路线采用更新：2026-08-01。
- **来源：** 下表的 GitHub 仓库 README、许可证文件和仓库元数据快照；来源优先级低于后续实现时的官方 Windows/API 文档。
- **关键证据/定位：** “证据定位”列为当日 README 的功能段或 GitHub `updated_at`/许可证元数据；不是未固定 commit 的源码保证。
- **结论：** 下列项目仅保留为交互和风险参考，不再作为阶段 1 技术栈候选。阶段 1 已采用原生 Win32 popup + GDI、no-activate overlay；不采用 Fyne、Wails/WebView，也不在本阶段引入完整托盘/GUI 框架。
- **适用边界：** 只借鉴设计和验证用例，不复制源码、不引入依赖；GitHub 活跃度不是安全性或生产成熟度证明。
- **会话 ID：** `ses_063cf0a78ffeGg0eNJsIrZ8ccZ`。
- **子代理类型：** scout。

## 候选矩阵

“直接配置”表示用户可直接填写/选择兼容端点、Base URL 或模型；“适配器”表示需要通过该项目既有 Provider/插件协议接入。二者都不代表 VoxInk 已兼容。

| 项目 | 接自有模型等级 | Windows 全局输入方式 | 可参考点 | 限制 | 许可证与当时维护状态 | 证据定位 |
| --- | --- | --- | --- | --- | --- | --- |
| [VoiceOnWindows](https://github.com/thiago4go/VoiceOnWindows) | 直接配置：OpenAI-compatible multipart 或 OpenRouter Base64 JSON，配置 `Endpoint`/`Model` | 默认 `Ctrl+Space`；剪贴板复制后粘贴 | 极小的端点/模型/快捷键配置面；把转写和粘贴分开 | 完整录音后上传；剪贴板/粘贴仍受焦点和权限影响 | MIT；GitHub `updated_at` 2026-05-25 | README “Features”“Configuration”“API formats” |
| [better-voice-typing](https://github.com/Elevate-Code/better-voice-typing) | 适配器：OpenAI 与 `custom` STT，支持 OpenAI-compatible、`/transcribe`、`/api/transcribe` | 面向 Windows Voice Typing 使用场景；托盘/可配置快捷键，文本通过输出 Provider 交付 | 自定义 STT 端点、历史/日志位置、输出插件和剪贴板恢复风险 | Python 桌面应用；默认/示例依赖云端和本地设置文件，README 记录上传大小限制 | MIT；GitHub `updated_at` 2026-07-13 | README “Custom Speech-to-Text Servers”“Plugins (Output Providers)” |
| [lbwalton/murmur](https://github.com/lbwalton/murmur) | 直接配置：可改 Base URL、模型，支持本地 OpenAI-compatible Whisper server | Windows 按住 Right Ctrl 或可重绑 toggle；Electron global shortcut/键盘 hook；剪贴板粘贴或键入 | 明确状态机（idle/listening/processing）、透明 overlay、剪贴板恢复、DPAPI | Electron/原生 hook 供应链较重；录音后批量提交而非实时 ASR | MIT；GitHub `updated_at` 2026-07-24 | README “General”“Architecture”“Troubleshooting” |
| [Terminal-Talk](https://github.com/Netropolitan/Terminal-Talk) | 直接配置：Whisper-compatible STT server，远程模型选择/API key | 可配置热键；源运行依赖 AutoHotkey v2，定位为 Windows 任意处听写 | 小型 Windows 交互、SAPI 与兼容 Whisper server 双路径 | 仓库未声明 SPDX 许可证；AutoHotkey/输出实现和安全边界均须另验 | 未见许可证声明；GitHub `updated_at` 2026-02-12 | README “STT”“Hotkeys”“Requirements”“License” |
| [OpenTypeless](https://github.com/tover0314-w/opentypeless) | 直接配置：多 STT/LLM Provider、Whisper-compatible、custom endpoint/BYOK | Windows 默认 Right Alt 及组合键；键盘模拟、clipboard、Windows `SendInput` | Provider 路由、按应用工作流、overlay、失败诊断和 `Copy Only` | 跨平台 Tauri 产品面较大；其 Provider/云模式不是 VoxInk 协议结论 | MIT；GitHub `updated_at` 2026-07-26 | README “Platform support”“STT provider choice”“Output”“Architecture” |
| [TypeWhisper for Windows](https://github.com/TypeWhisper/typewhisper-win) | 适配器：插件系统；OpenAI-compatible server 和本地/云引擎 | system-wide dictation 热键，向任意 app paste；WPF/托盘 | 清晰插件契约、引擎/模型枚举、工作流与历史边界 | 产品和依赖面大；GPLv3/商业许可须先评估，不能直接复用实现 | GPLv3（另有商业许可说明）；GitHub `updated_at` 2026-07-24 | README “System-wide dictation”“Plugin system”“License” |
| [OpenWhispr](https://github.com/OpenWhispr/openwhispr) | 适配器：OpenAI-compatible 以外可用 custom ASR shim | global hotkey 后自动粘贴到任意 app | 本地/云切换、custom ASR shim、公开 API/MCP 的边界拆分 | Electron + 多功能笔记/Agent 产品，远超 MVP；需核查其 output 安全策略 | MIT；GitHub `updated_at` 2026-07-26 | README “Voice dictation”“Custom ASR shim”“Technology” |

## 事实、工程判断与未确认项

### 已确认事实

- 上表的端点/模型配置与输入方式均来自指定仓库 README 的当日快照，而非 VoxInk 的实现。
- 多数候选用全局热键、系统级 key hook、键盘模拟或剪贴板粘贴完成“全局输入”；这不等于 TSF Text Service，也不保证可越过 UIPI/UAC。

### 工程判断与项目采用

- 所有候选仅用于提取状态、overlay、失败诊断和输出风险用例，不构成等价推荐，也不原样移植其框架或实现。
- 阶段 1 直接使用 Win32 热键和 popup/GDI overlay，以最小消息循环验证 no-activate 行为；Fyne、Wails/WebView 及 Electron/Tauri/WPF 路线均不采用。
- 外部 Windows 路线主记录为记忆 `#352`，核验日期 2026-08-01。

### 未确认项

- 未固定这些仓库的源码 commit，也未复现其在管理员窗口、密码框、RDP 或不同 UI 框架中的注入行为。
- “更新日期”只表示 GitHub 元数据，不代表 release 健康度、维护承诺或可用许可证兼容性；采纳前必须重查。
