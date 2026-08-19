package transcription

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob137/risper/models"
)

func TestTranscribeReadsRawFileAndWritesCleanFile(t *testing.T) {
	root := t.TempDir()
	rawPath := filepath.Join(root, "transcript.raw.txt")
	cleanPath := filepath.Join(root, "transcript.clean.txt")
	if err := os.WriteFile(rawPath, []byte("\n  transcript written by whisper.cpp\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	transcript, err := Transcribe(models.Profile{Command: "printf ''"}, "audio.wav", rawPath, cleanPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if transcript != "transcript written by whisper.cpp" {
		t.Fatalf("transcript = %q", transcript)
	}
	if got, err := os.ReadFile(cleanPath); err != nil || string(got) != "transcript written by whisper.cpp\n" {
		t.Fatalf("clean transcript = %q, %v", got, err)
	}
}

func TestTranscribeReadsCleanFileAndWritesRawFile(t *testing.T) {
	root := t.TempDir()
	rawPath := filepath.Join(root, "transcript.raw.txt")
	cleanPath := filepath.Join(root, "transcript.clean.txt")
	if err := os.WriteFile(cleanPath, []byte("clean transcript\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	transcript, err := Transcribe(models.Profile{Command: "printf ''"}, "audio.wav", rawPath, cleanPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if transcript != "clean transcript" {
		t.Fatalf("transcript = %q", transcript)
	}
	if got, err := os.ReadFile(rawPath); err != nil || string(got) != "clean transcript\n" {
		t.Fatalf("raw transcript = %q, %v", got, err)
	}
}

func TestTranscribePreservesQuotedPromptAsOneArgument(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	argumentsPath := filepath.Join(root, "arguments.log")
	engine := `#!/bin/sh
for arg in "$@"; do
  printf '<%s>\n' "$arg" >> "$ARGUMENTS_LOG"
done
printf 'prompt test transcript\n'
`
	enginePath := filepath.Join(bin, "stub-engine")
	if err := os.WriteFile(enginePath, []byte(engine), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ARGUMENTS_LOG", argumentsPath)

	prompt := "Names and terms: Abdullah, coEngen, Singular Machines, Culham, Adrian, James, Will, Flic, Claude, Claude Code, Codex, ChatGPT, Emacs, Temporal, divertor. Plainer, shorter."
	transcript, err := Transcribe(models.Profile{
		Command: "stub-engine --prompt \"{prompt}\" -f {audio}",
		Prompt:  prompt,
	}, "audio.wav", filepath.Join(root, "raw.txt"), filepath.Join(root, "clean.txt"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if transcript != "prompt test transcript" {
		t.Fatalf("transcript = %q", transcript)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(arguments), "<"+prompt+">\n") {
		t.Fatalf("prompt was split or changed: %q", arguments)
	}
}

func TestTranscribeAddsWhisperContextGuardWhenProfileOmitsIt(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	argumentsPath := filepath.Join(root, "arguments.log")
	engine := `#!/bin/sh
for arg in "$@"; do
  printf '<%s>\n' "$arg" >> "$ARGUMENTS_LOG"
done
printf 'guarded transcript\n'
`
	if err := os.WriteFile(filepath.Join(bin, "whisper-cli"), []byte(engine), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ARGUMENTS_LOG", argumentsPath)

	transcript, err := Transcribe(models.Profile{
		Engine:  "whisper.cpp",
		Command: "whisper-cli -f {audio}",
	}, "audio.wav", filepath.Join(root, "raw.txt"), filepath.Join(root, "clean.txt"), nil)
	if err != nil || transcript != "guarded transcript" {
		t.Fatalf("transcription = %q, %v", transcript, err)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(arguments), "<-mc>\n<0>\n") {
		t.Fatalf("whisper context guard missing: %q", arguments)
	}
}

func TestTranscribeReturnsEngineError(t *testing.T) {
	root := t.TempDir()
	rawPath := filepath.Join(root, "raw.txt")
	cleanPath := filepath.Join(root, "clean.txt")
	_, err := Transcribe(models.Profile{
		Command: "printf 'engine exploded\\n' >&2; exit 7",
	}, "audio.wav", rawPath, cleanPath, nil)
	if err == nil || !strings.Contains(err.Error(), "engine exploded") {
		t.Fatalf("error = %v", err)
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 7 {
		t.Fatalf("error = %v, want exit code 7", err)
	}
	if _, statErr := os.Stat(rawPath); !os.IsNotExist(statErr) {
		t.Fatalf("raw transcript unexpectedly exists: %v", statErr)
	}
}

func TestTranscribeReturnsNoTranscriptError(t *testing.T) {
	root := t.TempDir()
	_, err := Transcribe(models.Profile{Command: "printf ''"}, "audio.wav", filepath.Join(root, "raw.txt"), filepath.Join(root, "clean.txt"), nil)
	var noTranscript ErrNoTranscript
	if !errors.As(err, &noTranscript) {
		t.Fatalf("error = %v, want ErrNoTranscript", err)
	}
}

func TestReadTranscriptTrimsTextAndRejectsEmptyFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "transcript.txt")
	if text, ok := readTranscript(path); ok || text != "" {
		t.Fatalf("missing transcript = %q, %v", text, ok)
	}
	if err := os.WriteFile(path, []byte(" \n\t"), 0o644); err != nil {
		t.Fatal(err)
	}
	if text, ok := readTranscript(path); ok || text != "" {
		t.Fatalf("empty transcript = %q, %v", text, ok)
	}
	if err := os.WriteFile(path, []byte("\n  actual transcript  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if text, ok := readTranscript(path); !ok || text != "actual transcript" {
		t.Fatalf("read transcript = %q, %v", text, ok)
	}
}
