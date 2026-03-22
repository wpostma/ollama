# Plan: Delphi Ollama Client (TEdgeBrowser wrapper)

## Goal

Embed the existing Ollama React UI inside a Delphi VCL application using
`TEdgeBrowser` (WebView2). The React UI already works — we just need to host it
in a Delphi-owned window instead of the Go+CGO WebView2 wrapper.

## Architecture

```
┌─────────────────────────────────────┐
│  Delphi VCL App                     │
│  ┌───────────────────────────────┐  │
│  │ TPanel                        │  │
│  │  ┌─────────────────────────┐  │  │
│  │  │ TEdgeBrowser            │  │  │
│  │  │ → http://127.0.0.1:PORT │  │  │
│  │  └─────────────────────────┘  │  │
│  └───────────────────────────────┘  │
│  TTrayIcon  │  Menus  │  StatusBar  │
└──────────┬──────────────────────────┘
           │ HTTP (localhost)
┌──────────▼──────────────────────────┐
│  Go UI Server (subprocess)          │
│  - Serves React SPA (embedded)      │
│  - Handles /api/v1/* (chat, settings│
│  - Proxies /api/* → ollama engine   │
└──────────┬──────────────────────────┘
           │ HTTP (localhost:11434)
┌──────────▼──────────────────────────┐
│  ollama serve                       │
│  - LLM inference engine             │
│  - REST API on 127.0.0.1:11434      │
└─────────────────────────────────────┘
```

## Approach: Two phases

### Phase 1 — Thin Wrapper (MVP)

The Delphi app is a window shell around the existing Go UI server. Minimal
reimplementation, maximum reuse.

#### Steps

1. **Build the Go UI server as a standalone exe**
   - Modify `app/cmd/app/` or create a new `cmd/uiserver/` that:
     - Starts `ollama serve` as a child process (or connects to existing)
     - Runs the UI HTTP server on a **fixed port** (e.g. 3001)
     - Writes the port number to stdout or a known file on startup
     - Runs headless (no WebView, no tray — Delphi owns the window)
   - Build: `go build -trimpath -o ollama-ui-server.exe ./cmd/uiserver`

2. **Build the React SPA**
   - `cd app/ui/app && npm install && npm run build`
   - The Go binary embeds `dist/` via `//go:embed`, so this must happen
     before the Go build

3. **Create the Delphi project**
   - New VCL application (Delphi 11 or 12)
   - Main form:
     - `TPanel` (alClient) containing `TEdgeBrowser`
     - `TTrayIcon` with context menu (Show, Hide, Quit)
     - Optional: `TStatusBar` showing connection status / model name
   - On `FormCreate`:
     - Launch `ollama-ui-server.exe` as a child process (`TProcess` or
       `CreateProcess`)
     - Wait for the server to become ready (poll `GET /api/version`)
     - Navigate `TEdgeBrowser` to `http://127.0.0.1:3001`
   - On `FormClose`:
     - Terminate the child process
   - Handle `TEdgeBrowser` events:
     - `OnNavigationCompleted` — hide splash / loading indicator
     - `OnWebMessageReceived` — future: bridge JS↔Delphi calls

4. **Package**
   - Ship: `OllamaDelphiClient.exe` + `ollama-ui-server.exe` + `ollama.exe`
     + GPU libs
   - Or assume Ollama is already installed and just find `ollama.exe` on PATH

#### Build prerequisites

- **Go** (1.22+) with CGO support (for the UI server's WebView2 dep — but
  we may be able to strip that since Delphi owns the window)
- **Node.js + npm** (to build the React SPA: `npm run build` in `app/ui/app/`)
- **Delphi 11 or 12** with `TEdgeBrowser` component
- **WebView2 Runtime** on the target machine (ships with Windows 10/11)
- **CMake** (only if building GPU backend libs from source)

### Phase 2 — Native Delphi Backend (optional, future)

Replace the Go UI server with Delphi code. This gives full native control
and eliminates the Go dependency.

#### What the Go UI server does (would need reimplementation)

| Endpoint group          | Purpose                                    | Complexity |
|-------------------------|--------------------------------------------|------------|
| Static files (`/`)      | Serve React SPA from embedded dist/        | Trivial    |
| `/api/v1/chats`         | CRUD for chat history (SQLite via store)   | Medium     |
| `/api/v1/chat/{id}`     | Chat streaming (SSE) with tool support     | High       |
| `/api/v1/settings`      | App settings (theme, model, context len)   | Low        |
| `/api/v1/models/pull`   | Model download progress (SSE proxy)        | Medium     |
| `/api/tags` etc.        | Reverse proxy to ollama on 11434           | Low        |
| `/api/me`, `/api/signout` | Auth (ollama.com account)                | Medium     |

#### Delphi components needed

- `TIdHTTPServer` — serve SPA + handle API routes
- `TEdgeBrowser.SetVirtualHostNameToFolderMapping` — alternative to HTTP
  server for static files (maps a virtual hostname to a local folder)
- `TNetHTTPClient` — proxy requests to ollama engine
- SQLite or file-based storage for chat history
- SSE (Server-Sent Events) streaming support in both directions

#### Decision point

Phase 2 is only worth doing if:
- The Go UI server becomes a maintenance burden
- We want to add Delphi-native features (OnePOS integration, custom tools)
- We want to eliminate the Go toolchain from the build

## Key technical details

### React SPA API contract

The SPA uses **relative URLs** (`API_BASE = ""` in production). All fetch
calls go to the same origin. Important endpoints:

```
GET  /api/version          — health check
GET  /api/tags             — list models (proxied to ollama)
POST /api/v1/chat/{id}     — send message, get SSE stream back
GET  /api/v1/chats         — list conversations
GET  /api/v1/settings      — app settings
POST /api/v1/models/pull   — download a model (SSE progress)
```

### Port discovery

The Go UI server currently picks a **random port**. For Phase 1, we need
a fixed or discoverable port. Options:
- Add a `--port` flag to the Go server (cleanest)
- Have it write `{"port": 3001}` to a known file
- Parse stdout for a "listening on :XXXX" log line

### Process lifecycle

```
Delphi app starts
  → launches ollama-ui-server.exe (child process)
    → ollama-ui-server launches ollama serve (grandchild)
  → polls http://127.0.0.1:PORT/api/version until ready
  → navigates TEdgeBrowser to http://127.0.0.1:PORT
  → user interacts with React UI in the Delphi window
Delphi app closes
  → terminates ollama-ui-server.exe
    → ollama-ui-server terminates ollama serve
```

Alternatively, if ollama is already running system-wide (common), the UI
server just connects to it on 11434 instead of spawning it.

## Proven (2026-03-21)

Built and tested the following end-to-end:

1. **React SPA builds cleanly**: `cd app/ui/app && npm install && npm run build`
2. **Go app builds with CGO**: `CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o ollama-app.exe ./app/cmd/app`
   - Requires: Go 1.26+, MinGW-w64 GCC (MSYS2), CMake
   - Produces: 25MB exe with embedded React SPA
3. **Added `--headless` and `--port=N` flags** to `app/cmd/app/app.go`:
   - `--headless` — skips WebView2 window and tray, blocks on SIGINT
   - `--port=3001` — fixed port instead of random
   - Headless sets `Dev: true` on the UI server, which disables token
     cookie validation (required for external browser access)
4. **Edge browser works perfectly** at `http://127.0.0.1:3001`:
   - Model selector works (populates, selection sticks)
   - Changing models works
   - Edge = same WebView2 engine as TEdgeBrowser, so Delphi will be identical

### Key discovery: token cookie

The UI server generates a UUID token and normally injects it via JS into
the WebView2 `document.cookie`. External browsers don't get this cookie,
so all `/api/v1/*` calls return 403 Forbidden. Fix: headless mode sets
`Dev: true` which skips token validation. For production Delphi use, we
should implement proper token passing (e.g. Delphi sets the cookie via
`TEdgeBrowser.ExecuteScript`).

### Modified files

- `app/cmd/app/app.go` — added `headless`, `fixedPort` vars, `--headless`
  and `--port=N` arg parsing, `fixedPort` listener logic, `Dev: devMode || headless`
- `app/cmd/app/app_windows.go` — headless early-return in `osRun()`

## Open questions

- [x] ~~Should we fork the Go UI server code or patch the existing one?~~
      → Patched the existing one with minimal changes (2 files)
- [x] ~~Do we need auth (ollama.com login) support?~~
      → Local-only for now; headless skips token validation
- [ ] Do we want the Delphi app to manage `ollama serve` lifecycle, or
      assume it's already running?
- [ ] Target Delphi version: 11 (Alexandria) or 12 (Athens)?
- [ ] Should the Delphi app set the token cookie via
      `TEdgeBrowser.ExecuteScript` instead of running in Dev mode?
