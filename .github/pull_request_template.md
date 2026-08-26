## Summary

<!-- What problem does this solve, and why is this approach appropriate? -->

## Changes

- 

## Validation

<!-- List exact commands/checks run and their results. Explain any relevant checks not run. -->

- [ ] Relevant automated tests were added or updated.
- [ ] `./scripts/check-go-quality.sh` passes for backend changes.
- [ ] Relevant iOS, Android, and/or macOS checks pass.
- [ ] Documentation was updated where behavior, setup, architecture, or security changed.

## Reuse and architecture

- [ ] I reused existing shared logic instead of duplicating cross-provider or cross-platform behavior.
- [ ] Provider-specific commands, RPC methods, capabilities, and disk formats remain in the provider adapter.
- [ ] Session-scoped workspace isolation is preserved; no daemon-global workspace was introduced.
- [ ] This change does not claim or scaffold speculative provider support.

## Privacy and security

- [ ] No pairing keys, credentials, user transcripts, private paths, or other sensitive data are included.
- [ ] Authentication, path validation, process ownership, and permission-prompt implications were reviewed where relevant.
- [ ] Security-sensitive details are being coordinated privately rather than disclosed in this pull request.

## Cross-stack impact

- [ ] Backend, iOS, and Android contracts/models were updated together where applicable.
- [ ] Compatibility and migration behavior was considered.
- [ ] Affected platforms and intentionally unaffected platforms are identified below.

**Affected platforms/components:**

<!-- Backend / iOS / Android / macOS launcher / docs / tooling -->

## Related issue

<!-- Example: Closes #123 -->
