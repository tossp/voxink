# 路线图

## 阶段 0：脚手架（当前）

- 独立 Go module、最小可编译入口与无依赖领域类型；
- 产品、架构、Provider 和 Windows 边界文档；
- 不含网络、录音、GUI、数据库或 Windows 注入实现。

## 阶段 1：输入与 ASR PoC

- 采用 `github.com/gen2brain/malgo v0.11.25` 显式 WASAPI，固定 callback 输出 16k/mono/PCM16；callback 只复制 PCM 到有界 ingress 并计算 level；
- 采用 `golang.org/x/sys v0.47.0` 直接实现 `RegisterHotKey` + `MOD_NOREPEAT` 和原生 Win32 popup + GDI、no-activate overlay；不采用 go-wca、hotkey wrapper、Fyne、Wails/WebView；
- 按 `AsrVendor` 目标实现火山 V3 `bigmodel_async` 主供应商和 MiMo `mimo-v2.5-asr` batch 备用供应商，默认不并行双发；主失败后以同一段内存 PCM 转 WAV 后非流式 POST，主备均失败时显式报错；
- 实现非流式/文件路径的停顿渐进切段和单一 FIFO：500ms 最短自动段、600ms 连续静音、15 秒无停顿上限；停止或 60 秒上限冲刷非空短尾，队列排空后才产生完整 `Final`；
- 分离 live recognizer 与 completed-segment transcriber；火山不能伪装成 batch `SegmentTranscriber`，且 `definite` 仅作为稳定分句，停止捕获、发送末包并收到协议终态后才产生唯一完整 `Final`；
- 由 session owner 串行管理 controller，建立 `SessionID` 门控、partial/final 悬浮层、最小错误处理和脱敏诊断；火山认证 header/resource ID 必须由真实账户控制台确认，凭据不写入仓库或日志；
- Windows 实机按 `audio only → hotkey+overlay → 60s/session → session events → real ASR` 推进，每一步通过后再叠加下一层；
- 本阶段不包含实际 Windows 文本输出、历史/SQLite、完整托盘设置、TSF 或正式发布。

## 阶段 2：输出与历史

- 实现可配置的 `SendInput`、`Clipboard Paste`、`Copy Only`，并完成安全降级；
- 实现 SQLite 文本历史与最小设置；默认不保存原始音频；
- 覆盖焦点变化、权限差异和注入失败的 Windows 测试场景。

## 阶段 3：扩展供应商与能力

- 在阶段 1 MiMo 文件路径基础上完善模型验收，并接入/评估 MOSI `moss-transcribe`；异步查询 endpoint 在真实账户核验前不写死；
- 保持 Batch 与 Streaming API/调度边界，区分结果流与实时音频输入；
- 复核官方请求格式、限制、错误模型和数据处理条款。

## 阶段 4：智能理解

- 研究将 `mimo-v2.5` Audio Understanding 用于转写后的独立理解工作流；它不作为默认 ASR，也不预设与专用 `mimo-v2.5-asr` 等价；
- 明确用户触发、数据范围和输出位置；不破坏 ASR 实时路径。

## 可选：TSF 研究

- 仅在 MVP 输出可靠后，评估注册/激活 Text Service 与 TSF composition 的成本；
- 以可修订的输入框内 partial 为研究目标，不承诺进入产品范围。

## 开放问题

1. 阶段 2 以后 UI 技术栈、托盘实现与打包分发方式；
2. 快捷键冲突、剪贴板恢复及不同权限级别的完整测试矩阵；
3. SQLite schema、历史保留和导出/删除机制；
4. 供应商配置 UX、离线行为、速率限制与用户告知；
5. 许可证选择与外部依赖审批；
6. stable prefix 的 Provider 契约与是否开放增量输入。

malgo（Unlicense）、其内嵌 miniaudio（Public Domain/MIT-0）与 x/sys（BSD-3-Clause）仅记录依赖许可事实；VoxInk 自身许可证仍未选择，未来分发前须复核通知义务，不添加 `LICENSE`。上述路线保持 v0.4 的参数、主备和阶段范围不变，当前均未实现；Provider 与 Windows 外部研究记忆分别为 `#351`、`#352`，核验日期 2026-08-01。
