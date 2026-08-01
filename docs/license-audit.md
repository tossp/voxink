# VoxInk 第三方许可技术审计

## 结论与边界

本审计对应 [Issue #34](https://github.com/tossp/voxink/issues/34)，证据基线为 VoxInk commit `23faf74487f5f2b0de2f12be416aa12058e33022`，核验日期为 2026-08-01。

> [!IMPORTANT]
> 本文是工程与分发层面的技术审计，不是法律意见，也不替代权利人授权或专业法律审查。第三方组件的许可证不会自动推导、授予或决定 VoxInk 自身的许可证。VoxInk 仍未选择许可证，仓库不得据此新增 `LICENSE` 或宣称采用 MIT、Apache-2.0 等许可证。

本审计是 [#24](https://github.com/tossp/voxink/issues/24) 作出项目许可证决定前的证据，但不替代该决定；#24 继续 blocked。许可证、上游内容、依赖版本、CI action pin 和发布资产都具有时效性，每次分发前必须重新核验。

`v0.1.0-alpha.1` 已于 2026-08-01 作为 GitHub prerelease 发布，并包含 Windows exe 与 SHA256 asset。第三方归属材料因此是现有分发需要立即补齐的仓库材料，而不是“未来首次发布”事项。本变更合并后，发布主流程仍需为该现有 release 上传独立的 `THIRD_PARTY_NOTICES.md` asset、补充 release 说明，并回链 #34 与 #24；本审计不执行远端修改。

## 直接 Go 依赖与随附组件

基线 `go.mod` 直接声明以下三个模块；版本 tag 均核验到表中固定 commit。

| 组件 | 版本 / 固定 commit | 用途与分发边界 | 许可证与归属义务 | 风险结论 |
| --- | --- | --- | --- | --- |
| `github.com/coder/websocket` | `v1.8.15` / `9c8faadccd1b679e811a79ce506f8a10237251ad` | 编译进网络客户端 | ISC；所有副本保留版权与许可文本；无单独 NOTICE/PATENTS 文件 | 在分发材料中提供完整 ISC 文本；已收录于根目录 notice |
| `github.com/gen2brain/malgo` | `v0.11.25` / `dd586bde45ae1d27f7406f75aa05dfa8b02ec9ca` | Windows 音频采集 cgo 绑定 | malgo 为 Unlicense；无单独 NOTICE/PATENTS 文件 | 登记来源与完整上游文本；同时审计其内嵌 miniaudio |
| bundled miniaudio | `v0.11.25`，位于上述 malgo commit 的 `miniaudio.h` / `miniaudio.c` | 随 malgo 源码进入 cgo 构建，不是独立 Go module | 上游提供 `Public Domain OR MIT-0` 选择；无单独 NOTICE 文件 | 不能只登记 malgo 而遗漏 bundled C 组件；根 notice 保留固定快照的双许可原文 |
| `golang.org/x/sys` | `v0.47.0` / `9e7e939dcafac07e8ab4cffa6e5fc74908413f00` | 编译进 Windows 平台绑定 | BSD-3-Clause：源码保留版权、条件和免责声明；二进制分发在文档和/或其他材料中重现这些文本；不得用 Google LLC 或贡献者名称背书；另有 PATENTS | 根 notice 收录完整 BSD-3-Clause 与 PATENTS，随二进制 artifact 分发 |

当前根目录 [`THIRD_PARTY_NOTICES.md`](../THIRD_PARTY_NOTICES.md) 是可随 artifact/release 交付的归属材料。它只覆盖第三方组件，不是 VoxInk 自身的许可证。

## GitHub Actions：仅 `uses:` 的构建工具边界

| Action | workflow pin | 许可证 | 边界与处理 |
| --- | --- | --- | --- |
| `actions/checkout` | `3d3c42e5aac5ba805825da76410c181273ba90b1` (`v7.0.1`) | MIT | workflow 通过 `uses:` 调用；未把 action 源码复制、vendor 或打入 VoxInk artifact |
| `actions/setup-go` | `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` (`v7.0.0`) | MIT | 同上；仅提供 CI Go 工具链安装/缓存步骤 |
| `actions/upload-artifact` | `043fb46d1a93c77aae656e7c1c64a875d1fc6a0a` (`v7.0.1`) | MIT | 同上；仅上传构建输出与第三方 notice |

本审计把这三个 action 记录为 CI 构建工具，而不是 VoxInk 二进制中的 included software，因此不把它们并入 `THIRD_PARTY_NOTICES.md`。如果未来复制、修改、vendor action 代码，或把其文件放入发布包，必须重新评估并保留对应 MIT 文本。不得借本次审计改变 action pins、权限或测试矩阵。

## BiBi Keyboard：仅设计参考

VoxInk 仅把 [BiBi Keyboard 固定 commit `741fbb15df7041d11122e43ef5053ff8ff6642b8`](https://github.com/BryceWG/BiBi-Keyboard/tree/741fbb15df7041d11122e43ef5053ff8ff6642b8) 用作设计参考，当前仓库没有把 BiBi 或其第三方组件作为 included software，也不应将其列入 VoxInk 的第三方分发清单。

BiBi 仓库级许可证为 Apache-2.0，但其 `NOTICE` 还登记了多项第三方材料。尤其是 Ten VAD 模型及许可文件标为“Apache License 2.0 with additional conditions”，包含竞争用途限制、仅为自身 Application 与直接 End Users 部署、衍生作品继续受附加条件约束等非标准限制。工程上必须把它视为**高风险、需要专项法律审查的受限材料**，不能仅凭 BiBi 根许可证或“open source”字样推定可复制、改编或分发。

在复制或改编 BiBi 的任何源码、资源、模型、提示文本或构建文件前，必须逐文件完成 provenance（原始上游、固定版本、作者、许可证、NOTICE、修改历史）核验，并取得必要的法律审查/授权。当前只记录抽象设计参考，不夸大为代码采用；Ten VAD 模型与代码未进入 VoxInk。

## VoxInk 许可证选项的技术比较

下表只比较工程和分发影响，不作选择，也不构成兼容性或法律结论。

| 方案 | 技术/分发影响 | 与第三方材料的关系 | 未决点 |
| --- | --- | --- | --- |
| MIT | 项目级文本短；分发项目副本时通常需保留 VoxInk 自身版权与许可文本；不含 Apache-2.0 式明确专利授权条款 | 仍须分别携带 ISC、BSD-3-Clause、PATENTS、Unlicense、miniaudio 双许可等第三方文本；不能用一个项目 MIT 文本替代 | 需要所有者明确授权；需决定版权标识、年份、贡献与专利风险处理 |
| Apache-2.0 | 含明确专利授权/终止、修改文件标记、NOTICE 传播机制和商标边界，分发流程更长 | 仍须保留每个第三方许可证；若项目建立 NOTICE，不能删除适用的第三方归属，也不能把第三方附加限制误写成 Apache-2.0 | 需要所有者明确授权；需建立贡献、NOTICE 与修改标记流程，并审查专利/兼容性 |
| 暂不授权（不选择项目许可证） | 默认版权状态下不向公众提供通用复制、修改或再分发授权；协作、复用和二进制分发权限边界更不清晰，但不是第三方义务豁免 | 已分发或未来分发的二进制仍必须满足所有第三方条款并携带 notice；上游许可不授予 VoxInk 自身代码 | 现有 prerelease 的项目授权基础、贡献者权利和后续公开分发需由所有者/法律流程确认；#24 仍 blocked |

无论最终选择何种方案，都不能以第三方均为宽松许可证为由自动宣布 VoxInk 的许可证，也不能用 VoxInk 的未来许可证覆盖 Ten VAD 等非标准附加条件。

## 每次发布与现有 prerelease 补齐清单

- [ ] 从目标 tag/commit 重新核对 `go.mod`、`go.sum`、`go list -m all` 与实际编译平台依赖，记录版本和固定 commit。
- [ ] 检查 cgo、vendored、generated、embedded、模型、字体、图标和其他资源，不能只看 Go module 图；特别确认 malgo 内嵌 miniaudio 的版本/许可未变化。
- [ ] 逐项核验上游 `LICENSE`、`NOTICE`、`PATENTS` 及附加条件；差异必须更新 `THIRD_PARTY_NOTICES.md`。
- [ ] 检查提交差异是否复制/改编外部文件；如有，记录逐文件 provenance、修改声明与法务结论。
- [ ] 对 Ten VAD 或任何非标准、用途限制、非竞争、模型/数据条款执行专项法律审查；未获批准不得纳入。
- [ ] 确认 CI action 仍只通过固定 SHA 的 `uses:` 调用；若 vendor/打包 action 内容则补充其许可证。
- [ ] 构建 artifact 同时携带 exe 与 `THIRD_PARTY_NOTICES.md`；正式/prerelease 页面另附独立 notice asset，便于脱离压缩包获取。
- [ ] 验证 notice 没有 VoxInk 自身授权措辞，仓库在 #24 决策前仍无 `LICENSE`，release notes 仍明确项目许可证未选择。
- [ ] 对 `v0.1.0-alpha.1`：本变更合并后上传独立 notice asset、补充说明，并回链 #34/#24；不改变既有 exe 或 SHA256。
- [ ] 记录复核日期、审计人、目标 commit、命令输出和未决项；许可证决定仍由 #24 单独闭合。

## 固定来源

### 直接依赖与 bundled 组件

- coder/websocket tag commit: <https://github.com/coder/websocket/commit/9c8faadccd1b679e811a79ce506f8a10237251ad>
- coder/websocket ISC: <https://github.com/coder/websocket/blob/9c8faadccd1b679e811a79ce506f8a10237251ad/LICENSE.txt>
- malgo tag commit: <https://github.com/gen2brain/malgo/commit/dd586bde45ae1d27f7406f75aa05dfa8b02ec9ca>
- malgo Unlicense: <https://github.com/gen2brain/malgo/blob/dd586bde45ae1d27f7406f75aa05dfa8b02ec9ca/LICENSE>
- bundled miniaudio `v0.11.25` 与双许可原文: <https://github.com/gen2brain/malgo/blob/dd586bde45ae1d27f7406f75aa05dfa8b02ec9ca/miniaudio.h>
- x/sys tag commit: <https://github.com/golang/sys/commit/9e7e939dcafac07e8ab4cffa6e5fc74908413f00>
- x/sys BSD-3-Clause: <https://github.com/golang/sys/blob/9e7e939dcafac07e8ab4cffa6e5fc74908413f00/LICENSE>
- x/sys PATENTS: <https://github.com/golang/sys/blob/9e7e939dcafac07e8ab4cffa6e5fc74908413f00/PATENTS>

### CI 与设计参考

- actions/checkout MIT: <https://github.com/actions/checkout/blob/3d3c42e5aac5ba805825da76410c181273ba90b1/LICENSE>
- actions/setup-go MIT: <https://github.com/actions/setup-go/blob/b7ad1dad31e06c5925ef5d2fc7ad053ef454303e/LICENSE>
- actions/upload-artifact MIT: <https://github.com/actions/upload-artifact/blob/043fb46d1a93c77aae656e7c1c64a875d1fc6a0a/LICENSE>
- BiBi 根 LICENSE: <https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/LICENSE>
- BiBi NOTICE: <https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/NOTICE>
- BiBi 随附 Ten VAD 许可: <https://github.com/BryceWG/BiBi-Keyboard/blob/741fbb15df7041d11122e43ef5053ff8ff6642b8/app/src/main/assets/licenses/TenVAD-LICENSE>
- 已发布 prerelease: <https://github.com/tossp/voxink/releases/tag/v0.1.0-alpha.1>
