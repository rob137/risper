package transcriptionstate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/events"
	"github.com/rob137/risper/internal/files"
	"github.com/rob137/risper/session"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	root := t.TempDir()
	return config.Config{
		ConfigPath:               filepath.Join(root, "config.toml"),
		ModelsPath:               filepath.Join(root, "models.toml"),
		SessionsDir:              filepath.Join(root, "sessions"),
		StateDir:                 filepath.Join(root, "state"),
		CurrentTranscriptionPath: filepath.Join(root, "state", "current-transcription.json"),
		LogPath:                  filepath.Join(root, "state", "risper.log"),
		TranscriptionEngine:      "external",
		Model:                    "base.en",
		Language:                 "en",
		PasteMode:                "clipboard_only",
		SelectedModel:            "default",
	}
}

func TestStartCurrentSetWorkerAndFinish(t *testing.T) {
	cfg := testConfig(t)
	metadata, err := session.Create(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Start(cfg, metadata, "test-profile"); err != nil {
		t.Fatal(err)
	}
	state, err := Current(cfg)
	if err != nil || state == nil || state.Profile != "test-profile" || state.ControllerPID != os.Getpid() || state.WorkerPID != nil {
		t.Fatalf("unexpected current state: %+v, %v", state, err)
	}
	if err := SetWorkerPID(cfg, os.Getpid()); err != nil {
		t.Fatal(err)
	}
	state, err = Current(cfg)
	if err != nil || state == nil || state.WorkerPID == nil || *state.WorkerPID != os.Getpid() {
		t.Fatalf("worker PID was not persisted: %+v, %v", state, err)
	}
	if err := Finish(cfg); err != nil {
		t.Fatal(err)
	}
	if state, err := Current(cfg); err != nil || state != nil {
		t.Fatalf("state remained after Finish: %+v, %v", state, err)
	}
	if err := Finish(cfg); err != nil {
		t.Fatalf("Finish should be idempotent: %v", err)
	}
}

func TestCurrentRemovesMalformedOrStaleState(t *testing.T) {
	cfg := testConfig(t)
	if err := os.MkdirAll(filepath.Dir(cfg.CurrentTranscriptionPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.CurrentTranscriptionPath, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if state, err := Current(cfg); err != nil || state != nil {
		t.Fatalf("malformed state = %+v, %v", state, err)
	}
	stalePID := os.Getpid() + 1000000
	if err := files.AtomicWriteJSON(cfg.CurrentTranscriptionPath, State{ControllerPID: stalePID}); err != nil {
		t.Fatal(err)
	}
	if state, err := Current(cfg); err != nil || state != nil {
		t.Fatalf("stale state = %+v, %v", state, err)
	}
}

func TestCancelMarksSessionAndClearsState(t *testing.T) {
	cfg := testConfig(t)
	metadata, err := session.Create(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Start(cfg, metadata, "test-profile"); err != nil {
		t.Fatal(err)
	}
	state, err := Current(cfg)
	if err != nil || state == nil {
		t.Fatalf("missing state before cancel: %+v, %v", state, err)
	}
	if err := Cancel(cfg, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := session.LoadSession(session.SessionDir(metadata))
	if err != nil || loaded == nil || loaded.Status != "cancelled" || len(loaded.Errors) != 1 {
		t.Fatalf("unexpected cancelled session: %+v, %v", loaded, err)
	}
	if _, err := os.Stat(cfg.CurrentTranscriptionPath); !os.IsNotExist(err) {
		t.Fatalf("state file remains or unexpected stat error: %v", err)
	}
	records, err := events.Read(session.SessionDir(metadata), 0)
	if err != nil || records[len(records)-1]["event"] != "transcription.cancel_requested" {
		t.Fatalf("missing cancellation event: %#v, %v", records, err)
	}
	if _, err := os.Stat(cfg.LogPath); err != nil {
		t.Fatalf("missing application log: %v", err)
	}
}
