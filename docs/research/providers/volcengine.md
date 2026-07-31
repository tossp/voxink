# 火山 V3 大模型流式 ASR 证据

## 研究记录

- **问题/范围：** 固定火山 V3 BigModel ASR 的 true streaming 边界、partial/final 映射和 BiBi 实现参考位置。
- **日期：** 2026-07-26；官方协议复核与项目采用更新：2026-08-01。
- **来源：** [火山引擎流式语音识别 WebSocket](https://docs.volcengine.com/docs/6561/1354869?lang=zh)（协议权威）；下文 BiBi 固定 commit（实现参考）。
- **关键证据/定位：** 官方文档的 WebSocket 双向流、音频帧建议与 `definite` 结果语义；BiBi 分帧/回调代码行号。
- **结论：** V3 BigModel ASR 是 WebSocket true streaming。VoxInk 阶段 1 采用优化双向 `wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async`；`definite` 是稳定分句信息，不等于 VoxInk Session `Final`。
- **适用边界：** 官方协议优先于参考代码。本文不补猜未确认 request/header/错误字段、凭据、配额或数据处理条款。
- **会话 ID：** `ses_063c65e43ffe8fV3cak11ijJ7W`。
- **子代理类型：** explorer。

## 协议语义

官方页面同时列出 `bigmodel`、优化双向 `bigmodel_async` 与 `bigmodel_nostream`；项目只选择 `bigmodel_async` 作为阶段 1 目标 variant。公开资料存在新旧控制台认证 header 体系，具体 header 和 `X-Api-Resource-Id` 必须由真实账户控制台确认，文档不得预填或混用。

| 事实 | VoxInk 事件含义 |
| --- | --- |
| WebSocket 可在 capture 尚未结束时连续传送音频帧 | `RecognitionStreaming`，可作为实时输入路径 |
| 官方建议约 100–200 ms 的音频帧 | 这是实现期 framing 起点，须按当时协议/网络测试复核 |
| `definite=false` | 未稳定增量：可在 overlay 显示，不应立即交付文本 |
| `definite=true` | 稳定分句：可累积为稳定文本，但仍不是 Session `Final` |
| 停止捕获、发送末包、服务端协议终态/最终结果 | session final：三者满足后，当前 `SessionID` 才产生唯一完整 `Final` |

因此，VoxInk 的“会话 final”与服务端“断句/segment final”是两层：前者控制一次热键会话和最终交付，后者只描述一个可确定的识别片段。实现时需要保存 session 边界，避免先到的断句或旧会话消息误投递。

## BiBi Keyboard 固定快照（仅实现参考）

来源：[BryceWG/BiBi-Keyboard @ `741fbb15df7041d11122e43ef5053ff8ff6642b8`](https://github.com/BryceWG/BiBi-Keyboard/tree/741fbb15df7041d11122e43ef5053ff8ff6642b8)。

| 文件与行号 | 可参考点 |
| --- | --- |
| [`VolcStreamAsrEngine.kt:36`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/asr/VolcStreamAsrEngine.kt#L36) | 类注释记录 WebSocket、先发 full request、按约 200 ms 分包。 |
| [`VolcStreamAsrEngine.kt:345`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/asr/VolcStreamAsrEngine.kt#L345) | `chunkMillis = 200`，是实现选择，不是替代官方建议。 |
| [`VolcStreamAsrEngine.kt:573-599`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/asr/VolcStreamAsrEngine.kt#L573-L599) | 以服务器 final flag 分派 `onFinal`/`onPartial`。 |
| [`VolcStreamAsrEngine.kt:493-506`](https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/java/com/brycewg/asrkb/asr/VolcStreamAsrEngine.kt#L493-L506) | 音频帧发送和最后一帧标记处理的参考。 |

## 工程判断与未确认项

- **工程判断：** 火山必须实现独立 live recognizer 生命周期，不能伪装成接收完整 PCM 的 batch `SegmentTranscriber`。保留 `definite` 原始语义，再由 session controller 累积文本并门控唯一 `Final`。
- **未确认项：** 真实账户所属认证体系、完整 Upgrade headers、resource ID、会话上限与断连/计费语义；完成账户 PoC 前不得补猜。
- **外部记忆：** 官方协议主记录 `#351`，核验日期 2026-08-01。
