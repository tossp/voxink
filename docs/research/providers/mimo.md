# MiMo 音频能力与分类纠偏

## 研究记录

- **问题/范围：** 区分 `mimo-v2.5-asr` 与 `mimo-v2.5` 的官方定位、请求生命周期和 VoxInk 采用角色。
- **日期：** 2026-07-26；官方协议复核与项目采用更新：2026-08-01。
- **来源：** [MiMo Speech Recognition](https://mimo.mi.com/docs/zh-CN/quick-start/usage-guide/audio/Speech-Recognition)、[MiMo Audio Understanding](https://mimo.mi.com/docs/zh-CN/quick-start/usage-guide/multimodal-understanding/audio-understanding)，以及下文固定 BiBi 快照。
- **关键证据/定位：** 官方页面的模型分类/请求示例；BiBi `MiMoFileAsrEngine.kt` 与设置页的固定行号。
- **结论：** `mimo-v2.5-asr` 是专用完整音频 batch ASR，并采用为阶段 1 备用：完整段 PCM 在内存转 WAV 后，向 Chat Completions endpoint 非流式 POST。`mimo-v2.5` 保持 Audio Understanding，不作为默认 ASR。
- **适用边界：** 两者都不是实时麦克风 audio ingress。本文不保存 endpoint、密钥、配额、价格或未经官方确认的格式/质量结论。
- **会话 ID：** `ses_063a7faaaffeVXEClX7wii4H26`。
- **子代理类型：** explorer。

## 官方能力边界

| 模型 | 官方分类与输入 | 结果流语义 | VoxInk 分类 |
| --- | --- | --- | --- |
| `mimo-v2.5-asr` | 专用 Speech Recognition；提交完整 Base64 WAV，`asr_options.language` 指定语言 | 阶段 1 固定 `stream=false`，读取完整结果 | completed-segment transcriber；阶段 1 备用 |
| `mimo-v2.5` | Audio Understanding；`input_audio` 可为 URL 或 Base64，并带 text prompt | 没有实时接收麦克风帧的证据 | 独立 `AudioUnderstanding` capability；非默认 ASR |

### 不能混淆的事实

1. `stream=true` 不会改变“先有完整音频”的请求前提，因此不是实时 ingress。
2. `mimo-v2.5` 的 Audio Understanding 分类不能被改写成“官方保证的专用 ASR”；反过来，也不能据此绝对断言它不能按准确转写 prompt 返回文本。
3. 阶段 1 只采用 `mimo-v2.5-asr` 的 WAV 非流式路径；可用性、最大时长、超时、错误映射和真实账户权限仍须受控测试。

## BiBi Keyboard 固定快照（辅助实现证据）

来源：[BryceWG/BiBi-Keyboard @ `741fbb15df7041d11122e43ef5053ff8ff6642b8`](https://github.com/BryceWG/BiBi-Keyboard/tree/741fbb15df7041d11122e43ef5053ff8ff6642b8)。这是参考实现，不是协议权威。

| 文件与行号 | 可复核事实 |
| --- | --- |
| [`MiMoFileAsrEngine.kt:40-45`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/asr/MiMoFileAsrEngine.kt#L40-L45) | 定义默认 `mimo-v2.5-asr` 与可选 `mimo-v2.5`；后者使用“准确转写、直接输出结果”的固定 user prompt。 |
| [`MiMoFileAsrEngine.kt:60-63`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/asr/MiMoFileAsrEngine.kt#L60-L63) | 用模型名区分 audio-understanding 路径，空设置默认专用 ASR。 |
| [`MiMoFileAsrEngine.kt:152-215`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/asr/MiMoFileAsrEngine.kt#L152-L215) | `input_audio` 组装、AU 文本 prompt 和专用 ASR 的 `asr_options.language` 分支。 |
| [`MiMoFileAsrEngine.kt:136`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/asr/MiMoFileAsrEngine.kt#L136) | 把解析到的 `content` 以 `onFinal(text)` 交给上层。 |
| [`AsrOnlineVendorConfigs.kt:517-520`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/ui/settings/compose/screens/AsrOnlineVendorConfigs.kt#L517-L520) | 设置显式列出 `mimo-v2.5-asr` 和 `mimo-v2.5`，默认前者。 |

该快照证明 BiBi 已把两模型置于同一“文件识别”适配器，并以 prompt 将后者用于转写；它**不证明**两模型质量等价，也不改变 MiMo 官方分类。

## 裁决、工程判断与未确认项

- **裁决：** `mimo-v2.5-asr` 作为阶段 1 completed-segment 备用；`mimo-v2.5` 只保留 Audio Understanding，延后独立评估，不再作为等价 ASR 推荐。
- **工程判断：** 输入完整段 PCM 必须先在内存编码为 WAV，再 Base64 放入请求；不落盘，不启用结果流，也不把它包装成 live recognizer。
- **未确认项：** 最大音频秒数、SSE/结果终态（若未来启用）、超时、错误映射和实际账户可用性；不得补猜。
- **外部记忆：** 官方协议主记录 `#351`，核验日期 2026-08-01。
