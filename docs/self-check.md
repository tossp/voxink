# VoxInk 分层自检

`voxink self-check` 为 [#9](https://github.com/tossp/voxink/issues/9) 的 Windows 真机逐层验证提供可复核的前置报告。它不加载阶段 1 Provider 配置、不读取 Provider 凭据、不发起 ASR 网络请求，也不执行文本输出。

## 命令与退出码

```text
voxink self-check --mode=static --json
voxink self-check --mode=audio --duration=3s --json
voxink self-check --mode=interactive --timeout=15s --json
```

- 默认 `--mode=static`；`--duration` 和 `--timeout` 必须大于 0 且不超过 60 秒。
- `--json` 将唯一报告写入 stdout；用户提示和参数错误写入 stderr。省略 `--json` 时输出简短的人读文本。
- 任一 check 为 `fail` 时退出码为 `1`；参数错误为 `2`；只有 `pass`、`manual` 或 `skipped` 时为 `0`。

## 模式

### static

无需 Windows 设备或 Provider，检查：

- executable build/runtime、`GOOS`、`GOARCH`、Go version 与编译时 CGO 可用性；
- 16 kHz、mono、PCM16 固定音频契约；
- 500 ms 最短语音、600 ms 静音、15 秒最大连续段、60 秒最大会话参数；
- `Idle`、`Capturing`、`Transcribing`、`Delivering`、`Stopped`、`Failed` 六态及核心 controller 正常/失败路径。

Windows 且禁用 CGO 时，`windows_platform` 明确返回 `fail/cgo_disabled`。非 Windows 构建返回 `skipped/platform_unsupported`，但仍输出完整 JSON，便于开发者检查通用层。

### audio

仅在 Windows + CGO 上进行受控 WASAPI 短采集：初始化 capture、验证回调格式、启动，在指定时长内统计收到的字节、callback frame、电平事件与 overflow 次数，最后停止并关闭。探针只计数后立即丢弃 callback PCM 引用，不保存、不打印、不持久化 PCM。

无输入帧时返回 `audio_timeout`；无设备、权限、初始化、启动、运行或清理错误只映射为固定 code，不输出原始错误。非 Windows 或 Windows no-CGO 构建固定失败并分别使用 `platform_unsupported` 或 `cgo_disabled`。

### interactive

仅依赖 Windows overlay 与 `RegisterHotKey`，不启动 capture 或 Provider。命令显示固定 self-check notice，等待用户按一次 `Ctrl+Shift+Space`，然后安全关闭 overlay。

- 收到 toggle 只能证明本次热键事件路径通过。
- no-activate/焦点保持必须由用户观察，报告固定为 `manual/focus_confirmation_required`。
- 超时返回 `fail/toggle_timeout`；热键冲突或 overlay 初始化失败只返回固定安全 code。

## JSON 报告

schema version `1` 顶层字段为：

| 字段 | 含义 |
| --- | --- |
| `SchemaVersion` | 固定 schema 版本 |
| `Timestamp` | UTC RFC 3339 生成时间 |
| `Mode` | `static`、`audio` 或 `interactive` |
| `App.Version` | Go build info 版本；本地开发构建为 `dev` |
| `Runtime` | `GOOS`、`GOARCH`、`GoVersion`、`CGOAvailable` |
| `Checks` | `{Name, Status, Code, Metrics}` 数组 |

`Status` 仅允许 `pass`、`fail`、`manual`、`skipped`。`Code` 来自实现内固定 allowlist。`Metrics` 是关闭字段集合，只含有界数字、布尔值或固定枚举；没有 map、自由文本 payload 或动态错误字段。

合成 static 示例（不是 Windows 真机结果）：

```json
{"SchemaVersion":"1","Timestamp":"2026-08-01T12:00:00Z","Mode":"static","App":{"Version":"dev"},"Runtime":{"GOOS":"windows","GOARCH":"amd64","GoVersion":"go1.26.3","CGOAvailable":true},"Checks":[{"Name":"build_runtime","Status":"pass","Code":"ok","Metrics":{}},{"Name":"audio_contract","Status":"pass","Code":"ok","Metrics":{"SampleRate":16000,"Channels":1,"BytesPerSample":2,"Format":"pcm_s16le"}},{"Name":"session_policy","Status":"pass","Code":"ok","Metrics":{"MinimumSpeechMS":500,"SilenceSplitMS":600,"MaximumSegmentMS":15000,"MaximumSessionMS":60000}},{"Name":"session_controller","Status":"pass","Code":"ok","Metrics":{"StateCount":6}},{"Name":"windows_platform","Status":"pass","Code":"ok","Metrics":{}}]}
```

## 隐私与安全边界

报告禁止包含设备名、设备 endpoint、窗口标题、PCM/WAV/Base64、识别文本、路径、环境变量名或值、凭据、请求内容及 `error.Error()` 原文。指标最大值由实现钳制；报告只存在于命令输出，不新增持久日志或遥测。

分享报告前仍应按组织政策检查附件。实现的字段 allowlist 降低意外泄露风险，但不替代接收方的访问控制。

## 用于 #9

1. 先附 `static` JSON，记录被测 executable 的版本与平台构建边界。
2. 在同一 Windows 真机运行 `audio`，附 JSON 作为 capture 初始化、格式、短采集和清理的证据。
3. 单独运行 `interactive`，按一次热键，并在 #9 检查表中人工记录 overlay 是否抢焦点；JSON 中的 manual 结果保持不变。
4. 将命令、退出码和 JSON 作为评论附件；不要附设备截图中的敏感内容、Provider 配置或额外日志。

## 不能验证的事项

- CI、交叉编译和 Linux static 结果不能证明真实麦克风、Windows 桌面会话、热键冲突矩阵或 no-activate 行为通过。
- `audio` 不评价录音质量、用户语音内容或 Provider 识别质量。
- `interactive` 不能自动证明焦点未变化；人工确认是 #9 的独立证据。
- 自检不覆盖 Provider smoke（#33）、真实 ASR、文本注入、Clipboard、tray、安装器、签名或持久日志。
