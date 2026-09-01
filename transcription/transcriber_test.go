package transcription

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestTranscribeStdinRecognizesWithoutCreatingAudioFiles(t *testing.T) {
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
cat >/dev/null
printf '%s' quasar
`
	if err := os.WriteFile(filepath.Join(bin, "whisper-cli"), []byte(engine), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ARGUMENTS_LOG", argumentsPath)

	transcript, err := TranscribeStdin(models.Profile{
		ID:      "whispercpp-base-en",
		Engine:  "whisper.cpp",
		Command: "whisper-cli -f {audio} -mc 0",
	}, []byte("RIFF in memory"))
	if err != nil || transcript != "quasar" {
		t.Fatalf("transcript = %q, err = %v", transcript, err)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(arguments), "<->\n") {
		t.Fatalf("stdin input marker missing: %q", arguments)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".wav") {
			t.Fatalf("voice recognition created an audio file %s", entry.Name())
		}
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

func TestTranscribeWithFallbackUsesLocalAfterFastCloudTimeout(t *testing.T) {
	keyPath := openAITestKey(t, "test-key", 0o600)
	root := t.TempDir()
	audioPath := filepath.Join(root, "clip.wav")
	rawPath := filepath.Join(root, "raw.txt")
	cleanPath := filepath.Join(root, "clean.txt")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	useOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	})
	local := models.Profile{ID: "local-base", Engine: "whisper.cpp", Command: "printf 'local transcript'"}
	started := time.Now()
	result, err := TranscribeWithFallback(
		models.Profile{ID: "cloud", Engine: "openai", Model: "gpt-transcribe", APIKeyFile: keyPath},
		local, audioPath, rawPath, cleanPath, 30*time.Millisecond, nil,
	)
	if err != nil {
		t.Fatalf("fallback transcription error = %v", err)
	}
	if result.Transcript != "local transcript" || result.Profile.ID != local.ID {
		t.Fatalf("result = %#v, want local profile and transcript", result)
	}
	if result.PrimaryError == nil || !errors.Is(result.PrimaryError, context.DeadlineExceeded) {
		t.Fatalf("primary error = %v, want deadline exceeded", result.PrimaryError)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("bounded fallback took %s", elapsed)
	}
}

func TestTranscribeWithFallbackUsesLocalAfterCloudHTTPError(t *testing.T) {
	keyPath := openAITestKey(t, "test-key", 0o600)
	root := t.TempDir()
	audioPath := filepath.Join(root, "clip.wav")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	useOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"bad key"}}`)
	})
	local := models.Profile{ID: "local-base", Engine: "whisper.cpp", Command: "printf 'local after auth error'"}
	result, err := TranscribeWithFallback(
		models.Profile{ID: "cloud", Engine: "openai", Model: "gpt-transcribe", APIKeyFile: keyPath},
		local, audioPath, filepath.Join(root, "raw.txt"), filepath.Join(root, "clean.txt"), time.Second, nil,
	)
	if err != nil || result.Transcript != "local after auth error" {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if result.PrimaryError == nil || !strings.Contains(result.PrimaryError.Error(), "HTTP status 401") {
		t.Fatalf("primary error = %v, want HTTP 401", result.PrimaryError)
	}
}

func TestTranscribeWithFallbackDoesNotUseLocalForInputOpenFailure(t *testing.T) {
	keyPath := openAITestKey(t, "test-key", 0o600)
	root := t.TempDir()
	marker := filepath.Join(root, "fallback-ran")
	useOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("OpenAI endpoint was reached despite missing input audio")
	})
	local := models.Profile{ID: "local-base", Engine: "whisper.cpp", Command: fmt.Sprintf("printf ran > %s; printf local", shellQuote(marker))}
	_, err := TranscribeWithFallback(
		models.Profile{ID: "cloud", Engine: "openai", APIKeyFile: keyPath},
		local, filepath.Join(root, "missing.wav"), filepath.Join(root, "raw.txt"), filepath.Join(root, "clean.txt"), time.Second, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "open audio for transcription fallback") {
		t.Fatalf("error = %v, want input-open error", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("local fallback ran: %v", statErr)
	}
}

func TestTranscribeWithFallbackDoesNotUseLocalForTranscriptWriteFailure(t *testing.T) {
	keyPath := openAITestKey(t, "test-key", 0o600)
	root := t.TempDir()
	audioPath := filepath.Join(root, "clip.wav")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "fallback-ran")
	useOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"text":"cloud transcript"}`)
	})
	blockedParent := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blockedParent, []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}
	local := models.Profile{ID: "local-base", Engine: "whisper.cpp", Command: fmt.Sprintf("printf ran > %s; printf local", shellQuote(marker))}
	_, err := TranscribeWithFallback(
		models.Profile{ID: "cloud", Engine: "openai", APIKeyFile: keyPath},
		local, audioPath, filepath.Join(blockedParent, "raw.txt"), filepath.Join(root, "clean.txt"), time.Second, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "not-a-directory") {
		t.Fatalf("error = %v, want transcript storage error", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("local fallback ran: %v", statErr)
	}
}

func TestTranscribeWithFallbackReportsBothFailures(t *testing.T) {
	keyPath := openAITestKey(t, "test-key", 0o600)
	root := t.TempDir()
	audioPath := filepath.Join(root, "clip.wav")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	useOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"message":"service down"}}`)
	})
	local := models.Profile{ID: "local-base", Engine: "whisper.cpp", Command: "printf 'local failed' >&2; exit 9"}
	_, err := TranscribeWithFallback(
		models.Profile{ID: "cloud", Engine: "openai", APIKeyFile: keyPath},
		local, audioPath, filepath.Join(root, "raw.txt"), filepath.Join(root, "clean.txt"), time.Second, nil,
	)
	var fallbackErr *TranscriptionFallbackError
	if !errors.As(err, &fallbackErr) {
		t.Fatalf("error = %v, want TranscriptionFallbackError", err)
	}
	if !strings.Contains(fallbackErr.Primary.Error(), "HTTP status 503") || !strings.Contains(fallbackErr.Fallback.Error(), "local failed") {
		t.Fatalf("fallback error = %#v", fallbackErr)
	}
}

func TestTranscribeWithFallbackSuccessDoesNotStartLocal(t *testing.T) {
	keyPath := openAITestKey(t, "test-key", 0o600)
	root := t.TempDir()
	audioPath := filepath.Join(root, "clip.wav")
	marker := filepath.Join(root, "fallback-ran")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	useOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"text":"cloud transcript"}`)
	})
	local := models.Profile{ID: "local-base", Engine: "whisper.cpp", Command: fmt.Sprintf("printf ran > %s; printf local", shellQuote(marker))}
	result, err := TranscribeWithFallback(
		models.Profile{ID: "cloud", Engine: "openai", APIKeyFile: keyPath},
		local, audioPath, filepath.Join(root, "raw.txt"), filepath.Join(root, "clean.txt"), time.Second, nil,
	)
	if err != nil || result.Profile.ID != "cloud" || result.PrimaryError != nil || result.Transcript != "cloud transcript" {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("local fallback ran after cloud success: %v", statErr)
	}
}

func TestTranscribeWithFallbackPreservesLocalProcessStartCancellation(t *testing.T) {
	keyPath := openAITestKey(t, "test-key", 0o600)
	root := t.TempDir()
	audioPath := filepath.Join(root, "clip.wav")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	useOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	local := models.Profile{ID: "local-base", Engine: "whisper.cpp", Command: "sleep 10"}
	callbackErr := errors.New("controller cancelled local worker")
	called := false
	_, err := TranscribeWithFallback(
		models.Profile{ID: "cloud", Engine: "openai", APIKeyFile: keyPath},
		local, audioPath, filepath.Join(root, "raw.txt"), filepath.Join(root, "clean.txt"), time.Second,
		func(pid int) error {
			called = pid > 0
			return callbackErr
		},
	)
	if !called {
		t.Fatal("fallback did not invoke onProcessStart")
	}
	if !errors.Is(err, callbackErr) {
		t.Fatalf("error = %v, want callback cancellation", err)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
