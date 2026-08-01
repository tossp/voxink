# Windows 文本输入边界

## 已实现的最终输出方式

| 方式 | 适合情况 | 主要边界 |
| --- | --- | --- |
| `SendInput` | 目标窗口与前台焦点可验证，且注入安全 | 受权限隔离、目标控件与焦点变化影响；不是 IME composition |
| `Copy Only` | 焦点变化、无启动目标、密码框/安全性无法确认，或 `SendInput` 返回 0 | 将完整 final 留在剪贴板，绝不自动粘贴；UI 提示用户手动粘贴 |

当前实现会在会话开始时记录前台窗口，final 前重新核验同一目标，并只用 `KEYEVENTF_UNICODE` 发送 UTF-16 code units。partial 永不发送。`SendInput` 部分成功时不会再复制，以避免重复文本；注入与剪贴板都失败时会话失败。

TSF、`SendInput`、Unicode、UI Automation 与 IMM32 的官方证据定位见[Windows API 研究记录](research/windows-apis.md)；本页仅保留产品边界。

## partial 与 IME composition

普通托盘程序不能无注册地调用当前 IME 为任意输入框建立 composition。**TSF composition** 才是让输入框内 partial 可编辑、可修订的正确 Windows 语义，但它要求注册并激活 Text Service，涉及更高的部署、兼容性与安全复杂度。

因此 MVP 裁决为：

- partial 只在 VoxInk 悬浮层显示；
- 只对 final 执行输出策略；
- 若某 Provider 具备严格、可验证的 stable prefix，可作为后续可选“稳定增量输入”能力；
- 不以退格删除旧 partial 再重写为默认方案，因为它会破坏用户光标、选择区和目标应用文本。

## 安全限制

不尝试绕过 Windows 权限边界、UAC、受保护窗口、目标应用限制或企业防护策略。密码框、焦点不确定以及无法安全检查的控件走 `Copy Only`。当前不恢复旧剪贴板，因为 Copy Only 必须保留 final 供用户手动粘贴；未实现 Clipboard Paste、TSF 或 UI Automation。

当前 Win32 路径仅完成了 fake 驱动的自动化测试和 Windows 交叉编译，尚未在真实 Windows 桌面验证前台窗口核验、密码框识别、UIPI/高权限窗口、不同应用控件兼容性及剪贴板占用时的行为。
