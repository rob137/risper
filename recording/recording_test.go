package recording

import (
	"os"
	"path/filepath"
	"strings"
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

func TestStartRejectsRecorderThatExitsDuringStartup(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	recorder := `#!/bin/sh
set -eu
output=
for arg in "$@"; do output="$arg"; done
case "$*" in
  *stream.capture.sink=true*)
    printf 'RIFF system audio padding' > "$output"
    trap 'exit 0' INT TERM
    while :; do sleep 0.01; done
    ;;
  *)
    exit 17
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "pw-record"), []byte(recorder), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin")
	cfg := config.Config{
		SessionsDir:         filepath.Join(root, "sessions"),
		StateDir:            filepath.Join(root, "state"),
		CurrentStatePath:    filepath.Join(root, "state", "current.json"),
		Language:            "en",
		Model:               "base.en",
		PasteMode:           "clipboard_only",
		TranscriptionEngine: "external",
	}

	state, err := Start(cfg, false)
	if err == nil || state != nil || !strings.Contains(err.Error(), "pw-record mic exited during startup") {
		t.Fatalf("Start() = state %v, err %v", state, err)
	}
	metadata, err := session.Last(cfg)
	if err != nil || metadata == nil {
		t.Fatalf("last session = %#v, %v", metadata, err)
	}
	if metadata.Status != "failed" || len(metadata.Errors) != 2 {
		t.Fatalf("metadata = %#v", metadata)
	}
	if current, currentErr := Current(cfg); currentErr != nil || current != nil {
		t.Fatalf("current state = %#v, %v", current, currentErr)
	}
}
