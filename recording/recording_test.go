package recording

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/events"
	"github.com/rob137/risper/session"
)

func TestMarkStartFailedPersistsRecoverableSession(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		SessionsDir: filepath.Join(root, "sessions"),
		Language:    "en",
		Model:       "base.en",
		PasteMode:   "clipboard_only",
	}
	metadata, err := session.Create(cfg)
	if err != nil {
		t.Fatal(err)
	}

	markStartFailed(metadata, map[string]int{string(Mic): 1234})

	loaded, err := session.LoadSession(session.SessionDir(metadata))
	if err != nil || loaded == nil {
		t.Fatalf("failed session = %#v, %v", loaded, err)
	}
	if loaded.Status != "failed" {
		t.Fatalf("status = %q", loaded.Status)
	}
	if len(loaded.Errors) != 1 || loaded.Errors[0] != "could not start all audio recorders" {
		t.Fatalf("errors = %#v", loaded.Errors)
	}
	for _, name := range []string{session.MetadataFile, session.StatusLogFile, session.ErrorLogFile, session.EventsFile} {
		if _, err := os.Stat(filepath.Join(session.SessionDir(metadata), name)); err != nil {
			t.Errorf("missing recoverable session file %s: %v", name, err)
		}
	}
	records, err := events.Read(session.SessionDir(metadata), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 || records[len(records)-1]["event"] != "recorder.start_failed" {
		t.Fatalf("events = %#v", records)
	}
}
