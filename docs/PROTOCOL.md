# Any AI CLI Remote protocol

Any AI CLI Remote exposes one authenticated HTTP/WebSocket transport and delegates CLI-specific
behavior to a configured Provider. The current Provider ID is `grok`.

The WebSocket payload follows JSON-RPC 2.0 and Agent Client Protocol (ACP) where the Provider
supports it. The Go implementation reuses [`coder/acp-go-sdk`](https://github.com/coder/acp-go-sdk)
for ACP types and keeps xAI extension methods inside the Grok adapter.

## Authentication

The daemon accepts the pairing secret through one of these mechanisms:

- Query: `?key=<secret>`
- Cookie: `any_aicli_remote_key=<secret>`
- Header: `X-Any-AI-CLI-Remote-Key: <secret>`

Native clients use Query authentication for WebSocket and Header authentication for REST.
Legacy cookie/header names are accepted only by the centralized compatibility layer; current
servers never emit or persist them as new identifiers.

An unauthenticated remote request never receives a pairing URL, QR payload, cookie, or secret.
Pairing material is shown only by the trusted local launcher or by an already authenticated endpoint.
Loopback peers are not an authentication exception: every HTTP/WebSocket route except the shallow
`/health` probe requires the pairing secret. `/health/deep` may start or reconnect the Provider and is
therefore authenticated. Browser WebSocket upgrades also require a same-host Origin;
native clients may omit Origin.

## Device pairing

The macOS launcher emits this deep link:

```text
anyaicliremote://pair?url=<encoded-base-url>&key=<encoded-secret>&name=<encoded-device-name>
```

- `url` and `key` are required; `name` is optional.
- Pairing contains no workspace and must not create, load, or resume a session.
- Clients normalize the server URL and update the matching saved device instead of duplicating it.
- The pairing key belongs in Keychain/Keystore-backed storage, not ordinary preferences.
- `grokremote://pair` is a read-only upgrade path in native clients, not a current output format.

## WebSocket initialization

Connect to `ws://host:2421/ws?key=...` or the corresponding `wss://` URL, then initialize:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": 1,
    "clientInfo": { "name": "any-aicli-remote-app", "version": "0.1.0" },
    "clientCapabilities": {
      "fs": { "readTextFile": true, "writeTextFile": true },
      "terminal": true
    }
  }
}
```

The Grok adapter augments missing file/terminal capabilities before forwarding initialization.
The daemon caches no request that would create or resume a session during startup.

## Session and workspace lifecycle

The daemon maintains a binding of `(providerId, sessionId) -> canonical workspace`.

### New session

```text
session/new { cwd, mcpServers: [] }
```

`cwd` is required. The Provider adapter canonicalizes the directory before forwarding the request
and records the returned session ID only after a successful response.

### Existing session

```text
session/load { sessionId, mcpServers: [] }
```

The daemon resolves `sessionId` through Provider history metadata and restores the persisted
workspace. A client-supplied `cwd` cannot override an existing session's workspace.

### Prompt and cancellation

```text
session/prompt { sessionId, prompt: [{ type: "text", text: "..." }] }
session/cancel { sessionId }
session/set_model { sessionId, modelId, _meta: { reasoningEffort } }
```

Every workspace-bound reverse request and REST operation resolves the same session binding. File,
terminal, Git, project, Skills, and loop operations must reject a missing/unknown session instead of
falling back to process CWD or a daemon-global root.

## Streaming updates

The primary ACP notification is:

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "providerId": "grok",
    "sessionId": "...",
    "update": {
      "sessionUpdate": "agent_message_chunk",
      "content": { "type": "text", "text": "..." }
    }
  }
}
```

Common `sessionUpdate` values include:

- `user_message_chunk`
- `agent_message_chunk`
- `agent_thought_chunk`
- `tool_call`
- `tool_call_update`
- `plan`
- `session_recap`
- `turn_completed`
- `task_completed`

Clients append chunks to stable message IDs and finalize the active turn on terminal notifications;
they must not rebuild the entire transcript on each chunk. The Hub adds the authoritative
`providerId` to every session-scoped notification. Clients fail closed when either identity is missing
or when `(providerId, sessionId)` does not match the open conversation.

### Provider-neutral child Agent updates

The daemon normalizes Provider child-Agent lifecycle notifications to one method:

```json
{
  "jsonrpc": "2.0",
  "method": "session/child_agent_update",
  "params": {
    "providerId": "grok",
    "sessionId": "parent-session",
    "event": {
      "eventId": "parent-session-11",
      "sequence": 11,
      "occurredAt": 1776900123000,
      "replay": false,
      "kind": "completed",
      "agent": {
        "providerChildId": "child-1",
        "parentSessionId": "parent-session",
        "childSessionId": "child-session-1",
        "status": "completed",
        "description": "Index project files and run focused tests.",
        "durationMs": 8421,
        "toolCallCount": 3,
        "turnCount": 2,
        "tokensUsed": 1840,
        "toolsUsed": ["grep", "go test"]
      }
    }
  }
}
```

`kind` is fixed to `started`, `progress`, `completed`, `failed`, `cancelled`, or `updated`.
`status` is fixed to `running`, `completed`, `failed`, `cancelled`, or `unknown`.

`sequence` comes from the Provider's durable event ID suffix and is allowed to be `0`. Clients should
track `(providerId, sessionId, providerChildId)` and use `sequence` for deduplication and for rejecting
older lifecycle state that arrives late. `progress` is transient and may omit `sequence`; when that
happens, clients should treat `durationMs` as a non-decreasing progress signal for the same running
entity, and a terminal state (`completed`, `failed`, `cancelled`) must never be downgraded by a later
or replayed `progress` update.

Prompt text, output text, error text, terminal streams, and Markdown never enter this payload. Grok
child-Agent extensions that are malformed or use unknown `subagent_*` variants fail closed and are not
forwarded to clients.

## Reverse requests

ACP file and terminal requests include `sessionId`. The daemon validates the request against the
canonical workspace bound to that session before touching the filesystem or launching a process.

Supported ACP operations include:

```text
fs/read_text_file
fs/write_text_file
terminal/create
terminal/output
terminal/wait_for_exit
terminal/kill
terminal/release
session/request_permission
```

Provider aliases and extension names are classified only inside the Provider adapter. The generic
Hub operates on normalized operations and never contains Grok method strings.

### Permission response

Permission requests are forwarded only to authenticated clients that have sent traffic for the matching
`(providerId, sessionId)`. The first response from an eligible client wins; responses from unrelated
device conversations are ignored. If no matching client is connected, all eligible clients disconnect,
or the response deadline expires, the Hub returns `cancelled` and never silently selects an allow option.

Return the same JSON-RPC request ID:

```json
{
  "jsonrpc": "2.0",
  "id": 42,
  "result": {
    "outcome": { "outcome": "selected", "optionId": "allow" }
  }
}
```

Cancellation:

```json
{"jsonrpc":"2.0","id":42,"result":{"outcome":{"outcome":"cancelled"}}}
```

## History model

Provider-neutral session metadata uses this shape:

```json
{
  "providerId": "grok",
  "sessionId": "...",
  "title": "...",
  "summary": "...",
  "projectDir": "/canonical/workspace",
  "createdAt": 1776900000000,
  "lastActiveAt": 1776900123000,
  "sourcePath": "/canonical/provider/source/summary.json",
  "resumeCommand": "grok --resume ..."
}
```

Metadata listing is lightweight and sorted by `lastActiveAt` (falling back to `createdAt`) descending.
Messages are loaded separately and normalized to `role`, `content`, and millisecond `timestamp`.

For Grok, the adapter scans active and archived roots, validates `summary.info.id`, then reads
`chat_history.jsonl` on demand. It accepts string/object/array content, ACP text blocks, tool calls,
tool results, numeric seconds/milliseconds, and RFC 3339 timestamps. Reasoning/internal records are
excluded from normalized chat messages.

The compatibility history endpoint remains:

```text
GET /api/session/history?sessionId=...&limit=400&chat_only=1
```

The response always includes a non-null `childAgents: []` snapshot for the parent session, even when
`chat_only=1` filters lifecycle events out of the `events` array. For Grok, persisted child-Agent
lifecycle records are normalized to the same `session/child_agent_update` shape described above and
preserve on-disk file order inside `events`.

On reconnect, clients should rebuild child-Agent state from `childAgents` first, then merge replayed
`session/child_agent_update` events by `sequence`/`eventId`. History truncation or filtering may remove
older lifecycle events, but it must not remove the authoritative `childAgents` snapshot. `chat_only=1`
may hide lifecycle events from `events` while still returning the snapshot needed for state recovery.

The server resolves Provider and workspace from the session. A legacy `cwd` parameter is not an
authority for changing the session binding.

Other compatibility routes include session rename/archive/signals and the existing health/config/
stack endpoints. New provider-neutral clients should prefer metadata and session IDs rather than
encoded workspace paths.

## RPC direction boundary

Public HTTP and WebSocket clients may send protocol requests only toward the configured Provider agent. Any method classified by the Provider adapter as a reverse tool (including terminal, filesystem, and permission tools) is rejected at the Hub boundary; notifications are dropped and requests receive a JSON-RPC error before agent startup or forwarding. `Hub.CallRPC` follows the same rule.

Grok additionally enforces an explicit client-origin allowlist: `initialize`, `session/new`,
`session/load`, `session/prompt`, `session/cancel`, `session/set_model`, and the remote ping
extension. Unknown Provider RPC methods are not transparently forwarded.

Reverse tool requests travel only from the authenticated Provider upstream connection toward the Hub. Terminal execution requires an existing session and its bound workspace; this is not a public command, argument, stdin, shell, or PTY API. Fixed operations such as starting a configured Provider are lifecycle controls, not arbitrary command execution interfaces.
