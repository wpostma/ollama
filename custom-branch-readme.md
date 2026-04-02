# warren_custom branch

Fork of ollama with UI customizations, API token workaround, and Linux native GUI.

## UI Differences

- **Model badges:** Colored circle with 2-letter abbreviation instead of full model
  name in the picker and chat. Eventually these will be proper icons per model.
  (`ModelBadge.tsx`, `modelBadge.ts`, `ModelPicker.tsx`)
- **Custom logo:** Delphi-ollama helmet llama replaces the stock waving llama.
- **Narrow layout:** Sidebar auto-hides below 800px, font scales down on small
  viewports.
- **Linux GUI:** Native GTK3 + WebKit2GTK desktop app. Connects to the existing
  ollama systemd service instead of starting its own. See `linux-native-gui-app.md`
  for full details.

## API Token Workaround

The stock ollama app uses SSH key signing for authentication (`~/.ollama/id_ed25519`).
This doesn't work in headless/embedded scenarios (Delphi TEdgeBrowser, Linux without
the key file). Two workarounds on this branch:

- `OLLAMA_API_KEY` env var — set this to authenticate web_search and web_fetch tool
  calls directly via Bearer token, bypassing the SSH key signing flow.
- `--headless` flag — sets `Dev=true` which skips the token cookie auth check,
  allowing the UI to be accessed from any browser at the fixed port.

## Build (Linux)

```bash
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config
cd app/ui/app && npm install && npm run build && cd ../../..
CGO_ENABLED=1 go build -o ollama-app ./app/cmd/app/
./ollama-app
```

## Build (Windows)

```bash
export PATH="/c/msys64/mingw64/bin:$PATH"
cd app/ui/app && npm install && npm run build && cd ../../..
CGO_ENABLED=1 go build -o ollama-app.exe ./app/cmd/app
```

See `CLAUDE.md` for detailed build notes and the full file modification index.

## Note

If you only want the Linux native GUI without the other customizations (model
badges, API key workaround, headless mode, Delphi integration), use the
`linux_ui_feature` branch instead. It contains just the Linux GUI changes on
top of upstream main.
