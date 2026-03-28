//go:build linux

package updater

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
)

var BundlePath = ""

func init() {
	UserAgentOS = "Linux"

	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err == nil {
		var buf []byte
		for _, b := range uname.Release {
			if b == 0 {
				break
			}
			buf = append(buf, byte(b))
		}
		UserAgentOS = fmt.Sprintf("Linux/%s", string(buf))
	}

	appDataDir := filepath.Join(os.Getenv("HOME"), ".ollama")
	UpdateStageDir = filepath.Join(appDataDir, "updates")
	UpgradeLogFile = filepath.Join(appDataDir, "logs", "upgrade.log")
	UpgradeMarkerFile = filepath.Join(appDataDir, "upgraded")
}

func DoUpgrade(_ bool) error {
	// On Linux, updates are handled by the package manager or install script.
	slog.Info("upgrade not supported via GUI on Linux, use 'curl -fsSL https://ollama.com/install.sh | sh'")
	return nil
}

func DoPostUpgradeCleanup() error {
	if UpgradeMarkerFile != "" {
		return os.Remove(UpgradeMarkerFile)
	}
	return nil
}

func DoUpgradeAtStartup() error {
	return nil
}

func IsUpdatePending() bool {
	return false
}

func getStagedUpdate() string {
	return ""
}

func verifyDownload() error {
	return nil
}
