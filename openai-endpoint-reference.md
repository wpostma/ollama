# Ollama API Endpoint Reference (Tested 2026-03-22)

All endpoints tested against `http://127.0.0.1:11434` (ollama serve).
Two API families exist: **native Ollama** (`/api/*`) and **OpenAI-compatible** (`/v1/*`).

## Quick Comparison

| Feature | Native `/api/chat` | OpenAI `/v1/chat/completions` |
|---------|-------------------|-------------------------------|
| Format | NDJSON (`\n`-delimited) | SSE (`data: ...\n\n`) |
| Content-Type | `application/x-ndjson` | `text/event-stream` |
| Tool args | Parsed object (`{"location":"Berlin"}`) | JSON string (`"{\"location\":\"Berlin\"}"`) |
| Thinking | `message.thinking` field | `delta.reasoning` field |
| Done signal | `{"done":true}` | `data: [DONE]` |
| Tool finish | `done_reason: "stop"` | `finish_reason: "tool_calls"` |
| Timing info | Yes (durations, token counts) | Yes (usage object) |

**For Delphi client-side tool execution, use `/v1/chat/completions`.**
It follows the standard OpenAI protocol — no Go changes needed.

---

## OpenAI-Compatible Endpoints (`/v1/*`)

### GET /v1/models

List available models.

```bash
curl http://127.0.0.1:11434/v1/models
```

**Response:**
```json
{
  "object": "list",
  "data": [
    {"id": "qwen3:8b", "object": "model", "created": 1774205476, "owned_by": "library"},
    {"id": "deepseek-coder:6.7b-instruct-de1", "object": "model", "created": 1774198911, "owned_by": "library"}
  ]
}
```

### GET /v1/models/:model

Get a single model's info. Same format as list entry.

---

### POST /v1/chat/completions

Chat with tool support. **This is the primary endpoint for Delphi integration.**

#### Basic Chat (non-streaming)

```bash
curl http://127.0.0.1:11434/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3:8b",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": false
  }'
```

**Response:**
```json
{
  "id": "chatcmpl-640",
  "object": "chat.completion",
  "created": 1774206178,
  "model": "qwen3:8b",
  "system_fingerprint": "fp_ollama",
  "choices": [{
    "index": 0,
    "message": {"role": "assistant", "content": "Hello! How can I help?"},
    "finish_reason": "stop"
  }],
  "usage": {"prompt_tokens": 285, "completion_tokens": 7, "total_tokens": 292}
}
```

#### Streaming Chat (SSE)

Set `"stream": true`. Response is `text/event-stream`:

```
data: {"id":"chatcmpl-25","object":"chat.completion.chunk","created":...,"choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-25","object":"chat.completion.chunk","created":...,"choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}

data: {"id":"chatcmpl-25","object":"chat.completion.chunk","created":...,"choices":[{"index":0,"delta":{"content":""},"finish_reason":"stop"}]}

data: [DONE]
```

Each line starts with `data: `. Stream ends with `data: [DONE]`.

#### Thinking/Reasoning (qwen3, deepseek-r1)

Models with `thinking` capability emit a `reasoning` field:

```json
{"delta": {"role": "assistant", "content": "", "reasoning": "Let me think about this..."}}
```

`reasoning` tokens stream before `content` tokens. The model thinks first,
then responds.

#### Tool Calling (PROVEN WORKING)

**Step 1: Send request with tools**

```bash
curl http://127.0.0.1:11434/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3:8b",
    "messages": [{"role": "user", "content": "What is the weather in San Francisco?"}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get the current weather for a location",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {"type": "string", "description": "City name"},
            "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]}
          },
          "required": ["location"]
        }
      }
    }],
    "stream": false
  }'
```

**Response — model wants to call a tool:**
```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": "",
      "reasoning": "...thinking about how to call the tool...",
      "tool_calls": [{
        "id": "call_d8veg332",
        "index": 0,
        "type": "function",
        "function": {
          "name": "get_weather",
          "arguments": "{\"location\":\"San Francisco\",\"unit\":\"fahrenheit\"}"
        }
      }]
    },
    "finish_reason": "tool_calls"
  }]
}
```

Key signals:
- `finish_reason` = `"tool_calls"` (not `"stop"`)
- `message.tool_calls` array contains one or more calls
- `function.arguments` is a **JSON string** (must be parsed by client)

**Step 2: Execute the tool locally, send result back**

```bash
curl http://127.0.0.1:11434/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3:8b",
    "messages": [
      {"role": "user", "content": "What is the weather in San Francisco?"},
      {"role": "assistant", "content": "", "tool_calls": [
        {"id": "call_d8veg332", "type": "function",
         "function": {"name": "get_weather",
                      "arguments": "{\"location\":\"San Francisco\",\"unit\":\"fahrenheit\"}"}}
      ]},
      {"role": "tool",
       "content": "{\"temperature\": 62, \"unit\": \"fahrenheit\", \"conditions\": \"partly cloudy\"}",
       "tool_call_id": "call_d8veg332"}
    ],
    "stream": false
  }'
```

**Response — model uses the tool result:**
```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": "The current weather in San Francisco is 62°F with partly cloudy conditions."
    },
    "finish_reason": "stop"
  }]
}
```

#### Parallel Tool Calls (PROVEN WORKING)

The model can request multiple tools at once:

```json
"tool_calls": [
  {"id": "call_1", "function": {"name": "get_weather", "arguments": "{\"location\":\"Tokyo\"}"}},
  {"id": "call_2", "function": {"name": "web_search", "arguments": "{\"query\":\"Tokyo restaurants\"}"}}
]
```

Send results for each as separate `role: "tool"` messages,
each with the matching `tool_call_id`.

#### Streaming Tool Calls (SSE)

When streaming, tool calls arrive as deltas near the end of the stream:

```
data: {"choices":[{"delta":{"reasoning":"...thinking..."}}]}
data: {"choices":[{"delta":{"reasoning":"...more thinking..."}}]}
data: {"choices":[{"delta":{"tool_calls":[{"id":"call_abc","index":0,"type":"function","function":{"name":"get_weather","arguments":"{\"location\":\"Paris\"}"}}]}}]}
data: {"choices":[{"delta":{"content":""},"finish_reason":"tool_calls"}]}
data: [DONE]
```

The client accumulates `tool_calls` from deltas, then executes after `[DONE]`.

#### System Messages

Fully supported:

```json
"messages": [
  {"role": "system", "content": "You are a helpful assistant. Always use tools when available."},
  {"role": "user", "content": "Look up the capital of France."}
]
```

#### Options / Parameters

Standard OpenAI parameters work:

| Parameter | Type | Default | Notes |
|-----------|------|---------|-------|
| `temperature` | float | model default | 0.0-2.0 |
| `top_p` | float | model default | Nucleus sampling |
| `max_tokens` | int | model default | Max output tokens |
| `stream` | bool | true | SSE streaming |
| `stop` | string[] | none | Stop sequences |
| `frequency_penalty` | float | 0 | |
| `presence_penalty` | float | 0 | |

---

### POST /v1/completions

Text completion (not chat). No tool support.

```bash
curl http://127.0.0.1:11434/v1/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "deepseek-coder:6.7b-instruct-de1", "prompt": "function fibonacci(n) {", "max_tokens": 50, "stream": false}'
```

**Response:**
```json
{
  "id": "cmpl-116",
  "object": "text_completion",
  "choices": [{"text": "...", "index": 0, "finish_reason": "length"}],
  "usage": {"prompt_tokens": 286, "completion_tokens": 50, "total_tokens": 336}
}
```

---

### POST /v1/embeddings

Generate embeddings.

```bash
curl http://127.0.0.1:11434/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{"model": "nomic-embed-text:latest", "input": "Hello world"}'
```

**Response:**
```json
{
  "object": "list",
  "data": [{"object": "embedding", "embedding": [-0.006, 0.031, ...], "index": 0}],
  "model": "nomic-embed-text:latest",
  "usage": {"prompt_tokens": 0, "total_tokens": 0}
}
```

- `nomic-embed-text:latest` produces 768-dimensional vectors
- `input` can be a string or array of strings for batch

---

## Native Ollama Endpoints (`/api/*`)

### GET /api/version

```json
{"version": "0.18.0"}
```

### GET /api/tags

List models (same data as `/v1/models` but different format):

```json
{
  "models": [
    {"name": "qwen3:8b", "model": "qwen3:8b", "size": 5294886912,
     "details": {"family": "qwen3", "parameter_size": "8.2B", "quantization_level": "Q4_K_M"},
     "modified_at": "2026-03-20T11:11:16..."}
  ]
}
```

### POST /api/show

Model details including capabilities, template, and parameters:

```bash
curl http://127.0.0.1:11434/api/show -d '{"model": "qwen3:8b"}'
```

Key fields: `details`, `capabilities` (`["completion","tools","thinking"]`),
`template`, `model_info`, `parameters`.

### POST /api/chat

Native chat with tool support. Same tool-calling protocol as OpenAI but
with different wire format.

**Key differences from `/v1/chat/completions`:**
- `tool_calls[].function.arguments` is a **parsed object**, not a JSON string
- Thinking is in `message.thinking`, not `message.reasoning`
- Includes timing info: `total_duration`, `load_duration`, `eval_count`, etc.
- `done_reason: "stop"` (not `finish_reason: "tool_calls"`)
- Streaming uses NDJSON, not SSE

**Non-streaming with tools:**
```bash
curl http://127.0.0.1:11434/api/chat \
  -d '{"model":"qwen3:8b", "messages":[{"role":"user","content":"Weather in Berlin"}],
       "tools":[{"type":"function","function":{"name":"get_weather","description":"Get weather",
       "parameters":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}}}],
       "stream":false}'
```

**Response:**
```json
{
  "message": {
    "role": "assistant",
    "content": "",
    "thinking": "...model reasoning...",
    "tool_calls": [{
      "id": "call_z3bqfeov",
      "function": {"index": 0, "name": "get_weather", "arguments": {"location": "Berlin"}}
    }]
  },
  "done": true,
  "total_duration": 1164712115,
  "eval_count": 97
}
```

**Tool result round-trip:**
```bash
curl http://127.0.0.1:11434/api/chat \
  -d '{"model":"qwen3:8b", "messages":[
    {"role":"user","content":"What is the weather in Berlin?"},
    {"role":"assistant","content":"","tool_calls":[
      {"function":{"name":"get_weather","arguments":{"location":"Berlin"}}}
    ]},
    {"role":"tool","content":"{\"temperature\":15,\"conditions\":\"rainy\"}"}
  ], "stream":false}'
```

### POST /api/generate

Text generation (completion mode). Supports images for multimodal models.

```bash
curl http://127.0.0.1:11434/api/generate \
  -d '{"model":"deepseek-coder:6.7b-instruct-de1", "prompt":"Write hello world in Pascal", "stream":false}'
```

### POST /api/embed

Embeddings (native format):

```bash
curl http://127.0.0.1:11434/api/embed \
  -d '{"model":"nomic-embed-text:latest", "input":"Hello world"}'
```

### POST /api/pull

Download a model (streaming progress):

```bash
curl http://127.0.0.1:11434/api/pull -d '{"model":"llama3:8b", "stream":true}'
```

Streams `{"status":"downloading","digest":"sha256:...","total":1234,"completed":567}` lines.

### DELETE /api/delete

```bash
curl -X DELETE http://127.0.0.1:11434/api/delete -d '{"model":"modelname"}'
```

### POST /api/copy

```bash
curl http://127.0.0.1:11434/api/copy -d '{"source":"qwen3:8b","destination":"my-model"}'
```

### GET /api/ps

List currently loaded/running models:

```json
{"models":[{"name":"qwen3:8b","size":5294886912,"digest":"...","expires_at":"..."}]}
```

---

## Client-Side Tool Execution Protocol

**No Go changes needed.** Use `/v1/chat/completions` directly.

### Delphi Pseudocode

```pascal
procedure TChat.SendMessage(const Prompt: string);
var
  Request, Response: TJSONObject;
  ToolCalls: TJSONArray;
begin
  // Build request with user message + tool definitions
  Request := BuildChatRequest(Prompt, FModel, FMessages, FTools);

  repeat
    // POST to /v1/chat/completions
    Response := HTTPPost('http://127.0.0.1:11434/v1/chat/completions', Request);

    // Check finish_reason
    if GetFinishReason(Response) = 'tool_calls' then
    begin
      // Extract tool calls
      ToolCalls := GetToolCalls(Response);

      // Add assistant message (with tool_calls) to history
      FMessages.Add(GetAssistantMessage(Response));

      // Execute each tool locally
      for TC in ToolCalls do
      begin
        Result := FToolRegistry.Execute(TC.FunctionName, TC.Arguments);

        // Add tool result message to history
        FMessages.Add(BuildToolMessage(TC.ID, Result));
      end;

      // Rebuild request with updated message history
      Request := BuildChatRequest('', FModel, FMessages, FTools);
    end
    else
    begin
      // Normal response — display to user
      DisplayResponse(GetContent(Response));
      FMessages.Add(GetAssistantMessage(Response));
      Break;
    end;
  until False;
end;
```

### Key Implementation Notes

1. **Arguments are JSON strings** in the OpenAI format. Parse with
   `TJSONObject.ParseJSONValue(TC.Function.Arguments)`.

2. **Tool call IDs** must be echoed back in `tool_call_id` on the
   tool result message. The model uses these to correlate results.

3. **Parallel tool calls** — the model may return multiple `tool_calls`.
   Execute all of them, add all results as separate `role: "tool"`
   messages, then send the next request.

4. **Streaming** — for SSE, parse lines starting with `data: `. Skip
   empty lines. Stop on `data: [DONE]`. Accumulate `delta.tool_calls`
   across chunks.

5. **Model capabilities** — check `/api/show` for `capabilities` array.
   Only models with `"tools"` can use tool calling. As of this test:
   `qwen3:8b` has tools, `deepseek-coder` does not.

6. **No auth required** — the `/v1/*` and `/api/*` endpoints on the
   core ollama server (port 11434) have no token authentication.
   The token cookie is only on the UI server (port 3001).
