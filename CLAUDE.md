# Ollama Project — Build Notes

## Branch: warren_custom

Custom fork with headless mode, fixed port, and React UI modifications
for embedding in a Delphi TEdgeBrowser panel.

## Build Prerequisites

- **Go 1.26+** — on PATH
- **MinGW-w64 GCC** — from MSYS2: `C:\msys64\mingw64\bin` must be on PATH
- **Node.js 22+** and **npm** — for React SPA build
- **CMake** — at `C:\Program Files\CMake\bin\cmake.exe` (only needed for GPU backend libs)
- **CGO_ENABLED=1** — required (WebView2 C++ bindings, SQLite)

## Build Steps

The SPA is embedded in the Go binary via `//go:embed`. **SPA must be built
before Go build.** If you rebuild the SPA, you must also rebuild the Go
binary or the changes won't take effect.

```bash
# Ensure MinGW is on PATH (for CGO)
export PATH="/c/msys64/mingw64/bin:$PATH"

# 1. Build React SPA
cd app/ui/app && npm install && npm run build && cd ../../..

# 2. Build Go app (embeds SPA dist/ into binary)
CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o ollama-app.exe ./app/cmd/app

# 3. Kill any existing ollama app instances
taskkill.exe //F //IM "ollama app.exe" 2>/dev/null
taskkill.exe //F //IM ollama-app.exe 2>/dev/null

# 4. Run headless on fixed port (no WebView2 window, no tray)
OLLAMA_DEBUG=1 ./ollama-app.exe --headless --port=3001

# 5. Open http://127.0.0.1:3001 in Edge/Chrome
```

## Custom Flags (warren_custom branch)

- `--headless` — no WebView2 window, no tray icon. Blocks on SIGINT. Sets Dev=true (skips token cookie auth).
- `--port=N` — listen on fixed port instead of random.

## Gotchas

- **The stock "ollama app.exe" auto-restarts.** Kill it before launching
  our build, or the existing instance detection will abort ours.
- **SPA build timestamp must precede Go build.** Go embeds dist/ at
  compile time. If Go builds before the SPA finishes, you get stale UI.
- **MinGW GCC must be on PATH** for CGO. If you get `cgo: C compiler "gcc"
  not found`, add `/c/msys64/mingw64/bin` to PATH.

## React UI Modifications

- `src/components/ModelBadge.tsx` — colored circle badge (2-char tag per model)
- `src/utils/modelBadge.ts` — color assignment + size parsing logic
- `src/components/ModelPicker.tsx` — badge in picker, model name as tooltip only
- `src/components/Logo.tsx` — delphi-ollama helmet llama replaces waving llama
- `src/components/layout/layout.tsx` — auto-hide sidebar when width < 800px
- `src/index.css` — font scaling media queries for narrow viewports (≤480px, ≤360px)
- `index.html` — favicon changed to delphi-ollama.png
- `public/delphi-ollama.png` — helmet llama icon (v2)

## Go Modifications

- `app/cmd/app/app.go` — `--headless`, `--port=N` flags, `Dev: devMode || headless`
- `app/cmd/app/app_windows.go` — headless early-return in `osRun()`

## Documentation

- `plan-delphi-ollama-client.md` — Delphi wrapper architecture and phases
- `ollama-go-code-explore.md` — full codebase index
- `openai-endpoint-reference.md` — tested API endpoints with examples
- `client-side-tool-execution-plan.md` — tool calling via /v1/chat/completions
- `plan-react-ui-improvements.md` — model badges + narrow layout design
