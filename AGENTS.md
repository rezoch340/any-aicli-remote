# Any AI CLI Remote Engineering Rules

## Product boundaries

- The product is **Any AI CLI Remote**. New generic code, protocol names, build products, and documentation must not use a provider name as the product name.
- The daemon core is provider-neutral. Provider-specific commands, RPC method names, session layouts, configuration paths, and parsing belong under that provider's adapter.
- The first supported provider is `grok`. Do not add speculative implementations for other providers.
- Starting the daemon may start an idle provider service, but must not create, load, or resume a session.
- Pairing a device never selects a workspace. A workspace belongs to a session: an existing session restores its persisted workspace, and a new session supplies one explicitly.
- File, terminal, Git, skills, and project operations must resolve through the active session workspace. There is no daemon-global project root.

## Reuse is a quality gate

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
- Backward-compatibility reads and migrations must be centralized. New writes use only the current Any AI CLI Remote names.

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
- Hand-written Go source files must not exceed 600 physical lines. A larger file fails the quality gate and must be split by cohesive responsibility within the existing package; comments, regions, generated wrappers, or duplicate helper layers are not substitutes for a real split.
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

- `TODO.md` is the delivery checklist. Work through its top-level items in order unless the user explicitly changes the order.
- Hard gate: until TODO items 0–3 are all completed, checked off, and committed, do not modify Android or iOS client code. The only exception is an explicit user instruction changing the order.
- After the backend+Launcher E2E contract is frozen, implement clients in this order: Android first, then iOS.
- Release signing and notarization remain exclusively in the final release item.
- Prioritize functionality and Debug/Simulator E2E validation. Release signing, notarization, and package
  signature verification belong only to the final release TODO; platform-required local ad-hoc signing may be
  used for launch tests but must not be presented as Release signing.
- Dependencies inside each feature are sequential gates, not parallel suggestions. For every cross-stack feature, finish and validate the backend domain model, protocol, persistence, and tests before editing Android, iOS, macOS, or other app code that consumes it.
- Never make an app guess an unfinished backend contract. Freeze the typed backend payload and lifecycle semantics first; only then implement clients against that verified contract, in the order recorded by `TODO.md`.
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
