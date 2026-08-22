# grok-remote 客户端协议

依据 grok-remote 1.9.5 的 `server.py` 与 Web UI 整理。

## 鉴权

服务端接受以下任一种方式：

- Query：`?key=<secret>`
- Cookie：`grok_remote_key=<secret>`
- Header：`X-Grok-Remote-Key: <secret>`

原生客户端使用 Query 连接 WebSocket，REST 使用 Header。

## WebSocket

地址：`ws://host:2421/ws` 或 `wss://host/ws`。

协议是 JSON-RPC 2.0。连接完成后首先发送：

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": 1,
    "clientInfo": { "name": "grok-remote-app", "version": "0.1.0" },
    "clientCapabilities": {}
  }
}
```

### 会话

```text
_x.ai/sessions/list {}
session/new          { cwd, mcpServers: [] }
session/load         { sessionId, cwd, mcpServers: [] }
session/prompt       { sessionId, prompt: [{type:"text", text:"..."}] }
session/cancel       { sessionId }
session/set_model    { sessionId, modelId, _meta:{reasoningEffort} }
```

### 流事件

主要通知为：

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "...",
    "update": {
      "sessionUpdate": "agent_message_chunk",
      "content": { "type": "text", "text": "..." }
    }
  }
}
```

`sessionUpdate` 常见值：

- `user_message_chunk`
- `agent_message_chunk`
- `agent_thought_chunk`
- `tool_call`
- `tool_call_update`
- `plan`
- `session_recap`
- `turn_completed`
- `task_completed`

### 权限请求

服务端可能发送带 `id` 的反向 JSON-RPC 请求，method 包含 `permission`、
`ask_user`，或等于 `session/request_permission`。客户端需要返回相同的 `id`：

```json
{
  "jsonrpc": "2.0",
  "id": 42,
  "result": {
    "outcome": {
      "outcome": "selected",
      "optionId": "allow"
    }
  }
}
```

取消：

```json
{"jsonrpc":"2.0","id":42,"result":{"outcome":{"outcome":"cancelled"}}}
```

## REST

MVP 使用：

- `GET /health`
- `GET /api/session/history?sessionId=...&cwd=...&limit=400&chat_only=1`
- `POST /api/session/rename`
- `GET/POST /api/session/archived`
- `POST /api/effort`

历史接口返回的 `events` 继续按上述 `session/update` 规则归一化。
