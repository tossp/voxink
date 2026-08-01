# Provider 受控冒烟命令

`voxink smoke` 为 [#11](https://github.com/tossp/voxink/issues/11) 与 [#12](https://github.com/tossp/voxink/issues/12) 准备显式、单 Provider、脱敏的真实账户验证入口。它不会在普通启动、CI 或后台流程中自动运行。

## 显式命令与退出码

```text
voxink smoke volc --audio <authorized.wav> --confirm-send --json
voxink smoke mimo --audio <authorized.wav> --confirm-send --json
```

- Provider 位置参数只允许 `volc` 或 `mimo`。
- `--audio` 必须指向用户已授权发送的本地 WAV；`--confirm-send` 必须为 true。缺少任一项都不会读取文件、配置或调用 Provider。
- `--timeout` 默认 `30s`，允许 `1s..120s`；`--json` 输出稳定 JSON，否则输出固定字段的人读文本。
- 参数错误退出 `2`，冒烟失败退出 `1`，通过退出 `0`。stdout 仅写报告；固定授权提示和参数错误写 stderr。
- 命令在普通阶段 1 配置、麦克风、overlay 和应用启动前分派。每次只构造并调用选中的 Provider，不启动 fallback，也不采集麦克风。

## 实际发送内容

- `mimo`：把授权 WAV 原样作为现有 `mimo-v2.5-asr` 请求的内存输入；请求构造期会产生 Base64，但不会写文件或报告。
- `volc`：从同一 WAV 的 `data` chunk 取得 PCM16 LE，以不超过 100 ms 的有界帧发送给现有火山 V3 live client，随后发送末包并等待协议终态。
- 输入必须是 RIFF/WAVE、PCM、16-bit little-endian、16 kHz、mono，并具有完整非空 `data` chunk。未知附加 chunk 可跳过，但 RIFF/chunk 长度、`fmt ` 与 `data` 必须完整一致；RIFF chunk 按规范偶数字节对齐，奇数长度 chunk 后须有一个不计入 chunk 长度的填充字节。
- 本地文件上限为 2 MiB，解出的 PCM 最长 60 秒。WAV/PCM 仅驻留进程内存；命令结束后释放引用，不创建临时文件。这里的“释放”不宣称物理安全擦除。

## 凭据与配置

只读取既有环境变量，不新增明文配置文件，也不输出变量值：

- 火山新认证：`VOXINK_VOLC_API_KEY`、`VOXINK_VOLC_RESOURCE_ID`；legacy 认证：`VOXINK_VOLC_APP_KEY`、`VOXINK_VOLC_ACCESS_KEY`、`VOXINK_VOLC_RESOURCE_ID`。
- MiMo：`VOXINK_MIMO_API_KEY`，以及既有可选 `VOXINK_MIMO_AUTH_MODE`、`VOXINK_MIMO_ENDPOINT`。
- 火山继续支持既有可选 `VOXINK_VOLC_ENDPOINT` 与 `VOXINK_VOLC_READ_LIMIT_BYTES`。选择 MiMo 时不检查火山配置，选择火山时不检查 MiMo 配置。

endpoint、认证字段、权限、配额、留存、训练使用、处理地域与删除政策必须以真实账户设置和运行当期供应商条款为准；本命令不作账户政策保证。

## 报告 schema 与脱敏边界

schema version `1` 顶层固定字段为 `SchemaVersion`、`Timestamp`、`Provider`、`Model`、`Status`、`Code`、`Metrics`，字段名保持 PascalCase。

`Metrics` 是闭合集合，只可能包含：

- `AudioDurationMS`、`AudioBytes`、`LatencyMS`、`EventCount`；
- `FinalReceived`、`ProtocolTerminal`；
- 底层可靠提供 HTTP 状态时的 `HTTPStatusClass`（仅 `4xx` 或 `5xx`）。

报告和固定文本均不得包含识别文本、完整响应、request/header、endpoint、本地路径、设备、原始错误、PCM/WAV/Base64 或环境变量值。实现不向 diagnostic sink 写入 Provider 自由文本。

固定失败码为：

```text
invalid_arguments
config_missing
audio_unavailable
audio_too_large
audio_invalid
timeout
unauthorized
rate_limited
provider_unavailable
protocol_failed
response_invalid
internal_failure
```

成功码固定为 `ok`。参数错误类别为 `invalid_arguments`，但参数错误使用固定 stderr 文本和退出码 `2`，不会生成可能误导为已执行的 stdout 报告；未知底层错误只映射为 `internal_failure`，不会进入报告。

合成示例（mock 结果，不是真实账户证据）：

```json
{"SchemaVersion":"1","Timestamp":"2026-08-01T12:00:00Z","Provider":"volc","Model":"volcengine-v3","Status":"pass","Code":"ok","Metrics":{"AudioDurationMS":1000,"AudioBytes":32044,"LatencyMS":125,"EventCount":2,"FinalReceived":true,"ProtocolTerminal":true}}
```

## 用于 #11 / #12 的证据边界

- mock/CI 只能证明命令编排、单 Provider 选择、超时清理、schema 和脱敏约束，不能替代真实账户、真实 endpoint、权限或协议终态验证。
- 获得明确授权后，可把真实 `volc` 脱敏报告作为 #11 的辅助证据，把真实 `mimo` 脱敏报告作为 #12 的辅助证据；同时记录命令、退出码、被测版本和账户侧政策核验时间。
- 分享前仍需人工检查报告；不得附 WAV、凭据、环境、识别文本、响应正文或额外网络日志。
- 本仓库 CI 不注入 Provider secret，也不执行这两个真实网络命令。
