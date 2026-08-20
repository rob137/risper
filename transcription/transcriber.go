package transcription

import (
	"bytes"
	"context"
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

// TranscribeStdin runs a whisper.cpp profile against an in-memory WAV. The
// profile's input and text output are both redirected through standard
// streams, so the caller can recognize short-lived trigger audio without
// creating a file outside a durable recording session.
func TranscribeStdin(profile models.Profile, wav []byte) (string, error) {
	return TranscribeStdinContext(context.Background(), profile, wav)
}

func TranscribeStdinContext(ctx context.Context, profile models.Profile, wav []byte) (string, error) {
	if profile.Engine != "whisper.cpp" {
		return "", fmt.Errorf("voice trigger profile %q is not a whisper.cpp profile", profile.ID)
	}
	rendered := RenderCommand(profile, "-", "-", "-")
	cmd := exec.CommandContext(ctx, "sh", "-c", rendered)
	cmd.Stdin = bytes.NewReader(wav)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	var runErr error
	select {
	case runErr = <-wait:
	case <-ctx.Done():
		_ = terminateProcessGroup(cmd.Process.Pid)
		<-wait
		return "", ctx.Err()
	}
	if runErr != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return "", fmt.Errorf("voice transcription command failed: %w: %s", runErr, message)
		}
		return "", fmt.Errorf("voice transcription command failed: %w", runErr)
	}
	text := strings.TrimSpace(stdout.String())
	if text == "" {
		return "", ErrNoTranscript{}
	}
	return text, nil
}

// RenderCommand expands the profile placeholders for commands that need to
// run the same backend without writing a session transcript, such as the
// benchmark command.
func RenderCommand(profile models.Profile, audioPath, rawPath, cleanPath string) string {
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
	if profile.Engine == "whisper.cpp" && !hasWhisperMaxContext(rendered) {
		// whisper.cpp's default carries all prior decoded text into later
		// segments. On a quiet mic passage that can suppress real speech; a
		// zero max-context keeps each segment acoustically grounded.
		rendered = strings.TrimSpace(rendered) + " -mc 0"
	}
	if strings.HasPrefix(rendered, "~/") && !strings.ContainsAny(rendered, " \t\n") {
		if home, err := os.UserHomeDir(); err == nil {
			rendered = filepath.Join(home, rendered[2:])
		}
	}
	return rendered
}

func hasWhisperMaxContext(command string) bool {
	for _, field := range strings.Fields(command) {
		if field == "-mc" || field == "--max-context" || strings.HasPrefix(field, "-mc=") || strings.HasPrefix(field, "--max-context=") {
			return true
		}
	}
	return false
}

func renderCommand(profile models.Profile, audioPath, rawPath, cleanPath string) string {
	return RenderCommand(profile, audioPath, rawPath, cleanPath)
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

// WriteTranscript persists a post-processing result, such as removing the
// spoken stop word before it reaches the clipboard.
func WriteTranscript(rawPath, cleanPath, text string) error {
	return writeTranscript(rawPath, cleanPath, text)
}

func writeText(path, text string) error {
	return files.AtomicWriteText(path, strings.TrimSpace(text)+"\n")
}
