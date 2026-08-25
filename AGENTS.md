# Any AI CLI Remote Engineering Rules

## Reuse is a quality gate

This is the first gate. It outranks shipping speed, local convenience, and default agent helpfulness. A change that invents a parallel helper, parser, protocol model, transport, or dependency without a recorded reuse search fails, even if tests pass. Do not start coding until the search below is done. "I will reuse later" is a failure.

- Search for an existing implementation before adding one. Reuse the canonical helper or extend it instead of copying its logic.
- Before writing non-trivial infrastructure, search the relevant official SDK, package registry, and maintained open-source projects. The required order is: reuse repository code, use the platform or language standard library, adopt a compatible maintained library, and only then write the smallest missing adapter.
- That search is mandatory, not optional review advice. Check current upstream documentation and releases online before coding; stale memory of a library or version is not sufficient evidence.
- A custom implementation is allowed only when the dependency decision records why the existing libraries are incompatible, unmaintained, incorrectly licensed, unsafe, or materially larger than the missing behavior. "Faster to write" is not a valid reason.
- Do not reimplement a published protocol with hand-written duplicate wire types when an official or established SDK covers them. Provider extensions must wrap or extend the shared SDK rather than fork its model.
- Prefer actively maintained libraries with tests, documented releases, and an open-source license compatible with this repository. Do not add a dependency for behavior already covered clearly by the standard library.
- Record each non-obvious dependency/reuse decision in `docs/DEPENDENCY_DECISIONS.md`, including the candidates checked, the chosen implementation, and the scope that must not be duplicated.
- A new custom parser, renderer, protocol model, process enumerator, persistence format, crypto helper, or transport fails review unless the decision log names the maintained libraries checked and gives a concrete incompatibility for each rejected candidate.
- There must be one shared implementation for each cross-provider concern, including registry behavior, session metadata, canonical path containment, timestamp normalization, pagination, connection generations, and compatibility migration.
- Adapters contain only provider-specific behavior. If code does not depend on a provider's protocol or on-disk format, it does not belong in the adapter.
- Do not create near-duplicate helpers with different names. Consolidate them and update callers.
- Prefer real module boundaries during architecture and refactoring: layer cohesive responsibilities such as model, transport, storage, domain, feature, and composition rather than merely splitting files while retaining one monolithic module.
- Put each shared capability in one owning module and expose the narrow public API required by its consumers. Do not scatter or duplicate helpers, protocol parsing, storage, networking, or UI orchestration across modules; consolidate scattered responsibilities before extending them.
- Avoid over-modularization as well: do not create a fragment module or wrapper without an independent responsibility or a real second caller.
- A facade whose methods only forward to another type is not a split. The 600-line gate is failed by that wrapper, not satisfied by it. Move the logic; do not add `Coordinator` / `Controller` / `Scope` / `Effects` types to keep a file under the limit.
- Transcript follow is list behavior: pin to the latest item while following, pause on user drag, resume on the jump-to-bottom control. One owner per client, same behavior on Android and iOS. Do not add a FollowController, EffectsState, transcript-hash `snapshotFlow`, spacer anchor item, or per-widget scroll calls.
- Backward-compatibility reads and migrations must be centralized. New writes use only the current Any AI CLI Remote names.

## Current product

This repository is past the bootstrap. It started as a Grok-branded remote and is now **Any AI CLI Remote**: a provider-neutral Go daemon, a macOS launcher, and native Android/iOS chat clients. Grok is the first Provider adapter, not the product name. Do not restart items 0–6. Do not scaffold a second Provider. Do not treat leftover Grok Remote identifiers as the current brand.

Shipped native surfaces (Android and iOS, unless noted):

- Device pairing: `anyaicliremote://pair` QR/deep link, Keychain/Keystore, multi-device profiles, disconnect-and-return. Pairing never chooses a workspace.
- macOS launcher: device name, daemon/provider ports, bind and optional public host, start/stop of the owned daemon, pairing QR. No workspace picker.
- Session lifecycle: list from `GET /api/sessions`, new session with explicit `cwd`, load existing session and restore its persisted workspace, history messages, cancel, reconnect without replaying provider history on top of already-loaded chat.
- Chat transcript: streaming assistant/user chunks, thinking, Markdown, code, tables, tool cards with ACP `content` arrays, file attachments via workspace `GET /api/fs/list`, effort via `POST /api/effort`.
- Permission cards: reverse `session/request_permission` (match any method containing `permission`). The daemon copies the command into standard `toolCall.title`; clients never read `_meta`.
- Child-Agent strip: provider-neutral `session/child_agent_update`. Cards are typed lifecycle state only — no prompt, output, or error text.
- Structured interaction: provider-neutral `session/interaction_request` for ask-question and exit-plan. Ask form supports options, Other, per-question notes, cancel, chat-about-this, and skip. Plan sheet supports approve / cancel / abandon.
- Session status bar: ACP `current_mode_update` as a mode badge; Grok `retry_state` and `model_auto_switched` normalized to `session/status_update`.

Daemon REST that exists but is **not** a native client feature: Git, Skills list, loops, voice/TTS, room, stack control, project context. Those routes are daemon/compat surfaces. Do not invent phone UI for them unless the user asks.

Client-origin Provider RPC allowlist: `initialize`, `session/new`, `session/load`, `session/prompt`, `session/cancel`, `session/set_model`, remote ping. Unknown methods fail closed. Reverse file/terminal/permission/interaction travel only Provider → Hub → clients.

Clients consume only provider-neutral or ACP-standard payloads. Grok private wire (`_x.ai/*`, `x.ai/tool`, namespaced `_meta` keys, `subagent_*`, `feedback_request`) is absorbed in `internal/provider/grok`. Adding a client parser for those shapes fails review.

## Current progress

HEAD on `main` is `eb3caa4`. This is not a greenfield repo. Work from this snapshot.

Already on `main` — do not rebuild, re-split, or re-implement:

- TODO items 0–6, including the 4A/4B coordinator split. Android `ChatViewModel` and iOS `ChatStore` already forward into coordinators. That split is finished. Do not add another coordinator.
- Item 8 code: Android ask-form parity (`7dfe4d8`), `session/status_update` (`ac81bb8`), tool-call `content` arrays (`13bf7b9`), permission title + iOS permission-method match (`dd75f1e`), daemon auto-dismiss of `feedback_request` (`fc590e6`).
- YAGNI already decided: no `tool_call_delta_chunk` UI, no extra pipeline for `model_changed`.

Still open — see `TODO.md` **Open** only:

- Item 7 (release). Do not start it unless the user asks.

Follow-scroll collapse and device-live verification are complete. Android uses one list-owned
follow flag with `reverseLayout`; do not restore a FollowController, transcript-signature
`snapshotFlow`, spacer anchor, throttling, or a second follow stack.

`TODO.md` items 0–6 and 8 are done. Do not reopen them.

## Product boundaries

- The product is **Any AI CLI Remote**. New generic code, protocol names, build products, and documentation must not use a provider name as the product name.
- The daemon core is provider-neutral. Provider-specific commands, RPC method names, session layouts, configuration paths, and parsing belong under that provider's adapter.
- The first supported provider is `grok`. Do not add speculative implementations for other providers.
- Starting the daemon may start an idle provider service, but must not create, load, or resume a session.
- Pairing a device never selects a workspace. A workspace belongs to a session: an existing session restores its persisted workspace, and a new session supplies one explicitly.
- File, terminal, Git, skills, and project operations must resolve through the active session workspace. There is no daemon-global project root.

## Compatibility and open-source hygiene

- Current identifiers use `Any AI CLI Remote`, `any-aicli-remote`, `anyaicliremote`, and `com.anyaicliremote` as appropriate.
- Legacy Grok Remote identifiers may appear only in explicitly named compatibility or migration code and tests. Never use them as defaults for new data.
- Never commit local operating-system usernames, home paths, private domains, credentials, pairing keys,
  machine addresses, or generated local state. The repository owner's hosting namespace is allowed only
  where required by the canonical module or repository URL.
- Keep provider credentials inside the provider boundary. The core stores only its own pairing/authentication material.
- Never place pairing or provider secrets in process arguments, routine logs, persisted diagnostics, or crash text.
  Native launchers persist secrets in the platform credential store and may materialize a permission-restricted
  startup file only for the lifetime required by the daemon to read it.

## Code quality

- Use descriptive identifiers. New variable and declaration names shorter than three characters fail the quality gate, except established protocol or platform terms such as `ID`, `URL`, `RPC`, `HTTP`, `OS`, `UI`, and `IP`.
- Hand-written Go source files must not exceed 600 physical lines. A larger file fails the quality gate and must be split by cohesive responsibility within the existing package; comments, regions, generated wrappers, forwarding facades, or duplicate helper layers are not substitutes for a real split.
- Hand-written production Kotlin and Swift source files must not exceed 600 physical lines; Detekt and SwiftLint must report zero issues. Do not use baselines or file-wide suppression; only the narrowest platform-boundary suppression is allowed when accompanied by an explanatory comment. The 4A/4B split already happened. Do not split again to game this limit.
- Protocols, mappings, reducers, and compatibility/migration behavior must reuse one canonical implementation rather than parallel copies.
- Do not wait until a file reaches the hard limit to separate unrelated lifecycle, transport, protocol, persistence, platform, and domain responsibilities. File splitting must preserve one canonical implementation rather than copying shared logic into each file.
- Magic values and scattered operational defaults are forbidden. Ports, bind or public addresses, executable paths, timeouts, retry or polling intervals, resource limits, retention periods, feature switches, and other deployment- or behavior-tunable values must live in the canonical typed configuration or durable settings store, not inline at call sites.
- Fixed protocol values and genuine invariants may remain in code only as descriptively named constants. Test fixtures may use literals when the value is part of the scenario and its meaning is clear from the fixture name.
- The daemon owns one configuration schema, defaults, normalization, and validation path. The command-line interface and macOS launcher must consume that same serialized configuration and state directory; they must not carry independent copies of defaults, field mappings, validation, or migration logic.
- Keep configuration source precedence deterministic and documented. Ordinary durable settings belong in the shared configuration file; structured mutable state may use SQLite, PGlite, or another maintained store only after a dependency decision records the need, migration strategy, and cross-launcher compatibility. Do not introduce a database merely to hide constants.
- Secrets are not ordinary configuration: keep them out of command-line arguments and non-secret settings files. Use the platform credential store or the existing permission-restricted secret-file/environment handoff described above.
- Prefer small interfaces that have real callers. Do not add speculative abstractions or duplicate wrapper layers.
- Preserve cancellation, ownership, and generation checks across asynchronous boundaries.
- Canonicalize paths and enforce root containment before filesystem mutation or process launch. Test symlink and traversal cases.
- Generated Xcode project files must be regenerated from `project.yml`; do not hand-maintain conflicting project settings.

## Task and commit discipline

### Agent delegation and execution

- The primary model owns task decomposition, dependency ordering, acceptance criteria,
  validation, and the final feature commit. It must delegate concrete code editing and
  test-fix work to a low-cost, fast child Agent rather than implementing those changes itself.
- All delegation must use Codex built-in child Agents only, prioritizing the lowest-cost, fastest
  model with low reasoning effort; increase reasoning effort only when the task complexity makes it
  verifiably necessary. All external CLI agents, including provider-specific CLI agents, are prohibited for code,
  test, or documentation edits.
- Built-in child Agents must edit in place in the primary workspace and must not create or switch
  worktrees. The task must begin by checking `pwd` and `git rev-parse --show-toplevel`. After
  completion, the primary model must run `git worktree list --porcelain`, then inspect `git status`
  and `git diff` in the primary workspace to verify the changes landed there; reject the task if the
  primary workspace has no diff.
- The primary model must first converge on a concrete implementation plan. Delegation prompts
  must be narrowly targeted (“指哪打哪”): forbid broad exploration, redesign, or long reasoning;
  require reading only the minimum file set, then editing directly and running the specified tests.
- Every delegated task must state all of the following: exact files or modules, permitted
  modification scope, explicitly forbidden scope, reproduction and acceptance commands, and
  that the child Agent must not commit. Do not send vague requests such as “optimize this”.
- Do not dispatch tasks that modify the same file concurrently. For cross-stack work, complete
  and accept the backend first, then dispatch the dependent App work; never run those phases in
  parallel or let an App infer an unfinished backend contract.
- Child Agents must return the changed files, commands run, and evidence of acceptance. The
  primary model reviews that evidence and performs the final validation and commit.
- Do not spawn a child Agent for a one-file UI bug, a flicker, a scroll-follow fix, or a
  live-test failure. Do that in the primary workspace immediately. Delegation is for a
  bounded implementation after the contract is frozen, not for iterating a broken owner.
- Writing `HANDOFF.md` or checking a TODO box is not a substitute for the failing client
  fix. Do not skip the failing platform by running a different stack's E2E.

- `TODO.md` is the remaining delivery checklist, not a greenfield roadmap. Items 0–6 are done. Do not reopen them, and do not block Android or iOS edits on those old gates.
- The remaining top-level milestone is item 7 (release). Item 8 is done on `main`. Follow-scroll collapse and device-live verification are the open client work in `TODO.md`, not a new feature to design.
- New cross-stack work still goes backend first, then Android, then iOS. That is the order for a new contract, not a reason to rebuild the clients.
- Release signing and notarization remain exclusively in the final release item.
- Prioritize functionality and Debug/Simulator E2E validation. Release signing, notarization, and package
  signature verification belong only to the final release TODO; platform-required local ad-hoc signing may be
  used for launch tests but must not be presented as Release signing.
- Dependencies inside each feature are sequential gates, not parallel suggestions. For every cross-stack feature, finish and validate the backend domain model, protocol, persistence, and tests before editing Android, iOS, macOS, or other app code that consumes it.
- Never make an app guess an unfinished backend contract. Freeze the typed backend payload and lifecycle semantics first; only then implement clients against that verified contract.
- Every top-level TODO item is one coherent feature boundary and one Git commit. Do not mix unrelated features in a commit, and do not split one feature into noisy checkpoint commits merely to record progress.
- Every commit subject must start with one relevant emoji followed by a concise Chinese description, for example `✨ 配置：建立统一守护进程配置`. English-only subjects and conventional-commit-only subjects fail review; product names and technical proper nouns may remain in their canonical spelling inside the Chinese description.
- Mark a TODO item complete only after its stated validation passes. Include that checkbox update in the same feature commit; never pre-check unfinished work.
- Keep the working tree attributable while a feature is in progress. Before starting the next top-level item, the previous item must be validated, checked off, and committed.
- Within a top-level item, complete and check nested TODO boxes strictly from top to bottom. A later phase must stay untouched until all prerequisite boxes above it have passed their validation.
- Test-only evidence and bug fixes discovered while validating a feature belong to that feature's commit. An unrelated defect becomes a new TODO item rather than being hidden in the current commit.

## Required validation

- Backend changes: run `./scripts/check-go-quality.sh` plus focused tests for the changed package.
- Configuration changes: test default generation, file round-tripping, source precedence, validation, and migration. Prove the CLI and macOS launcher resolve the same effective non-secret configuration rather than merely duplicating matching literals.
- Provider changes: test active and archived history, malformed records, session ID validation, workspace restoration, and path containment.
- Lifecycle changes: prove daemon startup sends no create/load/resume request and that two session workspaces cannot contaminate one another.
- Android changes: run unit tests and assemble a debug build.
- Apple changes: regenerate projects and build the relevant simulator/generic destination.
- Before committing, run `git diff --check` and scan tracked files for legacy branding and private identifiers. Legacy matches must be confined to compatibility code and migration tests.

## Public RPC boundary

- Public HTTP/WebSocket/API requests must never accept `command`, `args`, or `stdin` and execute or write directly to a shell/PTY.
- Reverse tool execution is Provider-origin only, over the authenticated upstream connection, with an existing session and bound workspace.
- Reverse methods must be classified by the Provider adapter and fail closed before any public request is forwarded.
- Every Provider adapter must maintain an explicit client-to-agent allowlist based on official protocol SDK constants; unknown methods and reverse methods must fail closed before Ensure/forward.
