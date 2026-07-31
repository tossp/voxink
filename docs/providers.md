# Provider 能力与接入边界

本文仅记录已确认的能力边界和官方资料入口，不保存密钥、不列出计费结论，也不构成运行时接入实现。具体请求字段、限制与服务可用性必须在实现阶段以官方文档复核。

## 供应商中心与配置边界

- 后续实现以 `AsrVendor` 供应商中心承载配置、能力声明和引擎选择；具体模型仅作为供应商配置。当前代码只有稳定的 Provider/Session 领域类型，本节是实现目标和迁移边界。
- 阶段 1 固定火山 V3 true streaming 为主供应商，目标 endpoint variant 为 `bigmodel_async`；固定 MiMo `mimo-v2.5-asr` batch 为备用供应商。默认不并行双发；主失败后，备用接收同一段会话内存 PCM，转成 WAV 后以非流式 POST 提交。主备均失败时必须返回明确错误。
- MiMo `mimo-v2.5` 保持独立的 Audio Understanding capability，不作为默认 ASR，也不复用 `mimo-v2.5-asr` 的请求或验收语义。

## 能力矩阵

| Provider / 模型 | 分层 | 音频提交方式 | 结果返回 | 是否 true streaming ASR | VoxInk 角色 |
| --- | --- | --- | --- | --- | --- |
| 火山 V3 `bigmodel_async` | Streaming ASR | WebSocket 中持续提交音频 | 识别过程中返回结果 | 是 | 阶段 1 主供应商 |
| MiMo `mimo-v2.5-asr` | File/Batch ASR | 完整段 PCM 在内存转 WAV 后提交 | 非流式完整结果 | 否 | 阶段 1 备用供应商 |
| MOSI `moss-transcribe` | Batch ASR | 完整文件、`file_id` 或 URL | 同步结果或异步任务 | 否 | 延后阶段 3；查询 endpoint 待账户核验 |
| MiMo `mimo-v2.5` | Audio Understanding | URL/Base64 `input_audio` 与 text prompt | 完整音频后的 Chat 响应 | 否 | 非默认 ASR；阶段 4 独立评估 |

## true streaming 与 result streaming

**true streaming ASR** 指客户端能在收音尚未结束时持续送入音频，服务端也在识别过程中返回文本；火山 V3 的 WebSocket 属于此类，适合实时输入。

**result streaming** 指请求所需的完整音频或完整文件已经提交，服务端再以分块、SSE 或其他流形式逐步返回生成结果。MiMo 的完整 Base64 请求及 MOSI Diarize 的完整文件提交后 SSE 都属于此类。结果可以早显示，但不能消除“先取得完整音频”的边界。

Streaming、File/Batch 与 Audio Understanding 是不同能力。供应商能力和具体模型配置决定引擎路径；适配器可以共享会话、错误和事件边界，但不得为了统一接口而伪造相同的协议请求。非流式/文件路径遵循 500ms 最短自动段、600ms 连续静音、15 秒无停顿上限和 60 秒会话上限，并以单一 FIFO 顺序提交音频段。

内部网络契约相应分为 **live recognizer** 与 **completed-segment transcriber**。火山连接在捕获期间接收音频帧和末包，不能实现或伪装成现有 batch `SegmentTranscriber`；MiMo 备用只接收完整 PCM 段。火山返回的 `utterances[].definite=true` 仅表示稳定分句，可用于累积显示；只有停止捕获、发送末包并收到协议终态后，session controller 才为当前 `SessionID` 产生唯一完整 `Final`。

## 官方文档入口（2026-08-01 核验）

- 火山引擎： [流式语音识别 WebSocket](https://docs.volcengine.com/docs/6561/1354869?lang=zh)；目标采用优化双向 `bigmodel_async`。具体认证 header/resource ID 必须按真实账户控制台确认。
- 小米 MiMo： [语音识别（MiMo-V2.5-ASR）](https://mimo.mi.com/docs/zh-CN/api/audio/Speech-Recognition)；另见 [使用指南](https://mimo.mi.com/docs/zh-CN/quick-start/usage-guide/audio/Speech-Recognition) 和 [Audio Understanding](https://mimo.mi.com/docs/zh-CN/quick-start/usage-guide/multimodal-understanding/audio-understanding)。
- MOSI： [场景：转写](https://platform.mosi.cn/docs/scenarios/transcribe)；详细 API 入口见 [Transcriptions API](https://platform.mosi.cn/docs/reference/transcriptions)。公开页面对异步查询路径存在冲突，真实账户核验前不固定 endpoint。

链接仅作为实现前复核起点。不得从本文推断未核实的认证方式、配额、价格、数据保留或 SLA。

详细协议证据见[MiMo](research/providers/mimo.md)、[火山 V3](research/providers/volcengine.md)和[MOSI](research/providers/mosi.md)。最新官方协议主记录为外部研究记忆 `#351`，核验日期 2026-08-01；BiBi 固定 commit `741fbb15df7041d11122e43ef5053ff8ff6642b8` 仅作架构参考，见[BiBi 参考](bibi-keyboard-reference.md)。这些采用项保持 v0.4 的切段参数、单主单备和阶段范围不变，当前尚未实现或通过真实账户验证。
