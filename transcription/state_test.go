package transcription

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/session"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestStartSetWorkerAndFinish(t *testing.T) {
	cfg := testConfig(t)
	metadata, err := session.Create(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Start(cfg, metadata, "test-profile"); err != nil {
		t.Fatal(err)
	}
	state, err := Current(cfg)
	if err != nil || state == nil || state.Profile != "test-profile" || state.WorkerPID != nil {
		t.Fatalf("current state = %#v, %v", state, err)
	}
	if err := SetWorkerPID(cfg, os.Getpid()); err != nil {
		t.Fatal(err)
	}
	state, err = Current(cfg)
	if err != nil || state == nil || state.WorkerPID == nil || *state.WorkerPID != os.Getpid() {
		t.Fatalf("worker state = %#v, %v", state, err)
	}
	if err := Finish(cfg); err != nil {
		t.Fatal(err)
	}
	if state, err := Current(cfg); err != nil || state != nil {
		t.Fatalf("finished state = %#v, %v", state, err)
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
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := Cancel(cfg, state)
	if err != nil || !cancelled {
		t.Fatalf("cancel = %v, %v", cancelled, err)
	}
	persisted, err := session.LoadSession(session.SessionDir(metadata))
	if err != nil || persisted.Status != "cancelled" || len(persisted.Errors) != 1 {
		t.Fatalf("cancelled metadata = %#v, %v", persisted, err)
	}
	if _, err := os.Stat(cfg.CurrentTranscriptionPath); !os.IsNotExist(err) {
		t.Fatalf("state file still exists: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(session.SessionDir(metadata), "events.jsonl"))
	if len(data) == 0 {
		t.Fatal("missing cancellation event")
	}
	var last map[string]any
	for _, line := range splitJSONLines(data) {
		if err := json.Unmarshal(line, &last); err != nil {
			t.Fatal(err)
		}
	}
	if last["event"] != "transcription.cancel_requested" {
		t.Fatalf("last event = %#v", last)
	}
}

func splitJSONLines(data []byte) [][]byte {
	var lines [][]byte
	for len(data) > 0 {
		index := 0
		for index < len(data) && data[index] != '\n' {
			index++
		}
		if index > 0 {
			lines = append(lines, data[:index])
		}
		if index == len(data) {
			break
		}
		data = data[index+1:]
	}
	return lines
}
