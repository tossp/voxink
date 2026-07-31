# Go Windows 生态采用与架构判断

## 库事实取证

- **问题/范围：** 记录阶段 1 Go 依赖采用、替代方案否决及 `jchv/go-tsf` 的 `eternal WIP` 边界。
- **日期：** 2026-07-26；依赖版本与路线复核：2026-08-01。
- **来源：** 下表 GitHub 仓库 README/仓库说明快照；未记录 release tag/commit 的项目只可作为探索入口。
- **关键证据/定位：** 链接到各仓库根 README。
- **结论：** Provider Adapter 已采用 coder/websocket `v1.8.15`；未来 Windows 阶段 1 实现采用 malgo `v0.11.25`（显式 WASAPI）和 x/sys `v0.47.0`。go-wca、hotkey wrapper、Fyne、Wails/WebView 不采用；`jchv/go-tsf` 不作为生产 TSF 基础。
- **适用边界：** 不构成依赖批准、兼容性承诺或工期估算；实现前仍须复核 API、许可证、Windows 版本和维护状态。
- **会话 ID：** `ses_063c8de7fffex4qpgSsnrzZRfU`。
- **子代理类型：** explorer。

## 已采用依赖

| 依赖 | 固定版本 | 用途 | 许可证事实 | 采用边界 |
| --- | --- | --- | --- | --- |
| [coder/websocket](https://github.com/coder/websocket/tree/v1.8.15) | `v1.8.15` | 火山 V3 binary WebSocket 客户端 | ISC；模块本身无第三方依赖 | `HTTPHeader` 传握手认证、`CompressionDisabled`、单发送路径/单读取路径、显式正数 read limit；context 取消后连接不复用 |
| [gen2brain/malgo](https://github.com/gen2brain/malgo/tree/v0.11.25) | `v0.11.25` | 音频采集，显式 WASAPI | Unlicense；内嵌 miniaudio 为 Public Domain/MIT-0 | 请求并在启动后校验 16k/mono/PCM16，不满足则失败；callback 只复制到有界 ingress 并计算 level |
| [golang.org/x/sys](https://github.com/golang/sys/tree/v0.47.0) | `v0.47.0` | 直接绑定 Win32 热键、窗口与消息循环 API | BSD-3-Clause | 只绑定阶段 1 所需 API，不引入 GUI/hotkey wrapper |

coder/websocket 已写入 `go.mod`；malgo 与 x/sys 仍是未来 Windows 实现采用，尚未加入。VoxInk 许可证仍未选择；未来分发前需复核依赖通知义务，但本轮不添加 `LICENSE`。WebSocket 选型、版本与许可核验证据见外部研究记忆 `#354`，核验日期 2026-08-01。

## 已否决或延后方案

| 仓库 | 用途 | 证据定位/快照 | 成熟度判断 | 风险与采用前复核 |
| --- | --- | --- | --- | --- |
| [golang-design/hotkey](https://github.com/golang-design/hotkey) | 跨平台全局快捷键 | 根 README 的平台支持/API 示例；未记录固定 tag/commit | 阶段 1 不采用 | 单一热键直接使用 `RegisterHotKey` + `MOD_NOREPEAT` 更小 |
| [getlantern/systray](https://github.com/getlantern/systray) | 系统托盘菜单/图标 | 根 README 的 cross-platform tray 说明；未记录固定 tag/commit | 延后 | 阶段 1 明确不含完整托盘 |
| [moutend/go-wca](https://github.com/moutend/go-wca) | Windows Core Audio（WCA）接口封装 | 根 README 的 Windows Core Audio 定位；未记录固定 tag/commit | 阶段 1 不采用 | malgo 提供更短的显式 WASAPI 与格式转换/校验路径 |
| [zzl/go-win32api](https://github.com/zzl/go-win32api) | Win32 API Go 绑定 | 根 README 的 Win32 API 绑定/生成项目定位；未记录固定 tag/commit | 阶段 1 不采用 | x/sys 加最小 direct binding 已满足范围 |
| [jchv/go-tsf](https://github.com/jchv/go-tsf) | TSF 相关实验性绑定 | 根 README 将项目标为 `eternal WIP`；未记录固定 tag/commit | 不成熟 | **不作为生产 TSF 基础**；即使试验也需自行承担注册、COM、兼容性和长期维护 |

## 架构判断（非源材料事实）

该节来自 oracle 工程研判；其任务未直接读取源材料，以下是受假设约束的判断，不能转述为库或供应商事实。

- **会话 ID：** `ses_063ca9428ffeExHxVeDp8CCiAD`。
- **子代理类型：** oracle。

- **假设：** MVP 是 Windows 单机前端；需要一个实时 WebSocket Provider、可替换的完整音频 Provider、overlay 和安全 final 输出；当前不引入依赖。
- **判断：** 先稳定 `SessionID`、事件、`RecognitionMode` 和 provider capability，使 Capture、Provider、Overlay、Output 通过小接口连接；把 Windows 平台代码关在未来 adapter 内。这样可在不实现 TSF 的前提下验证实时路径。
- **判断：** 将 true streaming 与 complete-audio batch 作为不同调度生命周期；result streaming 不能使 batch 适配器成为 live-audio ingress。
- **判断：** audio callback 只复制借用 PCM 到有界 ingress 并计算 level；session owner 串行管理 controller，避免在实时 callback 内分段、联网或更新 UI。
- **边界：** 未给出也不应从中推导工期、并发指标、性能数字或外部库成熟度；实际选库前需作独立 spike。

## 未确认项

- malgo 的 cgo 工具链、设备格式转换、Windows ARM64 与设备切换必须在实机验证。
- 未来 UI、录音、托盘和全局快捷键是否可共享同一消息循环。
- 这些 Windows 不确定项不改变本轮只实现 Provider Adapter、未实现录音与 Windows 功能的范围。
- 外部 Windows 路线主记录为记忆 `#352`，核验日期 2026-08-01。
