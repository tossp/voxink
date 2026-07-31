# Windows 文本输入边界

## 可选最终输出方式

| 方式 | 适合情况 | 主要边界 |
| --- | --- | --- |
| `SendInput` | 目标窗口与前台焦点可验证，且注入安全 | 受权限隔离、目标控件与焦点变化影响；不是 IME composition |
| `Clipboard Paste` | 注入文本不适用但粘贴可行 | 会临时使用剪贴板，粘贴仍取决于目标应用与焦点 |
| `Copy Only` | 焦点变化、目标不安全或任何注入失败 | 只将最终文本复制，由用户自行粘贴，优先保证不误投递 |

默认策略由设置配置。执行时必须重新检查前台上下文；一旦焦点变化、无法判断安全性或执行失败，即降级为 `Copy Only`，并在 UI 中说明结果。

TSF、`SendInput`、Unicode、UI Automation 与 IMM32 的官方证据定位见[Windows API 研究记录](research/windows-apis.md)；本页仅保留产品边界。

## partial 与 IME composition

普通托盘程序不能无注册地调用当前 IME 为任意输入框建立 composition。**TSF composition** 才是让输入框内 partial 可编辑、可修订的正确 Windows 语义，但它要求注册并激活 Text Service，涉及更高的部署、兼容性与安全复杂度。

因此 MVP 裁决为：

- partial 只在 VoxInk 悬浮层显示；
- 只对 final 执行输出策略；
- 若某 Provider 具备严格、可验证的 stable prefix，可作为后续可选“稳定增量输入”能力；
- 不以退格删除旧 partial 再重写为默认方案，因为它会破坏用户光标、选择区和目标应用文本。

## 安全限制

不尝试绕过 Windows 权限边界、UAC、受保护窗口、目标应用限制或企业防护策略。高权限窗口、远程桌面、密码框、焦点不确定以及无法安全注入的控件都应走 `Copy Only`。是否允许特定应用、如何处理剪贴板恢复和快捷键冲突，均是后续实现前需测试的开放问题。
