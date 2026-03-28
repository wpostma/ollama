//go:build linux

package dialog

import (
	"os/exec"
	"strings"
)

func (b *MsgBuilder) yesNo() bool {
	args := []string{"--question", "--text", b.Msg}
	if b.Dlg.Title != "" {
		args = append(args, "--title", b.Dlg.Title)
	}
	cmd := exec.Command("zenity", args...)
	err := cmd.Run()
	return err == nil
}

func (b *MsgBuilder) info() {
	args := []string{"--info", "--text", b.Msg}
	if b.Dlg.Title != "" {
		args = append(args, "--title", b.Dlg.Title)
	}
	cmd := exec.Command("zenity", args...)
	cmd.Run()
}

func (b *MsgBuilder) error() {
	args := []string{"--error", "--text", b.Msg}
	if b.Dlg.Title != "" {
		args = append(args, "--title", b.Dlg.Title)
	}
	cmd := exec.Command("zenity", args...)
	cmd.Run()
}

func (b *FileBuilder) load() (string, error) {
	args := []string{"--file-selection"}
	if b.Dlg.Title != "" {
		args = append(args, "--title", b.Dlg.Title)
	}
	if b.StartDir != "" {
		args = append(args, "--filename", b.StartDir+"/")
	}
	args = appendFileFilters(args, b.Filters)
	cmd := exec.Command("zenity", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", ErrCancelled
	}
	return strings.TrimSpace(string(output)), nil
}

func (b *FileBuilder) loadMultiple() ([]string, error) {
	args := []string{"--file-selection", "--multiple", "--separator", "\n"}
	if b.Dlg.Title != "" {
		args = append(args, "--title", b.Dlg.Title)
	}
	if b.StartDir != "" {
		args = append(args, "--filename", b.StartDir+"/")
	}
	args = appendFileFilters(args, b.Filters)
	cmd := exec.Command("zenity", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, ErrCancelled
	}
	result := strings.TrimSpace(string(output))
	if result == "" {
		return nil, ErrCancelled
	}
	return strings.Split(result, "\n"), nil
}

func (b *FileBuilder) save() (string, error) {
	args := []string{"--file-selection", "--save", "--confirm-overwrite"}
	if b.Dlg.Title != "" {
		args = append(args, "--title", b.Dlg.Title)
	}
	if b.StartDir != "" {
		args = append(args, "--filename", b.StartDir+"/")
	}
	if b.StartFile != "" {
		args = append(args, "--filename", b.StartFile)
	}
	args = appendFileFilters(args, b.Filters)
	cmd := exec.Command("zenity", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", ErrCancelled
	}
	return strings.TrimSpace(string(output)), nil
}

func (b *DirectoryBuilder) browse() (string, error) {
	args := []string{"--file-selection", "--directory"}
	if b.Dlg.Title != "" {
		args = append(args, "--title", b.Dlg.Title)
	}
	if b.StartDir != "" {
		args = append(args, "--filename", b.StartDir+"/")
	}
	cmd := exec.Command("zenity", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", ErrCancelled
	}
	return strings.TrimSpace(string(output)), nil
}

func appendFileFilters(args []string, filters []FileFilter) []string {
	for _, f := range filters {
		var patterns []string
		for _, ext := range f.Extensions {
			if ext == "*" {
				patterns = append(patterns, "*")
			} else {
				patterns = append(patterns, "*."+ext)
			}
		}
		filter := f.Desc + " | " + strings.Join(patterns, " ")
		args = append(args, "--file-filter", filter)
	}
	return args
}
