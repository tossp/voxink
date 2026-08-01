# BiBi Keyboard 设计参考

参考仓库：[BryceWG/BiBi-Keyboard](https://github.com/BryceWG/BiBi-Keyboard)，许可证为 [Apache-2.0](https://github.com/BryceWG/BiBi-Keyboard/blob/main/LICENSE)。VoxInk 仅吸收架构思路并独立重写，不复制大段源码，也不迁移 Android 专属实现。

> [!CAUTION]
> 许可与归属风险见[第三方许可技术审计](license-audit.md)。BiBi 的 `NOTICE` 包含 Ten VAD“Apache-2.0 with additional conditions”等非标准附加限制；BiBi 与 Ten VAD 当前都不是 VoxInk 的 included software。复制或改编任何 BiBi 源码、资源、模型或构建文件前，必须逐文件核验 provenance、许可证和 NOTICE，并完成必要的法律审查，不能只依赖仓库级 Apache-2.0 标识。

固定参考 commit 为 `741fbb15df7041d11122e43ef5053ff8ff6642b8`。它不是 release `v4.2.1`：`v4.2.1` tag 指向 `8d0fcebd3fe2abe4648d83666fe6329daedf7d68`，固定 commit 比该 tag 前进 7 个提交。关键文件定位与许可证/NOTICE 风险见[BiBi Keyboard 研究证据](research/bibi-keyboard.md)；本页仅保留设计定位。

## 可借鉴

- `AsrVendorRegistry` / `AsrEngineModeResolver`：以供应商为配置、能力和引擎选择核心，模型作为供应商配置，按 Streaming、File/Batch、Audio Understanding 能力路由；
- `MiMoFileAsrEngine`：MiMo 内可选 `mimo-v2.5-asr` 或 `mimo-v2.5`，共享 endpoint/凭据不代表共享请求体或语义；
- `AsrBackupPolicy` / `LazyLocalBackupAsrEngine`：只配置不同的主、备用供应商，主失败后以同一份内存音频调用备用，默认不并行双发；
- `NonStreamingProgressiveChunking`：采用 500ms 最短自动段、600ms 连续静音、15 秒无停顿上限和 60 秒会话上限；停止/硬上限时提交非空短尾；
- 单一 FIFO 顺序识别与拼接：停止录音不等于识别完成；VoxInk 在录音期间保持 `Capturing`，停止后由 `Transcribing` 等待队列排空并产生完整 `Final`；
- 清晰事件：`Partial`、`Final`、`Stopped`、`Error`、`Level`，其中 `Stopped` 对应代码中的 `EventStopped`；
- `AsrSessionManager` 的显式状态与会话序号：VoxInk 固定使用 `Idle`、`Capturing`、`Transcribing`、`Delivering`、`Stopped`、`Failed` 六态，并继续以 `SessionID` 拒绝迟到事件，防止旧结果污染新会话；
- 历史记录与输出降级：输出失败时保留可复制结果。

上述采用结论参考外部研究记忆 `#347`（供应商模型与主备）、`#348`（停顿切段）、`#349`（参数最佳实践），适用固定 commit 如上，核验日期为 2026-07-31。它们是后续实现目标，不表示 VoxInk 当前已有供应商中心、录音、识别或 UI 实现。

## 需要 Windows 化

- Android 的输入目标语义应重建为 Windows 前台窗口、焦点检查和 `SendInput` / 剪贴板策略；
- Android UI 需替换为 Windows 悬浮层、托盘与设置界面；
- 权限、密钥存储和历史目录应遵循 Windows 的平台机制，凭据计划使用 DPAPI 或 Credential Manager。

## 无法直接迁移

- `InputMethodService`、`InputConnection` 和 Android Accessibility 的实现与权限模型；
- Android 生命周期、IME composition 和应用内编辑模型；
- 任何 Android 专用的 UI、构建、依赖与系统集成代码。

Apache-2.0 参考不自动决定 VoxInk 的许可证。VoxInk 自身许可证尚未获授权选择，当前不添加 `LICENSE`；若未来复制、改编或分发任何外部代码，须单独履行其许可证、NOTICE、附加条件与逐文件 provenance 要求。
