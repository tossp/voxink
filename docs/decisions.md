# 决策记录

## 已定

| 决策 | 结论 | 理由 |
| --- | --- | --- |
| 产品边界 | Windows 跨应用智能语音输入前端 | 聚焦快捷键收音到文本交付的单一工作流 |
| 阶段 1 主供应商 | 火山 V3 true streaming，目标 endpoint variant 为 `bigmodel_async` | 它是 WebSocket 实时音频 ingress；认证 header 与 resource ID 必须按真实账户控制台确认 |
| 阶段 1 备用供应商 | MiMo `mimo-v2.5-asr` batch | 完整段 PCM 保留在内存，转 WAV 后非流式 POST；只在主供应商失败后串行调用 |
| MOSI | `moss-transcribe` 延后到阶段 3 | 属于完整音频 Batch ASR，且官方资料对异步查询 endpoint 存在冲突，账户核验前不写死 |
| MiMo `mimo-v2.5` | 保持 Audio Understanding，不作为默认 ASR | 官方分类和响应语义不同于专用 `mimo-v2.5-asr`，不宣称二者等价 |
| partial 交付 | MVP 使用悬浮层，final 才输出 | 普通程序不能无注册地取得 IME composition 语义 |
| 输出降级 | 不安全时强制 `Copy Only` | 不误投递优先于自动化便利 |
| 会话音频与留存 | 单次最长 60 秒；原始 PCM 仅存会话内存，完成后释放，默认不落盘 | 支持同音频备用重试并最小化敏感数据留存 |
| 凭据 | 后续使用 DPAPI 或 Credential Manager | 避免文件、日志和仓库存放密钥 |
| 设计参考 | 参考 BiBi Keyboard 模式，独立重写 | 借鉴状态/事件/能力注册，不迁移 Android 实现 |
| 供应商中心 | 未来以 `AsrVendor` 作为配置、能力和引擎选择核心；模型属于供应商配置 | 对齐 BiBi 的 `AsrVendorRegistry` / `AsrEngineModeResolver` 结构，同时保持当前 Provider/Session 领域类型稳定 |
| MiMo 能力分离 | `mimo-v2.5-asr` 是阶段 1 batch 备用；`mimo-v2.5` 只登记 Audio Understanding | 可共享 endpoint/凭据，但不共享请求体、能力语义或 ASR 验收结论 |
| ASR 主备 | 只配置不同的一个主供应商和一个备用供应商；默认不并行 | 主失败后以同一段内存 PCM 调备用；主备均失败必须显式报错 |
| 非流式切段 | 500ms 最短自动段、600ms 连续静音、15 秒无停顿上限；停止/60 秒上限冲刷非空短尾 | 停顿中点切段且录音继续；500ms 不得误用于丢弃停止时短尾 |
| 段处理 | 单一 FIFO 顺序识别与拼接，队列排空后才产生完整 `Final` | 防止并行乱序；中文直拼，ASCII 字母/数字边界按需补空格 |
| 能力边界 | Streaming、File/Batch、Audio Understanding 分开路由 | 由供应商能力和模型配置选执行路径，不伪造统一协议 |
| 内部网络契约 | live recognizer 与 completed-segment transcriber 分离 | 火山实时协议不能伪装成现有 batch `SegmentTranscriber`；主备共享会话/事件边界，不共享网络生命周期 |
| 火山终态映射 | `definite` 只表示稳定分句，不是 VoxInk Session `Final` | 停止捕获、发送末包并收到协议终态后，才产生唯一完整 `Final` |
| WebSocket 客户端 | 采用 `github.com/coder/websocket v1.8.15`，保持 `CompressionDisabled` 并显式配置正数 read limit | context-first API、可注入 `HTTPClient`/`HTTPHeader`，适合一条协议包对应一次 binary message 的单读单写连接；采用证据见记忆 `#354`（2026-08-01） |
| Windows 阶段 1 路线 | `github.com/gen2brain/malgo v0.11.25` 显式 WASAPI；`RegisterHotKey` + `MOD_NOREPEAT`；原生 Win32 popup + GDI、no-activate overlay | 固定 callback 输出 16k/mono/PCM16；以最小原生 UI/热键面完成实机 PoC |
| Windows 绑定 | 采用 `golang.org/x/sys v0.47.0`；不采用 go-wca、hotkey wrapper、Fyne、Wails/WebView | 直接绑定所需 Win32 API，避免阶段 1 引入额外 COM、GUI 或 WebView 生命周期 |
| 并发责任 | audio callback 只复制借用 PCM 到有界 ingress 并计算 level；session owner 串行管理 controller | callback 不分段、不联网、不更新 UI，避免实时线程承担阻塞工作 |
| 实机 spike 顺序 | audio only → hotkey+overlay → 60s/session → session events → real ASR | 逐层隔离设备、消息循环、会话和协议风险 |
| 依赖许可事实 | coder/websocket 为 ISC；malgo 为 Unlicense；其 miniaudio 为 Public Domain/MIT-0；x/sys 为 BSD-3-Clause | VoxInk 自身许可证仍未选择；未来分发前需复核并履行依赖通知义务 |

## 尚未决定

- VoxInk 自身许可证：尚未获选择授权，当前不添加 `LICENSE`；
- 阶段 2 以后 UI/托盘技术栈、打包方式与最低系统版本；阶段 1 原生 overlay 路线不外推为完整 GUI 决策；
- SQLite 数据模型、历史保留期、导入导出与删除语义；
- 凭据配置 UX、速率限制和脱机行为；
- 如何判定输出目标安全、应用白名单与剪贴板恢复策略；
- 是否以及何时开展 TSF Text Service 研究。

这些事项是待决项，不应被实现或文档表述为既定产品承诺。

这些细化保持设计草案 v0.4 的 60 秒、切段参数、单主单备和阶段范围不变，且均为未实现的采用目标。BiBi 架构参考仍固定于 commit `741fbb15df7041d11122e43ef5053ff8ff6642b8`，见[设计参考](bibi-keyboard-reference.md)和[研究证据](research/bibi-keyboard.md)。最新官方 Provider 协议与 Windows 路线分别引用外部研究记忆 `#351`、`#352`，核验日期为 2026-08-01；项目内最小证据见[Provider 文档](providers.md)、[Windows API 研究](research/windows-apis.md)和[Go 生态研究](research/go-ecosystem.md)。
