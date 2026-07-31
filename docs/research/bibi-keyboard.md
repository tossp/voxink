# BiBi Keyboard 固定参考证据

## 研究记录

- **问题/范围：** 提取可复用的事件、会话、Provider 和文本输入设计证据，并明确 Android 专属部分不可直接迁移。
- **日期：** 2026-07-26；commit/tag 关系与采用结论于 2026-07-31 核验。
- **来源：** [BryceWG/BiBi-Keyboard @ `741fbb15df7041d11122e43ef5053ff8ff6642b8`](https://github.com/BryceWG/BiBi-Keyboard/tree/741fbb15df7041d11122e43ef5053ff8ff6642b8)，仓库 Apache-2.0 许可证及 NOTICE 义务。该固定 commit 不是 release `v4.2.1`；`v4.2.1` tag 指向 `8d0fcebd3fe2abe4648d83666fe6329daedf7d68`，固定 commit 比该 tag 前进 7 个提交。
- **关键证据/定位：** 下表固定 commit 的文件/行号；不复制大段源码。
- **结论：** 可借鉴 Provider 事件接口、会话序号门控、流式/文件适配器和输入组合态的概念；VoxInk 必须用 Windows API/安全模型重写，不能移植 Android IME、权限或存储实现。
- **适用边界：** 固定 commit 只说明当时实现，不是火山/MiMo 协议权威；复制/改编任何代码前必须单独履行 Apache-2.0 和 NOTICE 义务。
- **会话 ID：** `ses_063bcc616ffemur9MKP2TnPHrH`。
- **子代理类型：** explorer。

## 固定快照定位

| 主题 | 文件与行号 | 可借鉴的事实 |
| --- | --- | --- |
| 事件接口 | [`asr/AsrEngine.kt:17-39`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/asr/AsrEngine.kt#L17-L39) | `onPartial`、`onFinal`、`onError`、`onStopped`、音量事件是显式 listener 回调。 |
| 会话状态/防迟到 | [`ime/AsrSessionManager.kt:121-153`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/ime/AsrSessionManager.kt#L121-L153)、[`666-777`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/ime/AsrSessionManager.kt#L666-L777) | `sessionSeq` 递增，`onFinal`/`onPartial`/`onStopped` 会检查当前 session，防止迟到回调污染。 |
| 火山流式适配 | [`asr/VolcStreamAsrEngine.kt:36`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/asr/VolcStreamAsrEngine.kt#L36)、[`345`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/asr/VolcStreamAsrEngine.kt#L345)、[`573-599`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/asr/VolcStreamAsrEngine.kt#L573-L599)；细节见[火山 V3 证据](providers/volcengine.md) | 类注释记录 WebSocket 和约 200 ms 分帧；`chunkMillis = 200` 是实现选择；服务器 final flag 分派 `onFinal`/`onPartial`。协议事实仍以火山官方文档为准。 |
| MiMo 文件适配 | [`asr/MiMoFileAsrEngine.kt:40-45,152-215`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/asr/MiMoFileAsrEngine.kt#L40-L45) | 默认专用 ASR、可选 audio-understanding 模型和 prompt-configured 完整音频请求。 |
| 历史/音频留存 | [`store/AsrHistoryAudioStore.kt:1-25,78-144`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/store/AsrHistoryAudioStore.kt#L1-L25) | 参考实现有 app-private/no-backup 目录和按保留策略存删音频的能力。 |
| 输入组合态 | [`ime/InputConnectionHelper.kt:22-69`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/ime/InputConnectionHelper.kt#L22-L69) | Android `InputConnection` 的 `setComposingText`、`finishComposingText`、`commitText` 区分预览和最终提交。 |

## 可借鉴、需 Windows 化、不可迁移

### 可借鉴

- `AsrVendorRegistry`、`AsrEngineModeResolver`：供应商承载配置、能力和引擎选择，模型属于供应商配置；区分 streaming、文件/批量及 Audio Understanding。
- `MiMoFileAsrEngine`：同一 MiMo 供应商内的 `mimo-v2.5-asr` 与 `mimo-v2.5` 可共享 endpoint/凭据，但请求体和语义不同。
- `AsrBackupPolicy`、`LazyLocalBackupAsrEngine`：不同主备供应商、主失败后以同一内存音频调用备用、默认不并行双发。
- `NonStreamingProgressiveChunking`：500ms 最短自动段、600ms 连续静音、15 秒无停顿上限、60 秒会话上限；停止/硬上限时非空短尾仍提交。
- 单一 FIFO 顺序识别与拼接，队列排空后才形成完整 `Final`；主备均失败不得静默丢段。
- 独立的 session sequence/状态机和迟到事件门控；VoxInk 已以 `SessionID` 表达相同原则。
- final 交付和历史留存必须明确生命周期、失败路径和留存策略。

以上采用结论的外部研究记忆索引为 `#347`、`#348`、`#349`；本文件只保留项目可追溯定位，不复制完整调研。

### 必须 Windows 化

- 用 Windows 前台窗口、焦点、UIPI、安全降级和 `SendInput`/剪贴板策略取代 Android 输入目标语义。
- 用 Windows overlay/托盘/设置和凭据机制取代 `InputMethodService`、`InputConnection`、Android 权限与 app-private storage。
- 音频留存默认策略必须由 VoxInk 隐私设计决定；不能因参考实现有存档能力而改变“默认不保存原始音频”。

### 不可直接迁移

- `InputMethodService`、`InputConnection` composition、Android Accessibility、生命周期、Gradle/Android 依赖与权限模型。
- 任何 Android UI、网络封装、存储实现和第三方代码；本仓库没有复制它们。

## 仍需实现前复核

- 固定 commit 与后续分支差异及所有 NOTICE 内容应在任何代码采纳前重新核验；当前参考继续锁定 `741fbb15df7041d11122e43ef5053ff8ff6642b8`，不改锁到 `v4.2.1` tag。
- 参考实现的安全性、Provider 请求字段、留存默认值和实际兼容性不自动适用于 Windows。
