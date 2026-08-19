package retranscribe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/models"
	"github.com/rob137/risper/session"
)

func TestRetranscribeDefaultsToMicAndSupportsMixedAudio(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(bin, "whisper-cli"), `#!/bin/sh
printf '%s\n' "$2" >> "$WHISPER_LOG"
printf 'retranscribed transcript\n'
`)
	writeExecutable(t, filepath.Join(bin, "wl-copy"), `#!/bin/sh
cat > "$CLIPBOARD_PATH"
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	logPath := filepath.Join(root, "whisper.log")
	clipboardPath := filepath.Join(root, "clipboard.txt")
	t.Setenv("WHISPER_LOG", logPath)
	t.Setenv("CLIPBOARD_PATH", clipboardPath)

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := models.Write(cfg, models.Profile{
		ID: "stub", Engine: "stub", Model: "stub", Language: "en", Command: "whisper-cli -f {audio}",
	}, true); err != nil {
		t.Fatal(err)
	}

	first := makeSession(t, cfg, "first")
	first.Errors = []string{"stale error from an earlier attempt"}
	if err := session.SaveMetadata(first); err != nil {
		t.Fatal(err)
	}
	if code := Main([]string{first.SessionID}); code != 0 {
		t.Fatalf("mic retranscribe returned %d", code)
	}
	second := makeSession(t, cfg, "second")
	if code := Main([]string{"--mixed", second.SessionID}); code != 0 {
		t.Fatalf("mixed retranscribe returned %d", code)
	}

	log := readFile(t, logPath)
	if !strings.Contains(log, first.AudioSourcePaths["mic"]) {
		t.Fatalf("mic path not used: %q", log)
	}
	if strings.Contains(log, first.AudioPath) {
		t.Fatalf("mic retranscribe used mixed path: %q", log)
	}
	if !strings.Contains(log, second.AudioPath) {
		t.Fatalf("mixed path not used: %q", log)
	}
	if got := strings.TrimSpace(readFile(t, clipboardPath)); got != "retranscribed transcript" {
		t.Fatalf("clipboard = %q", got)
	}
	for _, metadata := range []*session.Metadata{first, second} {
		loaded, err := session.LoadSession(session.SessionDir(metadata))
		if err != nil || loaded == nil {
			t.Fatalf("load %s: %v", metadata.SessionID, err)
		}
		if loaded.Status != "complete" || loaded.PasteAttempted == nil || *loaded.PasteAttempted {
			t.Fatalf("metadata = %#v", loaded)
		}
		if len(loaded.Errors) != 0 {
			t.Fatalf("successful retranscription retained stale errors: %#v", loaded.Errors)
		}
	}
}

func TestRetranscribeRejectsRemovedCopyFlag(t *testing.T) {
	if code := Main([]string{"--copy", "last"}); code != 2 {
		t.Fatalf("--copy returned %d, want usage error 2", code)
	}
}

func makeSession(t *testing.T, cfg config.Config, name string) *session.Metadata {
	t.Helper()
	metadata, err := session.CreateAt(cfg, timeForTest(name))
	if err != nil {
		t.Fatal(err)
	}
	mic := filepath.Join(session.SessionDir(metadata), "audio.mic.wav")
	if err := os.WriteFile(mic, []byte("RIFF mic audio padding"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadata.AudioPath, []byte("RIFF mixed audio padding"), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata.AudioSources = []string{"mic", "system"}
	metadata.AudioSourcePaths = map[string]string{"mic": mic, "system": filepath.Join(session.SessionDir(metadata), "audio.system.wav")}
	if err := session.SaveMetadata(metadata); err != nil {
		t.Fatal(err)
	}
	return metadata
}

func timeForTest(name string) time.Time {
	if name == "first" {
		return time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	}
	return time.Date(2026, 8, 19, 12, 1, 0, 0, time.UTC)
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
