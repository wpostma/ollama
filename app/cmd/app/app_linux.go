//go:build linux

package main

/*
#cgo linux pkg-config: gtk+-3.0
#include <gtk/gtk.h>

static gboolean show_window_idle(gpointer data) {
    GtkWidget *w = (GtkWidget *)data;
    gtk_widget_show_all(w);
    gtk_window_present(GTK_WINDOW(w));
    return G_SOURCE_REMOVE;
}

static void show_window_on_idle(GtkWidget *w) {
    g_idle_add(show_window_idle, w);
}
*/
import "C"

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/ollama/ollama/app/updater"
	"github.com/ollama/ollama/app/version"
)

var (
	ollamaPath string
	appLogPath = filepath.Join(os.Getenv("HOME"), ".ollama", "logs", "app.log")
)

func init() {
	exe, err := os.Executable()
	if err != nil {
		slog.Warn("error discovering executable directory", "error", err)
	} else {
		ollamaPath = filepath.Join(filepath.Dir(exe), "ollama")
	}

	if _, err := os.Stat(ollamaPath); err != nil {
		// Try to find ollama in PATH
		if p, err := exec.LookPath("ollama"); err == nil {
			ollamaPath = p
		}
	}
}

func maybeMoveAndRestart() appMove {
	return CannotMove
}

func handleExistingInstance(_ bool) {
	// On Linux, kill other app instances via PID file
	pidFile := filepath.Join(os.Getenv("HOME"), ".ollama", "ollama-app.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return
	}
	if pid == os.Getpid() {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	// Check if process is alive
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return
	}
	slog.Info("killing existing app instance", "pid", pid)
	proc.Signal(syscall.SIGTERM)
}

func installSymlink() {
	// On Linux, ollama is typically installed via package manager
	// or the install script. No symlink needed from the GUI app.
}

func UpdateAvailable(_ string) error {
	slog.Debug("update available notification (Linux)")
	// TODO: show desktop notification via libnotify
	return nil
}

func osRun(_ func(), hasCompletedFirstRun, startHidden bool) {
	// Write our PID file for instance detection
	pidFile := filepath.Join(os.Getenv("HOME"), ".ollama", "ollama-app.pid")
	os.MkdirAll(filepath.Dir(pidFile), 0o755)
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
	defer os.Remove(pidFile)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signals
		slog.Debug("shutting down due to signal")
		wv.Terminate()
	}()

	if startHidden {
		startHiddenTasks()
		// Block until signal when running hidden
		select {}
	} else {
		// wv.Run("/") creates the webview and sets up all bindings,
		// but on Linux we don't start the event loop in a goroutine.
		// Instead, we run gtk_main() here on the main thread (which
		// is locked to the OS thread via runtime.LockOSThread in
		// webview.init()). gtk_main_quit() is called when the webview
		// is terminated.
		wv.Run("/")
		slog.Debug("starting GTK main loop on main thread")
		C.gtk_main()
		slog.Debug("GTK main loop exited")
	}
}

func quit() {
	wv.Terminate()
}

func LaunchNewApp() {
	// On Linux, updates are handled by the package manager
}

func logStartup() {
	slog.Info("starting Ollama", "version", version.Version, "OS", updater.UserAgentOS)
}

func hideWindow(_ unsafe.Pointer) {
	// On Linux, the webview's GTK constructor already calls
	// gtk_widget_show_all. Hiding the window before the main
	// loop starts prevents it from ever appearing. Skip hide.
}

func showWindow(ptr unsafe.Pointer) {
	if ptr == nil {
		return
	}
	widget := (*C.GtkWidget)(ptr)
	C.gtk_widget_show_all(widget)
	C.gtk_window_present((*C.GtkWindow)(unsafe.Pointer(widget)))
}

func styleWindow(_ unsafe.Pointer) {
	// No special styling needed on Linux
}

func runInBackground() {
	exe, err := os.Executable()
	if err != nil {
		slog.Error("failed to get executable path", "error", err)
		os.Exit(1)
	}
	cmd := exec.Command(exe, "hidden")
	if err := cmd.Start(); err != nil {
		slog.Error("failed to run Ollama in background", "error", err)
		os.Exit(1)
	}
}

func drag(_ unsafe.Pointer) {}

func doubleClick(_ unsafe.Pointer) {}

func checkAndHandleExistingInstance(_ string) bool {
	return false
}
