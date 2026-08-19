package toggle

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/events"
	"github.com/rob137/risper/session"
)

func TestFailLogsAndReturnsFailure(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		StateDir: root,
		LogPath:  filepath.Join(root, "risper.log"),
	}
	t.Setenv("PATH", "")

	if code := fail(cfg, errors.New("test failure")); code != 1 {
		t.Fatalf("code = %d", code)
	}
	data, err := os.ReadFile(cfg.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "test failure") {
		t.Fatalf("log = %q", data)
	}
}

func TestTranscriptionFailureLeavesSessionForRecovery(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		SessionsDir: filepath.Join(root, "sessions"),
		StateDir:    filepath.Join(root, "state"),
		LogPath:     filepath.Join(root, "state", "risper.log"),
		Language:    "en",
		Model:       "base.en",
		PasteMode:   "clipboard_only",
		PlaySounds:  false,
	}
	metadata, err := session.Create(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadata.AudioPath, []byte("saved audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")

	if code := transcriptionFailure(cfg, metadata, errors.New("engine unavailable")); code != 1 {
		t.Fatalf("code = %d", code)
	}
	loaded, err := session.LoadSession(session.SessionDir(metadata))
	if err != nil || loaded == nil {
		t.Fatalf("failed session = %#v, %v", loaded, err)
	}
	if loaded.Status != "failed" || len(loaded.Errors) != 1 || loaded.Errors[0] != "transcription failed: engine unavailable" {
		t.Fatalf("metadata = %#v", loaded)
	}
	for _, name := range []string{session.AudioFile, session.MetadataFile, session.StatusLogFile, session.ErrorLogFile, session.EventsFile} {
		if _, err := os.Stat(filepath.Join(session.SessionDir(metadata), name)); err != nil {
			t.Errorf("missing session file %s: %v", name, err)
		}
	}
	for _, name := range []string{session.StatusLogFile, session.ErrorLogFile} {
		data, readErr := os.ReadFile(filepath.Join(session.SessionDir(metadata), name))
		if readErr != nil || !strings.Contains(string(data), "transcription failed: engine unavailable") {
			t.Errorf("%s = %q, %v", name, data, readErr)
		}
	}
	records, err := events.Read(session.SessionDir(metadata), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 || records[len(records)-1]["event"] != "transcription.failed" {
		t.Fatalf("events = %#v", records)
	}
}
