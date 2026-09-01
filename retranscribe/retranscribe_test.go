package retranscribe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/events"
	"github.com/rob137/risper/models"
	"github.com/rob137/risper/session"
)

func TestRetranscribeUsesMixedAudioByDefault(t *testing.T) {
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
		t.Fatalf("first retranscribe returned %d", code)
	}
	second := makeSession(t, cfg, "second")
	if code := Main([]string{second.SessionID}); code != 0 {
		t.Fatalf("second retranscribe returned %d", code)
	}

	log := readFile(t, logPath)
	if !strings.Contains(log, first.AudioPath) || !strings.Contains(log, second.AudioPath) || strings.Contains(log, first.AudioSourcePaths["mic"]) || strings.Contains(log, second.AudioSourcePaths["mic"]) {
		t.Fatalf("default retranscribe paths = %q", log)
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

func TestRetranscribeFallsBackToLocalWhisper(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(bin, "whisper-cli"), "#!/bin/sh\nprintf 'local fallback transcript\\n'\n")
	writeExecutable(t, filepath.Join(bin, "wl-copy"), "#!/bin/sh\ncat >/dev/null\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateConfigValueAt(cfg.ConfigPath, "selected_model", "cloud"); err != nil {
		t.Fatal(err)
	}
	registry := `[models.cloud]
engine = "openai"
model = "gpt-transcribe"
api_key_file = "` + filepath.Join(root, "missing-key") + `"
fallback_profile = "whispercpp-small-en"

[models.whispercpp-small-en]
engine = "whisper.cpp"
model = "small.en"
language = "en"
command = "whisper-cli -f {audio}"
`
	if err := os.WriteFile(cfg.ModelsPath, []byte(registry), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := makeSession(t, cfg, "fallback")
	if code := Main([]string{metadata.SessionID}); code != 0 {
		t.Fatalf("fallback retranscribe returned %d", code)
	}
	loaded, err := session.LoadSession(session.SessionDir(metadata))
	if err != nil || loaded == nil {
		t.Fatalf("session = %#v, %v", loaded, err)
	}
	if loaded.Status != "complete" || loaded.TranscriptionEngine != "whisper.cpp" || loaded.Model != "small.en" {
		t.Fatalf("fallback metadata = %#v", loaded)
	}
	records, err := events.Read(session.SessionDir(metadata), 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range records {
		if record["event"] == "retranscription.fallback" && record["to_profile"] == "whispercpp-small-en" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fallback event missing: %#v", records)
	}
}

func TestRetranscribeRejectsRemovedCopyFlag(t *testing.T) {
	if code := Main([]string{"--copy", "last"}); code != 2 {
		t.Fatalf("--copy returned %d, want usage error 2", code)
	}
}

func TestRetranscribeRejectsRemovedAudioSourceFlags(t *testing.T) {
	for _, flag := range []string{"--mixed", "--system"} {
		if code := Main([]string{flag, "last"}); code != 2 {
			t.Fatalf("%s returned %d, want usage error 2", flag, code)
		}
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

// Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
