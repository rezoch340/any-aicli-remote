# 交接说明

## 最终状态

- Any AI CLI Remote 的源码开源发布准备已完成；MIT 已选定，`LICENSE` 与 `README.md` 已更新，本交接不再改动它们。
- canonical GitHub repository 为 `rezoch340/any-aicli-remote`，本地目录名为 `any-aicli-remote`，remote 已指向新仓库名。
- 仓库可见性公开与 push 将在最终 commit 之后进行；当前不声称它们已经完成。
- 后续工作属于发布后维护，不再有未开始的 release checklist item。

## 验证证据

- `./scripts/build-all.sh` 完成且 exit 0：Go quality/race/vet、macOS Launcher 36 项测试、Android 所有已配置 unit/assemble/lint/detekt modules、iOS generic Simulator build 与 concrete Simulator tests 全部通过。
- iOS ordinary suite 中 Simulator keychain/live tests 按预期跳过；这不是对真实链路结果的替代。
- focused real Grok iOS smoke 通过 2/2：pairing/session list，以及 structured ask interaction。
- broader 5-case live run 仅有 2 个 product-path pass，不能表述为 5/5。其余 3 项为自动化或 Provider 非确定性失败：child Agent 未触发；plan/stream cases 遇到 Simulator paste-menu/key-entry automation failures。
- privacy scan 未发现 tracked local username、home path、IP 或 credential pattern。legacy branding 仅保留在有文档说明的 compatibility/migration code 与 tests 中。

## 二进制签名限制

- 当前只有 Apple Development identity；没有 Developer ID identity，也没有名为 `AnyAICLIRemote` 的 notarytool keychain profile。
- ad-hoc app 与 embedded daemon 均通过 `codesign --verify --deep --strict`，但 Gatekeeper 拒绝该构建。
- 因此可以声明源码开源准备完成，但不能声明已有可分发的 notarized macOS binary。获得所需发行身份与公证凭据后，二进制发行仍需另行完成签名、公证和 Gatekeeper 验证。
