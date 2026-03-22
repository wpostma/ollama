# Ollama Go Codebase Exploration

## Top-Level Directory Layout

| Directory | Purpose |
|-----------|---------|
| `api/` | REST API type definitions and Go client library |
| `app/` | Cross-platform desktop GUI app (Windows/macOS) |
| `cmd/` | CLI commands, interactive mode, launch integrations, TUI |
| `server/` | Core HTTP server, API routing, model scheduler, inference pipeline |
| `llm/` | LLM backend abstraction — wraps runner processes via IPC |
| `ml/` | ML infrastructure: device discovery, memory management |
| `runner/` | Model inference runners (llamarunner, ollamarunner, common) |
| `llama/` | CGO wrapper binding to vendored llama.cpp (`llama/vendor`) |
| `converter/` | Format conversion (safetensors, GGUF, GPTQ) |
| `model/` | Model metadata parsing (`parsers/`), rendering (`renderers/`), image processing |
| `manifest/` | OCI manifest layer handling, blob storage, model resolution |
| `template/` | Prompt template management (named templates, auto-detection) |
| `harmony/` | Experimental prompt parsing framework (gpt-oss support) |
| `auth/` | Authentication/authorization |
| `tokenizer/` | Tokenization (BPE, SentencePiece, WordPiece) |
| `middleware/` | HTTP middleware (CORS, RBAC, observability) |
| `openai/` | OpenAI-compatible API translation layer |
| `anthropic/` | Anthropic-specific API extensions |
| `x/` | Experimental: mlxrunner, imagegen, agent, create |
| `parser/` | Config/prompt parsing (YAML, path expansion) |
| `format/` | Formatting utilities (bytes, time, human-readable) |
| `discover/` | GPU/CPU device discovery (NVIDIA, AMD, Intel, Apple) |
| `envconfig/` | Environment variable configuration |
| `version/` | Version and build metadata |
| `progress/` | Progress tracking and reporting |
| `integration/` | Integration tests |
| `docs/` | Documentation |
| `scripts/` | Build scripts (Windows, macOS, Linux, Docker) |

## Desktop App (`app/`) Subtree

| Path | Purpose |
|------|---------|
| `app/cmd/app/app.go` | Main entry point (Windows/macOS) |
| `app/cmd/app/app_windows.go` | Windows: tray, WebView2, window management |
| `app/cmd/app/app_darwin.go` | macOS-specific initialization |
| `app/cmd/app/webview.go` | WebView control — loads React UI, injects token cookie |
| `app/server/server.go` | Managed `ollama serve` subprocess launcher |
| `app/store/` | SQLite-backed app state (chats, settings, user) |
| `app/ui/ui.go` | UI HTTP server — serves SPA, proxies to ollama, chat API |
| `app/ui/app/` | React TypeScript frontend (Vite + TanStack Query) |
| `app/updater/` | Auto-update mechanism |
| `app/auth/connect.go` | Cloud authentication flow |
| `app/wintray/` | Windows system tray icon + menus (Win32 API) |
| `app/webview/` | WebView2 SDK bindings (CGO C++) |
| `app/dialog/` | OS dialog wrappers |
| `app/tools/` | Tool registry for agent/tool-use features |
| `app/logrotate/` | Log rotation for app/server logs |
| `app/assets/` | Embedded icons |
| `app/version/` | App version info |
| `app/ollama.iss` | Inno Setup installer script (Windows) |

## Core Server API Routes (`server/routes.go`)

### Inference

| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| POST | `/api/generate` | GenerateHandler | Text completion (streaming NDJSON) |
| POST | `/api/chat` | ChatHandler | Chat conversation (streaming NDJSON) |
| POST | `/api/embed` | EmbedHandler | Single embedding |
| POST | `/api/embeddings` | EmbeddingsHandler | Batch embeddings |
| GET | `/api/ps` | PsHandler | List running models |

### Model Management

| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| GET/HEAD | `/api/tags` | ListHandler | List local models |
| POST | `/api/show` | ShowHandler | Model details |
| POST | `/api/pull` | PullHandler | Download model (streaming progress) |
| POST | `/api/push` | PushHandler | Upload model (streaming progress) |
| POST | `/api/create` | CreateHandler | Create custom model |
| DELETE | `/api/delete` | DeleteHandler | Delete model |
| POST | `/api/copy` | CopyHandler | Copy model |
| HEAD/POST | `/api/blobs/:digest` | BlobHandler | Blob upload/check |

### Metadata & Auth

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/` | Health check ("Ollama is running") |
| GET/HEAD | `/api/version` | Version info |
| GET | `/api/status` | Cloud status |
| POST | `/api/me` | Current user info |
| POST | `/api/signout` | Sign out |

### OpenAI / Anthropic Compatibility

| Method | Path | Maps to |
|--------|------|---------|
| POST | `/v1/chat/completions` | ChatHandler |
| POST | `/v1/completions` | GenerateHandler |
| POST | `/v1/embeddings` | EmbedHandler |
| GET | `/v1/models` | ListHandler |
| GET | `/v1/models/:model` | ShowHandler |
| POST | `/v1/messages` | ChatHandler (Anthropic) |
| POST | `/v1/responses` | ChatHandler |
| POST | `/v1/images/generations` | GenerateHandler |

### Experimental

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/experimental/web_search` | Web search |
| POST | `/api/experimental/web_fetch` | Fetch web content |

## UI Server Routes (`app/ui/ui.go`)

The desktop app's HTTP server — serves the React SPA and adds chat persistence.

### UI-Specific API (`/api/v1/*`)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/v1/chats` | List all saved chats |
| GET | `/api/v1/chat/{id}` | Get single chat with messages |
| POST | `/api/v1/chat/{id}` | Send message (streams response via text/jsonl) |
| DELETE | `/api/v1/chat/{id}` | Delete chat |
| POST | `/api/v1/create-chat` | Create new chat |
| PUT | `/api/v1/chat/{id}/rename` | Rename chat |
| GET | `/api/v1/inference-compute` | List available GPUs |
| POST | `/api/v1/model/upstream` | Check model updates |
| GET | `/api/v1/settings` | Get app settings |
| POST | `/api/v1/settings` | Update settings |
| GET | `/api/v1/cloud` | Get cloud settings |
| POST | `/api/v1/cloud` | Update cloud settings |

### Proxied to Ollama Engine (localhost:11434)

| Method | Path |
|--------|------|
| GET | `/api/tags` |
| POST | `/api/show` |
| GET | `/api/version` |
| GET | `/api/status` |
| POST | `/api/me` |
| POST | `/api/signout` |

### Frontend

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/` | Serves React SPA (embedded `dist/`) |
| * | `/*` | SPA catch-all (client-side routing) |

### Token Authentication

All `/api/v1/*` routes require a `token` cookie (UUID), except in Dev mode.
The WebView2 wrapper injects it via `document.cookie = "token=..."; path=/`.
Headless mode sets `Dev: true` to skip validation.

## Streaming Protocol

### NDJSON (Core Server)

One JSON object per line, streamed as `application/x-ndjson`:
```
{"response":"Hello","done":false}
{"response":" world","done":false}
{"response":"","done":true,"total_duration":1234567}
```

### text/jsonl (UI Server Chat)

Chat events streamed via `Transfer-Encoding: chunked`:
```json
{"event":"chat","message":{"role":"assistant","content":"Hello"}}
{"event":"thinking","message":{"thinking":"Let me consider..."}}
{"event":"tool_call","tool_call":{...}}
{"event":"complete"}
```

## Key API Types (`api/types.go`)

### ChatRequest
```go
type ChatRequest struct {
    Model     string          // Model name
    Messages  []Message       // Chat history
    Stream    *bool           // Enable streaming (default true)
    Format    json.RawMessage // Response format (e.g. json)
    KeepAlive *Duration       // How long to keep model loaded
    Tools     []Tool          // Available tools
    Options   map[string]any  // Model-specific options (temperature, etc.)
    Think     *ThinkValue     // Thinking/reasoning control
}

type Message struct {
    Role       string      // "user", "assistant", "system"
    Content    string
    Thinking   string      // Thinking text (if Think enabled)
    Images     []ImageData // Attached images
    ToolCalls  []ToolCall
    ToolName   string
    ToolCallID string
}
```

### ChatResponse
```go
type ChatResponse struct {
    Model              string
    CreatedAt          time.Time
    Message            Message
    Done               bool
    DoneReason         string
    TotalDuration      time.Duration
    LoadDuration       time.Duration
    PromptEvalCount    int
    PromptEvalDuration time.Duration
    EvalCount          int
    EvalDuration       time.Duration
}
```

### GenerateRequest / GenerateResponse
Similar to Chat but with `Prompt`/`Response` strings instead of `Messages`.
Adds: `Context []int` (KV cache), `Suffix`, `Raw`, `Images`.

### ProgressResponse (Pull/Push/Create)
```go
type ProgressResponse struct {
    Status    string // "downloading", "verifying", "creating"
    Digest    string // SHA256 digest
    Total     int64  // Total bytes
    Completed int64  // Bytes completed
}
```

## App Store / Persistence (`app/store/`)

SQLite database — lazy-initialized, thread-safe via mutex.

**Database paths:**
- Windows: `%LOCALAPPDATA%\Ollama\db.sqlite`
- macOS: `~/Library/Application Support/Ollama/db.sqlite`

**Key types:**
```go
type Chat struct {
    ID           string           // UUID
    Messages     []Message
    Title        string
    CreatedAt    time.Time
    BrowserState json.RawMessage
}

type Settings struct {
    Expose, Browser, Survey    bool
    Models, WorkingDir         string
    Agent, Tools               bool
    ContextLength              int
    SelectedModel              string
    TurboEnabled               bool
    WebSearchEnabled           bool
    ThinkEnabled               bool
    ThinkLevel                 string // "high", "medium", "low"
    SidebarOpen                bool
    AutoUpdateEnabled          bool
}
```

## Inference Pipeline

```
CLI or HTTP Request
  ↓
server.routes.go → ChatHandler / GenerateHandler
  ↓
server.sched.go → Scheduler.GetRunner()
  ├─ Model already loaded? → reuse
  └─ Queue LlmRequest → processPending()
     ├─ Resolve model from manifest
     ├─ Detect GPU memory, select devices
     └─ llm.NewLlamaServer() → spawn runner subprocess
        ├─ Start `ollama-runner` child process
        ├─ HTTP IPC on random localhost port
        └─ POST /load → load model weights
  ↓
runner.Completion(ctx, req, callback)
  ↓
Callback fires per token → stream to HTTP response
  ↓
Model stays loaded for OLLAMA_KEEP_ALIVE (default 5m)
```

## Key Interfaces / Extension Points

### LlamaServer (`llm/server.go`)
```go
type LlamaServer interface {
    Load(ctx, systemInfo, gpus, requireFull) ([]DeviceID, error)
    Completion(ctx, req, fn) error
    Embedding(ctx, input) ([]float32, int, error)
    Tokenize(ctx, content) ([]int, error)
    Detokenize(ctx, tokens) (string, error)
    Close() error
    EstimatedVRAM() uint64
    EstimatedTotal() uint64
}
```
Implementations: subprocess runner (default), MLX runner (Apple), CGO in-process.

### Scheduler (`server/sched.go`)
Pluggable functions: `loadFn`, `getGpuFn`, `getSystemInfoFn`.

### Model Parsers (`model/parsers/`)
Architecture-specific: Llama, Gemma, Phi, Mistral, etc.
Add `convert_<arch>.go` for new model families.

### Template System (`template/`)
Named templates with auto-detection from GGUF metadata.

### Middleware (`middleware/`)
Gin-based middleware chain: CORS, auth, rate limiting, observability.

## Environment Variables (`envconfig/config.go`)

| Variable | Default | Purpose |
|----------|---------|---------|
| `OLLAMA_HOST` | `127.0.0.1:11434` | Server listen address |
| `OLLAMA_MODELS` | `~/.ollama/models` | Model cache directory |
| `OLLAMA_KEEP_ALIVE` | `5m` | Model unload timeout |
| `OLLAMA_LOAD_TIMEOUT` | `5m` | Model load stall timeout |
| `OLLAMA_ORIGINS` | localhost variants | Allowed CORS origins |
| `OLLAMA_DEBUG` | (unset) | Log level: 0=INFO, 1=DEBUG, 2=TRACE |
| `OLLAMA_FLASH_ATTENTION` | false | Flash Attention optimization |
| `OLLAMA_KV_CACHE_TYPE` | (unset) | K/V cache quantization (e.g. q8_0) |
| `OLLAMA_SCHED_SPREAD` | false | Spread models across all GPUs |
| `OLLAMA_NUM_PARALLEL` | 1 | Concurrent requests per model |
| `OLLAMA_MAX_LOADED_MODELS` | 0 (auto) | Max simultaneous models |
| `OLLAMA_MAX_QUEUE` | 512 | Max queued requests |
| `OLLAMA_CONTEXT_LENGTH` | 0 (auto) | Override context length |
| `OLLAMA_NO_CLOUD` | false | Disable cloud features |
| `OLLAMA_NOHISTORY` | false | Disable readline history |
| `OLLAMA_NOPRUNE` | false | Skip blob pruning on startup |
| `OLLAMA_NEW_ENGINE` | false | Experimental new engine |
| `OLLAMA_VULKAN` | (auto) | Force Vulkan backend |
| `OLLAMA_LLM_LIBRARY` | (auto) | Explicit backend library path |
| `OLLAMA_REMOTES` | `ollama.com` | Allowed registry hosts |
| `OLLAMA_EXPERIMENT` | (unset) | Feature flags (client2, web_search) |
| `OLLAMA_AUTH` | false | Require authentication |
| `OLLAMA_CORS` | false | Enable CORS (UI server) |

## Go Client API (`api/client.go`)

```go
client, _ := api.ClientFromEnvironment()  // reads OLLAMA_HOST

// Streaming chat
client.Chat(ctx, &api.ChatRequest{
    Model:    "llama3",
    Messages: []api.Message{{Role: "user", Content: "Hello"}},
}, func(resp api.ChatResponse) error {
    fmt.Print(resp.Message.Content)
    return nil
})

// List models
list, _ := client.List(ctx)

// Pull model (streaming progress)
client.Pull(ctx, &api.PullRequest{Model: "llama3"}, func(resp api.ProgressResponse) error {
    fmt.Printf("%s: %d/%d\n", resp.Status, resp.Completed, resp.Total)
    return nil
})
```

## Notable Dependencies (go.mod)

| Module | Use |
|--------|-----|
| `github.com/gin-gonic/gin` | HTTP server framework |
| `github.com/spf13/cobra` | CLI framework |
| `github.com/mattn/go-sqlite3` | App store (CGO) |
| `github.com/charmbracelet/bubbletea` | TUI (interactive mode) |
| `github.com/google/uuid` | UUID generation |
| `github.com/x448/float16` | bfloat16/float16 |
| `golang.org/x/sync` | Semaphores, errgroups |
| `golang.org/x/sys` | System calls |
| `golang.org/x/crypto` | SSH key handling |
| `golang.org/x/image` | WebP decoding |

## Key File Locations

| Path | Size | Contents |
|------|------|----------|
| `server/routes.go` | ~78 KB | All HTTP route handlers |
| `server/sched.go` | ~31 KB | Model scheduler |
| `cmd/cmd.go` | ~62 KB | CLI command definitions |
| `api/types.go` | ~39 KB | REST API type definitions |
| `app/ui/ui.go` | ~53 KB | UI server (chat, settings, proxy) |
| `llm/server.go` | | Backend abstraction + subprocess management |
| `llama/llama.go` | ~21 KB | CGO wrapper to llama.cpp |
| `envconfig/config.go` | | Environment variable parsing |
| `app/cmd/app/app.go` | | Desktop app entry point |
| `app/store/store.go` | | SQLite persistence |
| `scripts/build_windows.ps1` | ~24 KB | Windows build script |
| `Makefile.sync` | | llama.cpp upstream sync |

## Our Modifications (headless mode)

### `app/cmd/app/app.go`
- Added `headless bool`, `fixedPort int` vars
- Added `--headless` and `--port=N` CLI arg parsing
- `fixedPort > 0` → listen on fixed port instead of random
- `Dev: devMode || headless` → skip token validation in headless mode

### `app/cmd/app/app_windows.go`
- `osRun()` early-returns in headless mode: no tray, no WebView2, blocks on SIGINT

### Build & Run
```bash
# Build React SPA first
cd app/ui/app && npm install && npm run build && cd ../../..

# Build Go app (requires MinGW-w64 GCC for CGO)
CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o ollama-app.exe ./app/cmd/app

# Run headless on fixed port
OLLAMA_DEBUG=1 ./ollama-app.exe --headless --port=3001
```
