# Linux Native GUI App for Ollama

**Status:** Phase 1 Complete -- working GUI on Ubuntu 24.04 with NVIDIA RTX 3080
**Date:** 2026-03-27
**Branch:** `linux_ui_feature`
**Commit:** `1e1f2249` (39 files changed, 858 insertions, 44 deletions)

---

## Patch Notes (Phase 1)

### What shipped

A fully functional Linux desktop GUI for Ollama, matching the existing macOS and
Windows apps. The app launches a GTK3 window with a WebKit2GTK webview hosting
the same React SPA used on other platforms. It detects and connects to the
existing ollama systemd service rather than starting its own child process.

### New files

| File | Purpose |
|------|---------|
| `app/cmd/app/app_linux.go` | Linux platform entry point: `osRun`, `showWindow`/`hideWindow` via GTK CGO, PID file management, signal handling |
| `app/server/server_linux.go` | System service detection (`/api/version` probe), journalctl log reader for inference compute info, no-op `reapServers` |
| `app/dialog/dlgs_linux.go` | File/directory/message dialogs via `zenity` |
| `app/updater/updater_linux.go` | Stub updater (Linux updates via package manager) |
| `linux-native-gui-app.md` | This document |

### Modified files (35 total)

- **Build constraints:** Added `|| linux` to all `//go:build windows || darwin` files across `app/` (assets, auth, cmd, dialog, format, logrotate, server, store, tools, types, ui, updater, version, webview)
- **`app/webview/webview.go`:** Added Linux CGO flags (`-DWEBVIEW_GTK`, `pkg-config: gtk+-3.0 webkit2gtk-4.1`)
- **`app/server/server.go`:** Added `useExistingServer()` hook and `openServerLog()` platform abstraction
- **`app/server/server_unix.go`:** Added `useExistingServer()` (returns false) and `openServerLog()` stubs
- **`app/server/server_windows.go`:** Same stubs for interface parity
- **`app/cmd/app/app.go`:** Added `xdg-open` for Linux browser launching
- **`app/cmd/app/webview.go`:** Linux CSS injection for viewport height fix, Linux layout flag (`window.__IS_LINUX`), GTK event loop handling (Linux treated like Darwin -- no goroutine), Ctrl+N shortcut for Linux
- **`app/ui/app/src/components/layout/layout.tsx`:** Added `isLinux` detection, reduced title bar spacers (`h-13` to `h-2`), adjusted button positioning for GTK native title bar
- **`app/ui/app/src/components/Chat.tsx`:** Added `isLinux` for padding adjustments
- **`app/ui/app/src/components/ChatSidebar.tsx`:** Settings link visible on Linux (like Windows)
- **`app/ui/app/src/components/Settings.tsx`:** Back arrow navigation on Linux (like Windows), reduced left padding

### Build instructions

```bash
# Install dependencies (Ubuntu 22.04+)
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config

# Build the React SPA
cd app/ui/app && npm install && npm run build && cd ../../..

# Build the app
CGO_ENABLED=1 go build -o ollama-app ./app/cmd/app/

# Run (assumes ollama system service is running)
./ollama-app
```

### Runtime dependencies

- `libgtk-3-0`, `libwebkit2gtk-4.1-0` (GTK3 + WebKit2GTK)
- `zenity` (for native file/directory dialogs)
- `ollama` system service running on port 11434

---

## Retrospective

### What went well

1. **The GTK backend was already there.** The vendored `webview.h` (3900+ lines)
   already contained a complete GTK3 + WebKit2GTK implementation behind
   `#ifdef WEBVIEW_GTK`. Ollama hadn't stripped it -- they just never added CGO
   flags to activate it on Linux. The actual webview enablement was 3 lines of
   CGO directives.

2. **Clean platform abstraction.** The existing codebase has a well-designed
   platform layer: `osRun()`, `showWindow()`, `hideWindow()`, `installSymlink()`,
   etc. Each platform file implements the same interface. Adding Linux was mostly
   filling in the blanks.

3. **System service integration.** The biggest design win was recognizing that
   Linux doesn't need the GUI to manage its own ollama server. The system service
   is already running via systemd. The `useExistingServer()` hook lets the Linux
   GUI connect to the existing service while macOS/Windows continue starting
   their own child processes.

4. **Shared React SPA.** ~95% of the UI is platform-agnostic. The only
   Linux-specific changes were CSS (viewport height), layout spacing (GTK title
   bar vs custom title bar), and button positioning. Total React changes: 4
   files, ~15 lines.

### What was tricky

1. **WEBKIT_DISABLE_DMABUF_RENDERER killed the window.** Setting this env var
   (intended as an NVIDIA workaround) actually prevented the window from
   rendering at all. The webview.h already has its own NVIDIA dmabuf workaround
   that detects `/sys/module/nvidia` and applies the fix automatically. Lesson:
   don't second-guess the library's built-in workarounds.

2. **GTK main loop threading.** GTK's event loop (`gtk_main()`) must run on the
   same OS thread that called `gtk_init()`. The webview library's `init()` calls
   `runtime.LockOSThread()` to pin goroutine 1 to the main thread. On macOS,
   the Cocoa event loop (`C.run()`) takes over this thread. On Windows, the
   webview event loop runs in a goroutine (Windows event loops are per-window).
   On Linux, we needed to call `C.gtk_main()` on the main thread after webview
   setup, matching the Darwin pattern.

3. **Window visibility.** The webview code hides the window immediately after
   creation (`hideWindow(wv.Window())`), expecting the platform to show it later.
   On macOS, native Cocoa callbacks handle this. On Windows, `osRun` explicitly
   shows + centers the window. On Linux, nobody was calling `showWindow`. The fix
   was making `hideWindow` a no-op on Linux since the GTK webview constructor
   already calls `gtk_widget_show_all`.

4. **Inference compute from journalctl.** The `GetInferenceInfo()` function reads
   `serverLogPath` to find GPU info. On Linux with the system service, the local
   log file exists but is empty (created by `openRotatingLog()` but never written
   to since no child server starts). The fix: `openServerLog()` platform hook
   that checks file size and falls back to `journalctl -u ollama -o cat -b`.

5. **JSC_SIGNAL_FOR_GC.** WebKit's JavaScriptCore uses SIGUSR1 for GC, which
   conflicts with Go's signal handling. Setting `JSC_SIGNAL_FOR_GC=42` caused
   WebKit to abort with "invalid option". Removing the env var entirely works --
   the "Overriding existing handler for signal 10" message is a warning, not
   fatal. Go's runtime handles the signal conflict gracefully.

### Decisions made

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Tray library | Deferred to Phase 2 | Not needed for MVP; the window is the primary interface |
| Dialog system | zenity | Simple, no CGO, available everywhere, covers all dialog types |
| Webview approach | Activate existing GTK backend | It was already in webview.h, just needed CGO flags |
| GTK version | GTK 3 | Matches WebKit2GTK availability; GTK 4 would require webkit2gtk-5.0 |
| WebKit2GTK version | 4.1 | Available on Ubuntu 22.04+, better security defaults |
| Server management | Detect system service | Linux installs ollama as a systemd service; the GUI should use it, not fight it |
| Window management | No-hide + GTK show | Simplest approach that works; no need for idle callbacks or deferred show |

### Open issues / Phase 2

- **System tray:** No tray icon yet. When the window is closed, the app exits.
  Phase 2 should add getlantern/systray with Show/Hide/Quit menu.
- **Desktop integration:** No `.desktop` file, no icon in app launcher, no
  `ollama://` URL scheme handler, no autostart.
- **Packaging:** Currently just a raw binary. Need AppImage and/or .deb.
- **Scrollbar styling:** WebKit2GTK's default scrollbars are functional but
  don't match the custom styling on Windows. Could add Linux-specific CSS.
- **Window size persistence:** The `resize` binding saves size to the store,
  but the GTK window doesn't restore it on next launch.
- **Wayland testing:** WebKit2GTK supports Wayland, but untested. GTK3
  auto-selects the backend.
- **Multi-distro testing:** Only tested on Ubuntu 24.04 (GNOME, X11, NVIDIA).
  Should test KDE, Wayland, AMD GPU, Fedora, Arch.

---

## Original Planning Sections

The sections below are the original planning document, preserved for reference.
Items marked with checkmarks were completed in Phase 1.

### Architecture Summary

The Ollama desktop app lives in `app/` and consists of:

| Component | Shared | macOS-specific | Windows-specific | Linux |
|-----------|--------|----------------|-----------------|-------|
| **Main entry** (`app/cmd/app/app.go`) | Core init, logging, server mgmt | `app_darwin.go` -- Cocoa event loop, CGO | `app_windows.go` -- Win32 API, syscall | `app_linux.go` -- GTK main loop, CGO |
| **Webview** (`app/webview/`) | Go wrapper (`webview.go`) | WebKit framework via CGO | Edge WebView2 via CGO | GTK3 + WebKit2GTK via CGO |
| **System tray** | -- | Cocoa NSMenu (in `app_darwin.h`) | `app/wintray/` package (Win32) | Phase 2 |
| **Dialogs** (`app/dialog/`) | Builder API (`dlgs.go`) | `cocoa/` subpackage via CGO | `dlgs_windows.go` via w32 | `dlgs_linux.go` via zenity |
| **Server mgmt** (`app/server/`) | Core logic (`server.go`) | `server_unix.go` -- pkill/pgrep | `server_windows.go` -- wmic | `server_linux.go` -- system service detection |
| **UI server** (`app/ui/`) | HTTP API + React SPA | -- | -- | Reused as-is + CSS fixes |
| **Data store** (`app/store/`) | SQLite (shared) | -- | -- | Reused as-is |
| **Updater** (`app/updater/`) | Core update logic | DMG installer via ObjC | NSIS .exe installer | Stub (package manager) |
| **Login at startup** | -- | LaunchAgent plist | Startup folder shortcut | Phase 2 |

### Implementation Phases

- [x] **Phase 1: Minimal Viable Linux GUI** -- Complete
- [ ] **Phase 2: Desktop Integration** -- .desktop file, icons, autostart, URL scheme, system tray
- [ ] **Phase 3: Packaging** -- AppImage, .deb
- [ ] **Phase 4: Polish** -- Wayland, HiDPI, multi-distro testing

### Dependencies

```bash
# Build
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config

# Runtime
# libgtk-3-0 libwebkit2gtk-4.1-0 zenity (typically already installed on Ubuntu)
```

### Risk Assessment (updated)

| Risk | Impact | Status |
|------|--------|--------|
| Upstream webview divergence | High | **Non-issue:** GTK backend was already in the vendored header |
| WebKit2GTK rendering differences | Medium | **Resolved:** CSS injection fixes viewport height |
| NVIDIA GPU compatibility | Medium | **Resolved:** webview.h auto-detects NVIDIA and applies dmabuf workaround |
| Tray icon inconsistency across DEs | Medium | Deferred to Phase 2 |
| Wayland vs X11 differences | Low | Untested but expected to work (GTK3 auto-selects) |
| JSC signal conflict with Go | Low | **Resolved:** warning is harmless, no action needed |
