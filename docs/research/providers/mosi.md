# MOSI 转写与结果流协议证据

## 研究记录

- **问题/范围：** 记录 MOSI 音频转写提交方式、同步/异步任务、Diarize SSE 的触发条件与边界。
- **日期：** 2026-07-26；官方协议复核与项目采用更新：2026-08-01。
- **来源：** [场景：转写](https://platform.mosi.cn/docs/scenarios/transcribe)、[Transcriptions API](https://platform.mosi.cn/docs/reference/transcriptions)、[认证](https://platform.mosi.cn/docs/getting-started/auth)、[Files](https://platform.mosi.cn/docs/reference/files-create)、[异步任务](https://platform.mosi.cn/docs/getting-started/async-tasks)、[限制](https://platform.mosi.cn/docs/errors-and-limits/limits)、[Diarization](https://platform.mosi.cn/docs/scenarios/transcribe-diarization)、[错误码](https://platform.mosi.cn/docs/errors-and-limits/error-codes)。
- **关键证据/定位：** 上述 API 的 endpoint、输入互斥字段、任务和 SSE event 名称。
- **结论：** MOSI 常规转写是完整音频 batch，不是实时 audio ingress；VoxInk 将 `moss-transcribe` 延后到阶段 3，异步查询 endpoint 的官方冲突在真实账户核验前不写死。
- **适用边界：** 仅列公开文档已确认字段；不保存 token、不调用 API，不推断 codec/采样率/时长/大小等未明确公开的限制。
- **会话 ID：** `ses_063c65e43ffe8fV3cak11ijJ7W`。
- **子代理类型：** explorer。

## 已确认接口边界

| 主题 | 事实 |
| --- | --- |
| Endpoint 与认证 | `POST https://api.mosi.cn/v1/audio/transcriptions`；使用 Bearer 认证，详见官方 auth 页 |
| 音频来源 | `file`、`file_id`、`url`、`audio_url` 四选一；公开接口不支持 Base64/inline audio |
| 常规模型 | `moss-transcribe` 支持同步/异步的完整音频 batch；异步路径应按任务状态页轮询/获取结果 |
| 任务状态 | 官方页面分别出现 `/v1/audio/transcriptions/{task_id}` 与 `/v1/audio/tasks/{task_id}` 查询路径；账户核验前保持未确认，不能把任一路径写死 |
| Diarize 的 SSE 前提 | 仅 `moss-transcribe-diarize` 且 `version=v20260410-streamparam-20260703` 与 `stream=true` 时有 SSE |
| SSE 事件 | `task.created`、`transcript.text.delta`、`transcript.segment.done`、`transcript.text.done` |
| 错误处理 | 按 [error codes](https://platform.mosi.cn/docs/errors-and-limits/error-codes) 的类型和 HTTP/业务错误信息处理；不将未知字段写死 |

## 生命周期与事件映射

```text
完整 file/file_id/url/audio_url
  -> POST transcriptions
  -> 同步完整结果，或异步 task 状态
  -> （仅指定 diarize 版本 + stream=true）SSE 结果事件
```

- `transcript.text.delta` 可映射为**提交完整音频后的结果增量**，不是 capture 期间的 `Partial`。
- `transcript.segment.done` 是一个完成的转写 segment；`transcript.text.done` 是结果文本完成事件。它们仍不意味着客户端可在录音过程中持续上传音频。
- 产品代码应把这条能力登记为 `RecognitionBatch`；如果 UI 想显示 result streaming，需要显式标为“转写处理中”，不能伪装为实时听写。

## 未确认项与实现前复核

- 公开页面未在本轮证据中明确给出可安全写入设计的 codec、采样率、声道、时长、文件大小或所有任务/错误字段。它们必须保持“未确认”，不能补猜。
- 实现前重读 limits、错误码、auth 与 transcriptions 页面，确认模型/version 是否仍可用、任务状态词和值、上传限制以及数据处理条款。
- 本轮没有执行任何 MOSI 请求，也没有处理凭据。
- 项目采用边界是阶段 3 再做账户 PoC；在此之前不进入阶段 1 主备，也不把异步路径描述成已确定。
- 外部官方协议主记录为记忆 `#351`，核验日期 2026-08-01。
