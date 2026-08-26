# Contributing to Any AI CLI Remote

Thank you for helping improve Any AI CLI Remote. Issues, documentation fixes, tests, and focused code changes are welcome.

## Before you start

- Read the [Code of Conduct](CODE_OF_CONDUCT.md) and [Security Policy](SECURITY.md).
- Search [existing issues](https://github.com/rezoch340/any-aicli-remote/issues) and pull requests before opening a duplicate.
- Use a [GitHub Issue Form](https://github.com/rezoch340/any-aicli-remote/issues/new/choose) for bugs and feature proposals.
- Report vulnerabilities privately through [GitHub Security Advisories](https://github.com/rezoch340/any-aicli-remote/security/advisories/new).

## Design expectations

- Keep each change focused and avoid unrelated cleanup.
- Reuse existing shared logic rather than duplicating behavior across providers or platforms.
- Keep Grok commands, RPC methods, capabilities, and disk formats inside the Grok provider adapter.
- Preserve session-scoped workspace isolation; do not introduce a daemon-global workspace.
- Avoid speculative provider integrations or interfaces that have no working implementation.
- Never add pairing keys, credentials, user transcripts, private filesystem paths, or other sensitive data to code, fixtures, screenshots, issues, or logs.

## Development and testing

Backend requirements include Go 1.25+. Run the mandatory backend gate from the repository root:

```bash
./scripts/check-go-quality.sh
```

It checks naming and Go file size, formatting, tests, the race detector, and `go vet`.

For the full macOS-hosted validation suite:

```bash
./scripts/build-all.sh
```

This also covers the macOS launcher, Android modules, native source checks, and iOS Simulator build/tests. If your environment cannot run a relevant platform check, state exactly what was not run and why in the pull request.

Add or update tests for behavior changes. Cross-stack contract changes should update the backend and both native clients where applicable.

## Pull requests

1. Create a focused branch in your fork.
2. Make the smallest coherent change.
3. Update documentation when behavior, setup, security, or architecture changes.
4. Run the relevant checks and record the results.
5. Open a [pull request](https://github.com/rezoch340/any-aicli-remote/compare) and complete the template.

A pull request should explain the problem, the chosen approach, security/privacy impact, affected platforms, and validation performed. Maintainers may request changes to preserve provider boundaries, shared abstractions, compatibility behavior, or test coverage.

By contributing, you agree that your contribution is provided under the repository's [MIT License](LICENSE).
