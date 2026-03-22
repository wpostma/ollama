# Client-Side Tool Execution Plan

## Problem

Ollama's tool execution loop runs entirely server-side inside
`app/ui/ui.go:chat()`. The model generates `tool_calls`, the Go server
executes them via `registry.Execute()`, feeds results back, and loops
until the model stops calling tools. The client (React UI or Delphi) only
sees the finished results streamed as events.

This means the only tools available are ones compiled into the Go binary
(web_search, web_fetch, browser.*). A Delphi client can't provide its own
tools — database queries, file system access, OnePOS integration, etc. —
because the server has no way to delegate tool execution back to the client.

## Goal

Enable a **client-side tool execution mode** where the headless Go server
pauses its tool loop, sends tool_call events to the client, waits for the
client to execute the tools and POST results back, then resumes the model
conversation.

## What Exists Today

### Server-Side Tool Loop (`app/ui/ui.go:830-1104`)

```
registry = tools.NewRegistry()
// Register built-in tools based on settings + model capabilities
if WebSearchEnabled && hasToolsCapability {
    registry.Register(&tools.WebSearch{})
    registry.Register(&tools.WebFetch{})
}

for {
    toolsExecuted := false
    availableTools := registry.AvailableTools()
    chatReq := buildChatRequest(messages, model, tools: availableTools)

    c.Chat(ctx, chatReq, func(res api.ChatResponse) {
        for _, toolCall := range res.Message.ToolCalls {
            result, content, err := registry.Execute(ctx, toolCall.Function.Name, ...)
            // Store tool message, emit events
            toolsExecuted = true
        }
    })

    if !toolsExecuted { break }
}
```

### Tool Interface (`app/tools/tools.go`)

```go
type Tool interface {
    Name() string
    Description() string
    Schema() map[string]any
    Execute(ctx context.Context, args map[string]any) (any, string, error)
    Prompt() string
}
```

`Execute()` returns:
- `any` — structured result (stored in DB as ToolResult, sent to client as ToolResultData)
- `string` — text content the model sees in the next turn
- `error`

### Event Stream (`app/ui/responses/types.go`)

The server streams `text/jsonl` events to the client:

```json
{"eventName":"chat","content":"Let me search..."}
{"eventName":"tool_call","toolCalls":[{"type":"function","function":{"name":"web_search","arguments":"{\"query\":\"...\"}"}}]}
{"eventName":"tool","toolName":"web_search","content":"Searching..."}
{"eventName":"tool_result","toolName":"web_search","toolResult":true,"toolResultData":{...}}
{"eventName":"chat","content":"Based on the results..."}
{"eventName":"done"}
```

### Tool Registration (runtime)

Tools are registered per-chat-request, not at startup. The global
`toolRegistry` in `app.go` is created empty (`tool_count=0`). Actual tools
are registered inside `chat()` based on:
- `req.WebSearch` flag in the chat request
- Model's `CapabilityTools` (from model metadata)
- `supportsBrowserTools()` check (gpt-oss models get browser.* tools)
- `hasAttachments` — tools are skipped when files are attached

### Chat Request From Frontend (`responses/types.go`)

```go
type ChatRequest struct {
    Model       string       `json:"model"`
    Prompt      string       `json:"prompt"`
    Attachments []Attachment `json:"attachments,omitempty"`
    WebSearch   *bool        `json:"web_search,omitempty"`
    FileTools   *bool        `json:"file_tools,omitempty"`
    Think       any          `json:"think,omitempty"`
}
```

The `WebSearch` and `FileTools` booleans gate which tools are available.
There is no mechanism to pass custom tool definitions from the client.

## Proposed Architecture

### Option A: Synchronous Callback (Server Pauses)

```
Delphi client
  → POST /api/v1/chat/{id} with {prompt, model, tools: [...]}
  ← stream: {"eventName":"tool_call_pending","toolCalls":[...],"callbackId":"abc"}
     [stream pauses — server blocks on a channel]

  → POST /api/v1/chat/{id}/tool-result {callbackId: "abc", results: [...]}
     [server unblocks, feeds results to model, continues loop]

  ← stream: {"eventName":"chat","content":"Based on the results..."}
  ← stream: {"eventName":"done"}
```

**Pros:** Simple client logic. One HTTP stream per conversation turn.
**Cons:** Ties up a server goroutine waiting. HTTP connection must stay
open. Timeout management needed.

### Option B: Multi-Request Loop (Client Drives)

```
Delphi client
  → POST /api/v1/chat/{id} with {prompt, model, tools: [...]}
  ← stream completes: {"eventName":"tool_calls_pending","toolCalls":[...]}

  [client executes tools locally]

  → POST /api/v1/chat/{id} with {toolResults: [...]}
  ← stream: {"eventName":"chat","content":"Based on the results..."}
  ← stream: {"eventName":"done"}
```

**Pros:** Stateless server. No blocking. Clean HTTP semantics.
Each request/response is a complete cycle.
**Cons:** Client must drive the loop. More client-side logic.
Tool results need to be stored correctly in chat history.

### Recommendation: Option B (Client-Driven Loop)

This matches how the OpenAI API works — the client receives tool_calls,
executes them, and sends a follow-up request with tool role messages.
It's simpler to implement, doesn't require keeping HTTP streams open
during tool execution, and the Delphi client already needs to parse the
event stream anyway.

## Implementation Sketch

### Wire Protocol Changes

**Extended ChatRequest** — add optional fields:

```go
type ChatRequest struct {
    // ... existing fields ...
    Tools       []ToolDef    `json:"tools,omitempty"`       // client-provided tool definitions
    ToolResults []ToolResult `json:"tool_results,omitempty"` // results from client-executed tools
}

type ToolDef struct {
    Name        string         `json:"name"`
    Description string         `json:"description"`
    Schema      map[string]any `json:"schema"`
}

type ToolResult struct {
    ToolCallID string `json:"tool_call_id"`
    ToolName   string `json:"tool_name"`
    Content    string `json:"content"`         // text for the model
    Result     any    `json:"result,omitempty"` // structured data for storage
    Error      string `json:"error,omitempty"`
}
```

**New event type** — signals client should execute tools:

```json
{
  "eventName": "tool_calls_pending",
  "toolCalls": [
    {
      "id": "call_abc123",
      "type": "function",
      "function": {
        "name": "query_database",
        "arguments": "{\"sql\": \"SELECT ...\"}"
      }
    }
  ]
}
```

### Server-Side Changes (`app/ui/ui.go`)

1. **Accept `tools` in ChatRequest** — register client-defined tools as
   "proxy" tools that don't execute server-side
2. **Accept `tool_results` in ChatRequest** — when present, skip the user
   message, inject tool role messages, and resume the model conversation
3. **New tool type: ProxyTool** — registered in the registry but its
   `Execute()` returns a sentinel error "client-side execution required"
4. **Modified tool loop** — when a ProxyTool is called, emit
   `tool_calls_pending` event and end the stream. The client sends a
   follow-up request with `tool_results`.

### Client-Side (Delphi) Flow

```pascal
// 1. Send chat with tool definitions
POST /api/v1/chat/{id}
  {"prompt": "...", "model": "...", "tools": [
    {"name": "query_pos", "description": "Query OnePOS database",
     "schema": {"type":"object","properties":{"sql":{"type":"string"}}}}
  ]}

// 2. Parse event stream
while ReadLine(stream) do
begin
  event := ParseJSON(line);
  case event.eventName of
    'chat':               AppendToUI(event.content);
    'thinking':           ShowThinking(event.thinking);
    'tool_calls_pending': ExecuteToolsLocally(event.toolCalls);
    'done':               Break;
  end;
end;

// 3. Execute tools and send results back
POST /api/v1/chat/{id}
  {"tool_results": [
    {"tool_call_id": "call_abc", "tool_name": "query_pos",
     "content": "3 rows returned: ..."}
  ]}

// 4. Parse the continuation stream (may have more tool calls)
// Repeat from step 2
```

### Files to Modify

| File | Change |
|------|--------|
| `app/ui/responses/types.go` | Add `ToolDef`, `ToolResult` to `ChatRequest`. Add `tool_calls_pending` event. |
| `app/ui/ui.go` | Accept client tools + tool_results in `chat()`. Add ProxyTool logic. |
| `app/tools/tools.go` | Add `ProxyTool` type that signals client-side execution. |

## Sequence Diagram

```
Delphi                   Go UI Server              Ollama Engine
  │                         │                          │
  │ POST /chat/{id}         │                          │
  │ {prompt, tools:[...]}   │                          │
  │────────────────────────>│                          │
  │                         │ POST /api/chat           │
  │                         │ {messages, tools}        │
  │                         │─────────────────────────>│
  │                         │                          │
  │                         │ stream: tool_calls       │
  │                         │<─────────────────────────│
  │                         │                          │
  │                         │ [tool is client-side]    │
  │                         │                          │
  │ event: tool_calls_      │                          │
  │        pending           │                          │
  │<────────────────────────│                          │
  │                         │                          │
  │ [execute tool locally]  │                          │
  │                         │                          │
  │ POST /chat/{id}         │                          │
  │ {tool_results:[...]}    │                          │
  │────────────────────────>│                          │
  │                         │ POST /api/chat           │
  │                         │ {messages+tool results}  │
  │                         │─────────────────────────>│
  │                         │                          │
  │                         │ stream: content          │
  │                         │<─────────────────────────│
  │                         │                          │
  │ event: chat, done       │                          │
  │<────────────────────────│                          │
  │                         │                          │
```

## Research Topics

### 1. How does the OpenAI-compatible endpoint handle tool calls today?

The `/v1/chat/completions` endpoint already supports the standard OpenAI
tool-calling protocol where the client drives the loop. We need to
understand:

- Does the OpenAI middleware (`middleware/openai.go`) already pass tool
  definitions through to the model?
- Does it already return `finish_reason: "tool_calls"` when the model
  wants to call tools?
- Can a client already POST back `role: "tool"` messages with results?
- If so, the **entire client-driven loop may already work** via
  `/v1/chat/completions` without any changes to the UI server. The UI
  server's `/api/v1/chat/{id}` would just need to be taught the same
  pattern.

**Action:** Test with curl — send a ChatRequest with tools to
`/v1/chat/completions`, see if the model returns tool_calls, then send
a follow-up with tool results. If this works, the Delphi client can
just use the OpenAI-compatible endpoint directly and skip the UI
server's chat handler entirely.

### 2. Can server-side and client-side tools coexist?

The current design registers tools per-request based on flags. We need
to figure out:

- Should client-provided tool definitions **replace** server-side tools,
  or **merge** with them? (e.g., client provides `query_pos` alongside
  server's `web_search`)
- If they merge, what happens when the model calls a server-side tool
  (execute immediately) vs. a client-side tool (pause and delegate)?
  The tool loop needs to handle a mix.
- Should the server pre-filter which tools to offer the model based on
  capabilities, or should the client be fully responsible for curating
  the tool list?

**Action:** Read `buildChatRequest()` in `ui.go` to see how
`availableTools` gets assembled and passed to the ollama engine. Check
whether the engine's chat handler strips/validates tools or passes them
through verbatim. Determine whether mixing server + client tools in one
request is architecturally clean or creates ordering problems in the
tool execution loop.

### 3. What is the Delphi-side tool execution model?

Before coding the protocol, we need to design how the Delphi client
will actually execute tools:

- **Tool registry in Delphi** — a `TToolRegistry` with registered
  `ITool` implementations. Each tool has a name, schema, and Execute
  method. Mirrors the Go interface.
- **Threading** — tool execution may be slow (database queries, API
  calls). Does it run on the main thread (blocking UI) or a worker
  thread? The HTTP stream is already being consumed on a background
  thread, so tool execution should happen there too.
- **Tool approval UI** — for dangerous tools (file writes, shell
  commands), should Delphi show a confirmation dialog before executing?
  This requires marshalling back to the main thread.
- **Tool result serialization** — Delphi objects need to serialize to
  JSON that the Go server can parse. Standard `TJSONObject` should
  suffice.

**Action:** Sketch the Delphi `ITool` interface and a sample tool
(e.g., `TFileReadTool` or `TPOSQueryTool`) to validate the design
before committing to the wire protocol.

### 4. Does the React frontend need changes?

The current React UI has no concept of client-side tools — it only
displays server-executed tool results. If we want the Ollama web UI
(running in TEdgeBrowser) to also support client-side tools:

- The React event handler would need to recognize `tool_calls_pending`
  events
- It would need to call back to Delphi via `window.chrome.webview.postMessage()`
  (WebView2's JS→native bridge)
- Delphi handles `OnWebMessageReceived`, executes the tool, and POSTs
  results back to the server

This is a more complex integration path. The simpler alternative is to
bypass the React UI for tool-enabled chats and build a native Delphi
chat panel that talks directly to the API.

**Action:** Decide whether tool-enabled conversations should use the
React UI (with JS↔Delphi bridging) or a native Delphi UI. This
affects the entire architecture.
