# Linux Native GUI App for Ollama

**Status:** Phase 1 MVP - Compiles and links successfully
**Date:** 2026-03-27
**Goal:** Build a Linux/Ubuntu native GUI equivalent to the existing macOS and Windows desktop apps in `app/`.

---

## 1. Current Architecture Summary

The Ollama desktop app lives in `app/` and consists of:

| Component | Shared | macOS-specific | Windows-specific | Linux |
|-----------|--------|----------------|-----------------|-------|
| **Main entry** (`app/cmd/app/app.go`) | Core init, logging, server mgmt | `app_darwin.go` — Cocoa event loop, CGO | `app_windows.go` — Win32 API, syscall | **MISSING** |
| **Webview** (`app/webview/`) | Go wrapper (`webview.go`) | WebKit framework via CGO | Edge WebView2 via CGO | **MISSING** |
| **System tray** | — | Cocoa NSMenu (in `app_darwin.h`) | `app/wintray/` package (Win32) | **MISSING** |
| **Dialogs** (`app/dialog/`) | Builder API (`dlgs.go`) | `cocoa/` subpackage via CGO | `dlgs_windows.go` via w32 | **MISSING** |
| **Server mgmt** (`app/server/`) | Core logic (`server.go`) | `server_unix.go` — pkill/pgrep, XDG paths | `server_windows.go` — wmic, LOCALAPPDATA | **Reuse `server_unix.go`** |
| **UI server** (`app/ui/`) | HTTP API + React SPA | — | — | **Reusable as-is** |
| **Data store** (`app/store/`) | SQLite (shared) | — | — | **Reusable as-is** |
| **Updater** (`app/updater/`) | Core update logic | DMG installer via ObjC | NSIS .exe installer | **MISSING** |
| **Login at startup** | — | LaunchAgent plist | Startup folder shortcut | **MISSING** |

**Key insight:** The app uses a webview-hosted React SPA for all UI. No native widgets. The platform layer is thin: tray icon, window management, dialogs, process management.

---

## 2. What Needs to Be Built

### 2.1 Webview Backend (GTK + WebKit2)

The vendored webview library (`app/webview/`) is a fork of the [webview](https://github.com/nicoria/webview) project by Serge Zaitsev / Steffen André Langnes. The upstream C/C++ library supports three backends:

- `WEBVIEW_COCOA` — macOS (currently used)
- `WEBVIEW_EDGE` — Windows (currently used)
- `WEBVIEW_GTK` — Linux via GTK3 + WebKit2GTK (**stripped from ollama's vendor**)

**Approach:** Re-add the GTK backend from upstream webview. This requires:

- Adding `#cgo linux` directives to `webview.go`:
  ```
  #cgo linux CXXFLAGS: -DWEBVIEW_GTK -std=c++11
  #cgo linux pkg-config: gtk+-3.0 webkit2gtk-4.1
  #cgo linux LDFLAGS: -ldl
  ```
- Restoring the GTK implementation in `webview.cc` / `webview.h` from the upstream project
- **Build dependency:** `libgtk-3-dev`, `libwebkit2gtk-4.1-dev`

**Risk:** The upstream webview project has evolved. Need to check compatibility with ollama's vendored version. May need to update the vendor or cherry-pick the GTK backend.

### 2.2 System Tray (`app/linuxtray/` or use getlantern/systray)

**Option A: getlantern/systray** (recommended)
- 3.7k GitHub stars, mature, cross-platform
- Linux support via `libappindicator3` or `libayatana-appindicator3` (CGO)
- Already has a webview example in their repo
- Apache-2.0 license (compatible with ollama's MIT)
- Used by 1.6k+ projects
- **Build dependency:** `libayatana-appindicator3-dev` (Ubuntu 22.04+) or `libappindicator3-dev` (older)

**Option B: Custom GTK tray via gotk3**
- More control, tighter integration
- Heavier dependency, more code to maintain
- Overkill for a tray icon + menu

**Option C: dbus/StatusNotifierItem protocol directly**
- No CGO needed
- Complex to implement correctly
- Would handle KDE, GNOME, etc. natively

**Recommendation:** Option A. getlantern/systray is battle-tested and maps cleanly to the existing `wintray` pattern. The tray needs: icon, tooltip, menu items (Show/Hide, Models, Quit, update notification).

### 2.3 Platform Entry Point (`app/cmd/app/app_linux.go`)

New file implementing:

- `osRun()` — Initialize tray, start GTK main loop (or systray.Run)
- `handleExistingInstance()` — Check for running instance (PID file from `server_unix.go`)
- `showWindow()` / `hideWindow()` — GTK window show/hide or webview visibility
- `installSymlink()` — Create `/usr/local/bin/ollama` symlink (or skip if already installed via package manager)
- `registerLoginItem()` — Create `.desktop` file in `~/.config/autostart/`

### 2.4 Dialog System (`app/dialog/dlgs_linux.go`)

**Option A: Zenity/kdialog** (simplest)
- Shell out to `zenity` (GNOME) or `kdialog` (KDE)
- No CGO needed
- Available on virtually all Linux desktops
- Covers: message boxes, file open/save, directory browse

**Option B: GTK dialogs via gotk3**
- Native, no external dependency
- Requires CGO + GTK dev libs (already needed for webview)
- More code to write

**Option C: Portal API (xdg-desktop-portal)**
- Modern, sandbox-friendly (Flatpak/Snap compatible)
- D-Bus based, no CGO
- Best for future-proofing

**Recommendation:** Option A for initial implementation (fast, reliable), with a plan to migrate to Option C for future Flatpak/Snap packaging.

### 2.5 Server Management (`app/server/`)

`server_unix.go` already works for Linux. It uses:
- PID file at `~/.ollama/ollama.pid` (via `server.PIDFile()`)
- Server log at `~/.ollama/logs/server.log`
- `pgrep`/`pkill` for process management
- `os.Interrupt` signal for graceful shutdown

**Needed changes:**
- Verify paths are appropriate for Linux (XDG compliance: `$XDG_DATA_HOME`, `$XDG_CONFIG_HOME`)
- The current paths (`~/.ollama/`) match what the CLI already uses on Linux, so no change needed initially

### 2.6 Build Constraints

Every file with `//go:build windows || darwin` needs to be updated. Two approaches:

**Approach A: Add `linux` to existing constraints**
```go
//go:build windows || darwin || linux
```
For shared code that should also compile on Linux.

**Approach B: Split into platform files**
Where Darwin and Windows behavior diverge from Linux, create `*_linux.go` files.

**Practical plan:**
1. Update shared files: `app.go`, `webview.go`, `server.go`, `ui/*.go`, `store/*.go` → add `|| linux`
2. Create new Linux-specific files: `app_linux.go`, `dlgs_linux.go`
3. Leave Darwin/Windows files unchanged

### 2.7 Updater (`app/updater/updater_linux.go`)

Linux update strategy depends on distribution method:

- **AppImage:** Self-contained, can self-update (download + replace)
- **Deb/RPM:** Updates via apt/dnf, app should just notify
- **Snap/Flatpak:** Updates via store, app should just notify
- **Manual/tarball:** Self-update like AppImage

**Initial approach:** Detect install method, show notification only (don't auto-update). The CLI `ollama` already handles updates on Linux.

### 2.8 Desktop Integration

- **`.desktop` file** for application launcher (`/usr/share/applications/ollama.desktop` or `~/.local/share/applications/`)
- **Icon assets** — SVG + multiple PNG sizes for `/usr/share/icons/hicolor/`
- **URL scheme handler** — Register `ollama://` protocol via `.desktop` file `MimeType` field
- **Autostart** — `.desktop` file in `~/.config/autostart/` with `X-GNOME-Autostart-enabled=true`

---

## 3. Dependency Summary

### Runtime
- GTK 3 (`libgtk-3-0`)
- WebKit2GTK (`libwebkit2gtk-4.1-0`)
- libayatana-appindicator3 (for systray, if using getlantern/systray)

### Build
- `libgtk-3-dev`
- `libwebkit2gtk-4.1-dev`
- `libayatana-appindicator3-dev`
- Go 1.22+ with CGO_ENABLED=1
- `pkg-config`

### Ubuntu install:
```bash
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev libayatana-appindicator3-dev pkg-config
```

---

## 4. Implementation Phases

### Phase 1: Minimal Viable Linux GUI
**Goal:** Webview window showing the React UI, with a system tray icon.

1. Re-add GTK backend to `app/webview/` from upstream webview library
2. Update build constraints on shared code (`app.go`, `ui/`, `store/`, `server/`)
3. Create `app/cmd/app/app_linux.go` with `osRun()` using getlantern/systray
4. Create `app/dialog/dlgs_linux.go` using zenity
5. Verify the React SPA loads and works in WebKit2GTK
6. Test on Ubuntu 22.04 and 24.04

**Deliverable:** `go build -tags linux ./app/cmd/app` produces a working binary.

### Phase 2: Desktop Integration
1. Create `.desktop` file and icon assets
2. Implement autostart registration
3. Implement `ollama://` URL scheme handling
4. Create build script (`scripts/build_linux.sh`)

### Phase 3: Packaging
1. AppImage packaging (self-contained, works everywhere)
2. Deb package for Ubuntu/Debian
3. Optional: Snap or Flatpak

### Phase 4: Polish
1. Update notification (detect install method, show appropriate message)
2. XDG compliance audit (config/data/cache directories)
3. Wayland compatibility testing (WebKit2GTK handles this, but verify)
4. Multi-monitor / HiDPI testing

---

## 5. Key Decisions Needed

| Decision | Options | Recommendation | Status |
|----------|---------|----------------|--------|
| Tray library | getlantern/systray vs gotk3 vs dbus | getlantern/systray | Pending |
| Dialog system | zenity vs gotk3 vs xdg-portal | zenity (phase 1), xdg-portal (later) | Pending |
| Webview approach | Re-add upstream GTK backend vs full update | Re-add GTK backend | Pending |
| GTK version | GTK 3 vs GTK 4 | GTK 3 (matches WebKit2GTK availability) | Pending |
| WebKit2GTK version | 4.0 vs 4.1 | 4.1 (Ubuntu 22.04+, better security) | Pending |
| Primary packaging | AppImage vs Deb vs Snap | AppImage (phase 1) + Deb (phase 3) | Pending |
| Minimum Ubuntu | 20.04 vs 22.04 | 22.04 LTS (webkit2gtk-4.1 available) | Pending |

---

## 6. Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| Upstream webview divergence | High — GTK backend may not drop in cleanly | Audit upstream, pin to compatible version |
| WebKit2GTK rendering differences | Medium — React SPA may render differently than Chrome/Safari | Early testing, CSS fixes |
| Tray icon inconsistency across DEs | Medium — GNOME, KDE, XFCE all handle trays differently | getlantern/systray handles this; test on major DEs |
| Wayland vs X11 differences | Low — WebKit2GTK + GTK3 handle both | Test under both |
| CGO cross-compilation | Medium — Need Linux build environment | Build on Linux, CI with Ubuntu runners |

---

## 7. Files to Create/Modify

### New Files
- `app/cmd/app/app_linux.go` — Linux platform entry point
- `app/dialog/dlgs_linux.go` — Linux dialog implementation
- `app/linuxtray/` (if not using getlantern/systray as a module)
- `app/updater/updater_linux.go` — Linux update stub
- `scripts/build_linux.sh` — Build script
- `app/assets/ollama.desktop` — Desktop entry
- `app/assets/ollama.svg` — Linux icon (SVG)

### Modified Files (build constraint updates)
- `app/cmd/app/app.go` — Add `|| linux` to build constraint
- `app/cmd/app/webview.go` — Add `|| linux`
- `app/webview/webview.go` — Add Linux CGO flags + `|| linux` build tag
- `app/webview/webview.h` — Restore GTK backend definitions
- `app/ui/ui.go` — Add `|| linux`
- `app/ui/app.go` — Add `|| linux`
- `app/store/*.go` — Add `|| linux` where constrained
- `app/server/server.go` — Add `|| linux`
- `app/dialog/dlgs.go` — Add `|| linux`
- `app/updater/updater.go` — Add `|| linux`
- `go.mod` / `go.sum` — Add getlantern/systray dependency

---

## 8. Open Questions

1. **Should this be a fork or upstream PR?** If targeting upstream ollama, need to match their code style and get buy-in. If local fork, more freedom but maintenance burden.
2. **WebKit2GTK version:** 4.0 API is available on older distros but 4.1 has better security defaults. Ubuntu 22.04 has both.
3. **Should the Linux app reuse the existing `wintray` menu structure?** Or create a Linux-native menu with different items (e.g., no "Check for Updates" if installed via apt)?
4. **Testing matrix:** Which Linux distros/DEs to officially support? Ubuntu (GNOME), Kubuntu (KDE), Fedora, Arch?
