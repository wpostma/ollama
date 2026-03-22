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

## Key Finding (2026-03-22): No Go Changes Needed

**The `/v1/chat/completions` endpoint already supports client-driven tool
loops.** Tested and proven working:

1. Client sends `tools` array in request → model returns `finish_reason: "tool_calls"`
2. Client executes tools locally
3. Client sends results back as `role: "tool"` messages → model responds
4. Parallel tool calls work (multiple tool_calls in one response)
5. Streaming works (SSE with `data:` prefix)
6. No auth token needed on port 11434

This is the standard OpenAI protocol. The Delphi client talks directly to
`http://127.0.0.1:11434/v1/chat/completions` and drives its own tool loop.
The UI server on port 3001 is only needed for the React SPA and chat
persistence.

See `openai-endpoint-reference.md` for full protocol details and examples.

## Original Goal (Revised)

~~Enable a client-side tool execution mode where the headless Go server
pauses its tool loop.~~ **Not needed.** The existing OpenAI-compatible
endpoint already provides this. The remaining work is:

1. Build the Delphi tool registry and loop (client-side only)
2. Optionally extend the UI server's `/api/v1/chat/{id}` to also support
   client-provided tools (for React UI integration — lower priority)

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

**ANSWERED — it works fully.** Tested 2026-03-22 with qwen3:8b.

- Tools pass through to the model: YES
- Returns `finish_reason: "tool_calls"`: YES
- Accepts `role: "tool"` messages with results: YES
- Parallel tool calls: YES (multiple tool_calls in one response)
- Streaming tool calls via SSE: YES
- System messages: YES

**The Delphi client uses `/v1/chat/completions` directly. No Go changes
needed for client-side tool execution.**

Key protocol details:
- Tool arguments are **JSON strings** (not parsed objects) in OpenAI format
- Tool call IDs must be echoed back in `tool_call_id`
- Models need `"tools"` in capabilities (check via `/api/show`)
- Thinking models emit `reasoning` field before tool calls

### 2. Can server-side and client-side tools coexist?

**PARTIALLY ANSWERED.** Two separate paths exist:

- **`/v1/chat/completions` (port 11434)** — pure pass-through. The core
  server sends tools to the model and returns tool_calls to the client.
  No server-side execution. The client provides ALL tools and handles
  ALL execution. This is the Delphi path.

- **`/api/v1/chat/{id}` (port 3001, UI server)** — server-side loop.
  The UI server registers its own tools (web_search, browser.*) and
  executes them. Client-provided tools are NOT supported here.

**For now, these are separate paths — no mixing needed.** The Delphi
client uses the OpenAI endpoint for tool conversations. The React UI
uses the UI server. They don't interfere.

**Future consideration:** If we want the React UI (in TEdgeBrowser) to
also support Delphi-provided tools, we'd need to either:
- Extend the UI server to accept client tool definitions, or
- Have Delphi intercept the React UI's chat requests and redirect
  tool-enabled ones to the OpenAI endpoint

This is a Phase 2 concern. Not blocking.

### 3. What is the Delphi-side tool execution model?

**OPEN — needs design work before coding.**

Now that we know the wire protocol (OpenAI `/v1/chat/completions`),
the Delphi design questions are:

- **Tool registry** — `IOllamaTool` interface with `Name`, `Description`,
  `Schema`, `Execute` methods. `TOllamaToolRegistry` holds registered tools.
  Mirrors Go's `tools.Tool` interface.
- **Threading** — the chat loop (HTTP POST → parse SSE → execute tool →
  POST again) should run on a worker thread. Tool execution happens on
  that same worker thread. UI updates via `TThread.Synchronize`.
- **Tool approval** — for dangerous tools, `Synchronize` to main thread
  to show a confirmation dialog. Worker thread blocks until approved.
  Use Ollama's `x/agent/approval.go` patterns as reference for
  allowlist/denylist.
- **JSON serialization** — `TJSONObject` for tool results. Tool arguments
  arrive as JSON strings from the OpenAI format — parse with
  `TJSONObject.ParseJSONValue`.
- **SSE parsing** — read lines from `TNetHTTPClient` response stream.
  Lines start with `data: `. Skip blanks. Stop on `data: [DONE]`.
  Parse each `data:` payload as JSON. Accumulate `delta.tool_calls`.

**Action:** Sketch `IOllamaTool` interface and a sample
`TFileReadTool` in the WarpExpert project. Validate the threading
model with a simple non-tool chat first, then add tool support.

### 4. Does the React frontend need changes?

**OPEN — but not blocking Phase 1.**

Two UI paths for tool-enabled chats:

**Path A: Native Delphi chat panel (recommended for Phase 1)**
- Delphi builds its own chat UI (TMemo/TRichEdit + tool status panel)
- Talks directly to `/v1/chat/completions` on port 11434
- Full control over tool execution, approval dialogs, result display
- No React/JS changes needed
- The React UI in TEdgeBrowser is still available for non-tool chats

**Path B: React UI + JS↔Delphi bridge (Phase 2)**
- React UI runs in TEdgeBrowser as today
- Tool calls bridge to Delphi via `window.chrome.webview.postMessage()`
- Delphi handles `OnWebMessageReceived`, executes tool, POSTs result
- Requires React changes to emit `postMessage` on `tool_calls_pending`
- More complex but keeps a single unified UI

**Decision:** Start with Path A. It's simpler, proves the tool
infrastructure, and doesn't require React changes. Path B can come
later once the Delphi tool registry is battle-tested.
