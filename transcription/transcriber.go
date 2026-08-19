package transcription

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/rob137/risper/internal/files"
	"github.com/rob137/risper/models"
)

// ErrNoTranscript means that the configured local engine completed without
// printing or writing a transcript.
type ErrNoTranscript struct{}

func (ErrNoTranscript) Error() string { return "transcription command produced no transcript" }

// Transcribe runs the selected local engine and accepts either stdout or the
// raw/clean output files documented by the model profile format.
func Transcribe(profile models.Profile, audioPath, rawPath, cleanPath string, onProcessStart func(pid int) error) (string, error) {
	rendered := renderCommand(profile, audioPath, rawPath, cleanPath)
	cmd := exec.Command("sh", "-c", rendered)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	if onProcessStart != nil {
		if err := onProcessStart(cmd.Process.Pid); err != nil {
			_ = terminateProcessGroup(cmd.Process.Pid)
			_ = cmd.Wait()
			return "", err
		}
	}
	if err := cmd.Wait(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return "", fmt.Errorf("transcription command failed: %w: %s", err, message)
		}
		return "", fmt.Errorf("transcription command failed: %w", err)
	}

	if text := strings.TrimSpace(stdout.String()); text != "" {
		if err := writeTranscript(rawPath, cleanPath, text); err != nil {
			return "", err
		}
		return text, nil
	}
	if text, ok := readTranscript(rawPath); ok {
		if err := writeText(cleanPath, text); err != nil {
			return "", err
		}
		return text, nil
	}
	if text, ok := readTranscript(cleanPath); ok {
		if err := writeText(rawPath, text); err != nil {
			return "", err
		}
		return text, nil
	}
	return "", ErrNoTranscript{}
}

func renderCommand(profile models.Profile, audioPath, rawPath, cleanPath string) string {
	replacements := map[string]string{
		"{audio}":        audioPath,
		"{raw}":          rawPath,
		"{raw_no_txt}":   strings.TrimSuffix(rawPath, filepath.Ext(rawPath)),
		"{clean}":        cleanPath,
		"{clean_no_txt}": strings.TrimSuffix(cleanPath, filepath.Ext(cleanPath)),
		"{model}":        profile.Model,
		"{language}":     profile.Language,
		"{prompt}":       profile.Prompt,
	}
	rendered := profile.Command
	for placeholder, value := range replacements {
		rendered = strings.ReplaceAll(rendered, placeholder, value)
	}
	if strings.HasPrefix(rendered, "~/") && !strings.ContainsAny(rendered, " \t\n") {
		if home, err := os.UserHomeDir(); err == nil {
			rendered = filepath.Join(home, rendered[2:])
		}
	}
	return rendered
}

func readTranscript(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	text := strings.TrimSpace(string(data))
	return text, text != ""
}

func writeTranscript(rawPath, cleanPath, text string) error {
	if err := writeText(rawPath, text); err != nil {
		return err
	}
	return writeText(cleanPath, text)
}

func writeText(path, text string) error {
	return files.AtomicWriteText(path, strings.TrimSpace(text)+"\n")
}
