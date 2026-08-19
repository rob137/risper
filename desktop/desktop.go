// Package desktop contains the small Linux command adapters used by the Go
// toggle. They are deliberately best-effort: a notification or sound must
// never turn a saved transcript into a failed session.
package desktop

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/internal/files"
)

func CopyText(text string) (bool, string) {
	candidates := []string{"wl-copy", "xclip", "xsel"}
	sessionType := strings.ToLower(os.Getenv("XDG_SESSION_TYPE"))
	if sessionType == "" {
		sessionType = "wayland"
	}
	if sessionType != "wayland" {
		candidates = []string{"xclip", "xsel", "wl-copy"}
	}
	for _, name := range candidates {
		if !config.CommandExists(name) {
			continue
		}
		var args []string
		switch name {
		case "xclip":
			args = []string{"-selection", "clipboard"}
		case "xsel":
			args = []string{"--clipboard", "--input"}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Stdin = strings.NewReader(text)
		err := cmd.Run()
		cancel()
		if err == nil {
			return true, "copied with " + name
		}
	}
	return false, "no clipboard command available"
}

// OpenPath asks the desktop to open a file or directory with its default
// handler. It is intentionally asynchronous, matching the Python command
// surface and keeping a missing GUI application from blocking the CLI.
func OpenPath(path string) (bool, string) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, "not found: " + path
		}
		return false, err.Error()
	}
	if !config.CommandExists("gio") {
		return false, "gio is not installed"
	}
	if err := exec.Command("gio", "open", path).Start(); err != nil {
		return false, "could not open " + path + ": " + err.Error()
	}
	return true, "opened " + path
}

// TrashPath uses the desktop trash where available. Callers may choose a
// more explicit fallback only after asking the user for confirmation.
func TrashPath(path string) (bool, string) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, "not found: " + path
		}
		return false, err.Error()
	}
	if !config.CommandExists("gio") {
		return false, "gio is not installed"
	}
	if err := exec.Command("gio", "trash", path).Run(); err != nil {
		return false, "could not move to trash: " + err.Error()
	}
	return true, "moved to trash: " + path
}

func DiagnosticCommands() []string {
	return []string{
		"pw-record", "arecord", "ffmpeg", "wl-copy", "wtype", "xclip", "xsel",
		"xdotool", "ydotool", "dotool", "notify-send", "paplay",
		"canberra-gtk-play", "gio", "gtk-launch", "python3", "go",
	}
}

func Notify(cfg config.Config, title, body string) {
	if title == "" || !config.CommandExists("notify-send") {
		return
	}
	args := []string{"--app-name=Risper", "--print-id"}
	idPath := ""
	if cfg.StateDir != "" {
		idPath = filepath.Join(cfg.StateDir, "notification-id")
	}
	if idPath != "" {
		if data, err := os.ReadFile(idPath); err == nil && strings.TrimSpace(string(data)) != "" {
			id := strings.TrimSpace(string(data))
			if isDigits(id) {
				args = append(args, "--replace-id="+id)
			}
		}
	}
	args = append(args, title, body)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "notify-send", args...)
	output, err := cmd.Output()
	if err != nil {
		fallback := exec.Command("notify-send", "--app-name=Risper", title, body)
		fallback.Stdout = io.Discard
		fallback.Stderr = io.Discard
		_ = fallback.Start()
		return
	}
	id := strings.TrimSpace(string(output))
	if idPath != "" && isDigits(id) {
		_ = files.AtomicWriteText(idPath, id+"\n")
	}
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func Play(cfg config.Config, kind string) {
	if !cfg.PlaySounds || !config.CommandExists("canberra-gtk-play") {
		return
	}
	event, volume := sound(kind)
	cmd := exec.Command("canberra-gtk-play", "-i", event, "-V", volume, "-d", "Risper "+kind)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	_ = cmd.Start()
}

func sound(kind string) (string, string) {
	switch kind {
	case "recording_start", "start":
		return "message-new-instant", "-18"
	case "transcription_start":
		return "service-login", "-18"
	case "transcription_progress":
		return "bell", "-6"
	case "success", "stop":
		return "complete", "-18"
	case "cancel":
		return "service-logout", "-18"
	case "error":
		return "dialog-error", "-18"
	default:
		return "message", "-18"
	}
}
