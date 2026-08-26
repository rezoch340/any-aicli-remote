# Security Policy

## Reporting a vulnerability

Please do not report suspected vulnerabilities in a public issue, discussion, pull request, or social media post.

Use GitHub's private vulnerability reporting flow:

[Report a vulnerability privately](https://github.com/rezoch340/any-aicli-remote/security/advisories/new)

Include, when available:

- the affected component and revision;
- prerequisites and a minimal reproduction;
- the observed and expected behavior;
- the potential impact;
- suggested mitigations or fixes; and
- whether the report contains sensitive data that requires special handling.

Do not include real pairing keys, credentials, user transcripts, or unrelated private filesystem paths. Use clearly synthetic values in reproductions.

Maintainers will acknowledge the report through the private advisory, assess impact and affected versions, coordinate a fix, and publish disclosure information when it is safe to do so. Please allow a reasonable investigation period before public disclosure. No bounty or fixed response timeline is promised.

## Scope and deployment responsibility

Security fixes are made on the current development line; older revisions may not receive backports.

The daemon can access session-selected workspaces and acts with the permissions of the account that starts it. Operators are responsible for protecting the host, limiting network exposure, securing transport for their environment, safeguarding pairing material, and reviewing provider permission prompts.

Only the shallow `/health` endpoint is intentionally public. Deep health checks, REST operations, and WebSocket sessions require authentication. Pairing keys must not be passed through plaintext command-line secret arguments.

The repository does not publish a notarized macOS binary. Locally built launcher artifacts are ad-hoc signed by default and should be treated as local development builds.
