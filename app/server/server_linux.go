//go:build linux

package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

var (
	pidFile       = filepath.Join(os.Getenv("HOME"), ".ollama", "ollama.pid")
	serverLogPath = filepath.Join(os.Getenv("HOME"), ".ollama", "logs", "server.log")
)

func commandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...)
}

func terminate(proc *os.Process) error {
	return proc.Signal(os.Interrupt)
}

func terminated(pid int) (bool, error) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, fmt.Errorf("failed to find process: %v", err)
	}

	err = proc.Signal(syscall.Signal(0))
	if err != nil {
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			return true, nil
		}

		return false, fmt.Errorf("error signaling process: %v", err)
	}

	return false, nil
}

// isSystemServiceRunning checks if ollama is already serving on the default port
func isSystemServiceRunning() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:11434/api/version")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// useExistingServer checks if an ollama system service is already running.
// If so, the GUI uses it instead of starting a child process. Blocks until
// the context is cancelled.
func useExistingServer(ctx context.Context) bool {
	if !isSystemServiceRunning() {
		return false
	}

	slog.Info("using existing ollama system service on :11434")

	// Monitor the system service — just wait for context cancellation
	<-ctx.Done()
	return true
}

// openServerLog opens the server log for reading. On Linux, if the local log
// file is missing or empty (e.g. when using the systemd service), it falls
// back to reading the journal.
func openServerLog() (io.ReadCloser, error) {
	if info, err := os.Stat(serverLogPath); err == nil && info.Size() > 0 {
		return os.Open(serverLogPath)
	}

	// Fall back to journalctl for the system service.
	// Only read from current boot to keep output small and fast.
	cmd := exec.Command("journalctl", "-u", "ollama", "--no-pager", "-o", "cat", "-b")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to read journalctl: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start journalctl: %w", err)
	}
	return &journalReader{cmd: cmd, ReadCloser: stdout}, nil
}

// journalReader wraps journalctl stdout and waits for the process on Close.
type journalReader struct {
	cmd *exec.Cmd
	io.ReadCloser
}

func (j *journalReader) Close() error {
	err := j.ReadCloser.Close()
	j.cmd.Wait()
	return err
}

// reapServers on Linux does NOT kill the system service.
func reapServers() error {
	if isSystemServiceRunning() {
		slog.Info("ollama system service is running, GUI will use it")
		return nil
	}
	slog.Debug("no existing ollama system service detected")
	return nil
}
