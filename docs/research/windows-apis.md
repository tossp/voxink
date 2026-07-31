# Windows API 采用与文本输入边界

## 研究记录

- **问题/范围：** 说明阶段 1 热键/overlay 的 Win32 路线，以及 partial/final 文本输入不可绕过的安全边界。
- **日期：** 2026-07-26；阶段 1 路线复核：2026-08-01。
- **来源：** Microsoft Learn 官方页面，逐项链接见下表。
- **关键证据/定位：** TSF 章节的 composition/display attribute/architecture/registration 说明，Win32 API 的参数与安全说明，UI Automation 的安全与 ValuePattern 说明。
- **结论：** 阶段 1 采用 `RegisterHotKey` + `MOD_NOREPEAT` 与原生 Win32 popup + GDI、no-activate overlay，且不包含文本交付。以后若需控件内可修订 partial，正确语义仍是 **TSF composition**，普通 EXE 不能伪造。
- **适用边界：** 文档说明接口语义，不保证特定应用控件可编辑或接受自动化；实现前必须在目标 Windows 版本和应用矩阵验证。
- **会话 ID：** `ses_063c8de7fffex4qpgSsnrzZRfU`。
- **子代理类型：** explorer。

## 官方证据与结论

| 主题 | 官方链接 | 关键事实 | 对 VoxInk 的含义 |
| --- | --- | --- | --- |
| TSF composition | [Compositions](https://learn.microsoft.com/en-us/windows/win32/tsf/compositions) | composition 是文本服务与应用协调可编辑、可修订文本的机制 | partial 若要进入控件内并可替换，应走 composition，而非退格重写 |
| TSF 显示属性 | [Providing Display Attributes](https://learn.microsoft.com/en-us/windows/win32/tsf/providing-display-attributes) | text service 可为 composition 提供显示属性 | 候选 TSF 路线需设计候选/暂定文本的视觉语义；overlay 不需要此接口 |
| TSF 架构 | [TSF Architecture](https://learn.microsoft.com/en-us/windows/win32/tsf/architecture) | TSF 是 text service、manager、context 等协作架构 | 不是一个普通窗口可随手调用的“把 partial 写进任意控件” API |
| TSF 注册 | [Text Service Registration](https://learn.microsoft.com/en-us/windows/win32/tsf/text-service-registration) | text service 的安装、注册、profile/激活是平台集成的一部分 | 普通 EXE 不注册 text service 不能静默建立跨应用 composition；若研究 TSF，部署/兼容性/安全是独立工作流 |
| `SendInput` | [SendInput](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendinput) | 合成输入受 UIPI 约束，只能注入相同或更低 integrity level；失败原因并不总能从返回值判明 | 仅用于已验证焦点和权限的 final；失败必须降级 `Copy Only` |
| Unicode 键入 | [KEYBDINPUT](https://learn.microsoft.com/en-us/windows/win32/api/winuser/ns-winuser-keybdinput) | `KEYEVENTF_UNICODE` 以 `VK_PACKET` 传 Unicode，需用 key down/up 配对 | 可作为 final 的一种编码路径；仍不是 composition，也不绕过 UIPI |
| `WM_CHAR` / UTF-16 | [WM_CHAR](https://learn.microsoft.com/en-us/windows/win32/inputdev/wm-char) | UTF-16 代理项以两个 `WM_CHAR` 消息传递 | 不能按 Unicode code point 粗暴拆分/删除文本；文本/选择处理要以 UTF-16 边界测试 |
| UI Automation 安全 | [UI Automation Security Overview](https://learn.microsoft.com/en-us/windows/win32/winauto/uiauto-securityoverview) | UIA 同样受安全和桌面边界限制 | UIA 不是绕过 UAC、隔离桌面或受保护控件的后门 |
| `ValuePattern.SetValue` | [IUIAutomationValuePattern::SetValue](https://learn.microsoft.com/en-us/windows/win32/winauto/uiauto-implementingvalue) | 只适用于实现 ValuePattern 且可写的控件，可能失败 | 若未来作为 final 输出候选，应先探测 pattern/可写性并保留 `Copy Only` |
| IMM32 边界 | [About Input Method Manager](https://learn.microsoft.com/en-us/windows/win32/intl/about-input-method-manager)、[WM_IME_COMPOSITION](https://learn.microsoft.com/en-us/windows/win32/inputdev/wm-ime-composition) | IMM32 消息面向 IME 交互；`WM_IME_COMPOSITION` 是 IME 组合相关消息 | 不把向目标窗口伪造 IME 消息当作普通 EXE 的通用 partial 方案；跨框架可靠性和安全性不成立 |

## 阶段 1 Win32 采用

- 全局热键直接调用 [`RegisterHotKey`](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-registerhotkey)，启用 `MOD_NOREPEAT`；`WM_HOTKEY` 只投递开始/停止请求，耗时工作交给 session owner。
- overlay 使用原生 popup + GDI；`WS_EX_NOACTIVATE` 避免激活，`WS_EX_TOOLWINDOW` 避免进入任务栏/Alt+Tab，置顶时使用 no-activate 行为。
- UI 消息循环与绘制保持在锁定 OS thread；后台状态通过消息投递回 UI 线程。该路线只服务阶段 1 的状态/文本展示，不扩展到 tray、文本注入或 TSF。
- Win32 绑定采用 `golang.org/x/sys v0.47.0`；不采用 hotkey wrapper、Fyne 或 Wails/WebView。以上均待 Windows 实机验证。

## MVP 裁决与安全边界

1. `Partial` 只显示在 VoxInk overlay；`Final` 才进入 `SendInput`、`Clipboard Paste` 或 `Copy Only` 策略。
2. 每次 final 交付前重新确认前台焦点和可注入性。管理员/UAC/安全桌面、密码框、远程桌面、受保护窗口或能力不明的控件，均不尝试规避限制，走 `Copy Only`。
3. UIA `ValuePattern` 可以是以后评估的目标控件能力，不可把它表述为通用注入方案。
4. 未来若选择 TSF，须单列 Text Service 注册、激活、显示属性、卸载/升级、不同编辑器兼容性及安全审查；该研究不承诺实现。

## 事实、工程判断与未确认项

- **事实：** composition、UIPI 与 UTF-16 处理边界来自上述 Microsoft 页面。
- **工程判断：** overlay + final 是在未实现 TSF 前最小且不会伪造 IME 语义的 MVP 路线。
- **未确认项：** 各编辑器、WebView、终端、RDP 与企业防护产品对 `SendInput`/剪贴板/UIA 的实际接受程度；须通过实现期测试矩阵确认。
- **外部记忆：** Windows 路线主记录 `#352`，核验日期 2026-08-01。
