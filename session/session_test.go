package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rob137/risper/config"
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

func TestCreateAtWritesFrozenSessionFilesAndEvent(t *testing.T) {
	cfg := testConfig(t)
	when := time.Date(2026, 8, 19, 10, 20, 30, 0, time.FixedZone("BST", 3600))
	metadata, err := CreateAt(cfg, when)
	if err != nil {
		t.Fatal(err)
	}
	dir := SessionDir(metadata)
	for _, name := range []string{"metadata.json", "events.jsonl", "status.log", "error.log"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	if metadata.SessionID != filepath.Base(dir) || metadata.Status != "recording" || metadata.EndedAt != nil {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	trail, err := os.ReadFile(EventsPath(metadata))
	if err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(trimLine(trail), &event); err != nil || event["event"] != "session.created" {
		t.Fatalf("created event = %s, %v", trail, err)
	}
}

func trimLine(data []byte) []byte {
	for len(data) > 0 && (data[len(data)-1] == '\n' || data[len(data)-1] == '\r') {
		data = data[:len(data)-1]
	}
	return data
}

func TestCreateAtSuffixesCollidingIDs(t *testing.T) {
	cfg := testConfig(t)
	when := time.Date(2026, 8, 19, 10, 20, 30, 0, time.UTC)
	first, err := CreateAt(cfg, when)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CreateAt(cfg, when)
	if err != nil {
		t.Fatal(err)
	}
	if first.SessionID != "2026-08-19_11-20-30" && first.SessionID != "2026-08-19_10-20-30" {
		t.Fatalf("first session id = %q", first.SessionID)
	}
	if second.SessionID != first.SessionID+"-2" {
		t.Fatalf("second session id = %q", second.SessionID)
	}
}

func TestRewritePreservesUnknownAndOptionalMetadataKeys(t *testing.T) {
	cfg := testConfig(t)
	dir := filepath.Join(cfg.SessionsDir, "legacy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	metadataJSON := `{"audio_path":"` + filepath.Join(dir, "audio.wav") + `","duration_seconds":null,"ended_at":null,"errors":[],"language":"en","model":"base.en","session_id":"legacy","session_type":"wayland","started_at":"2026-08-19T10:00:00+01:00","status":"complete","target_app":null,"transcript_clean_path":"` + filepath.Join(dir, "transcript.clean.txt") + `","transcript_raw_path":"` + filepath.Join(dir, "transcript.raw.txt") + `","transcription_engine":"external","future_key":{"kept":true}}`
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(metadataJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata, err := LoadSession(dir)
	if err != nil || metadata == nil {
		t.Fatalf("load legacy metadata = %#v, %v", metadata, err)
	}
	metadata.Status = "recorded"
	if err := SaveMetadata(metadata); err != nil {
		t.Fatal(err)
	}
	var persisted map[string]any
	data, _ := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted["future_key"].(map[string]any)["kept"] != true {
		t.Fatalf("unknown key was lost: %#v", persisted)
	}
	if _, present := persisted["paste_attempted"]; present {
		t.Fatalf("absent optional key was introduced: %#v", persisted)
	}
}

func TestPruneAndRecover(t *testing.T) {
	cfg := testConfig(t)
	retention := 3600.0
	cfg.AudioRetentionSeconds = &retention
	old, err := CreateAt(cfg, time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old.AudioPath, []byte("RIFF"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	if err := os.Chtimes(old.AudioPath, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	count, err := PruneExpiredAudioAt(cfg, now)
	if err != nil || count != 1 {
		t.Fatalf("prune = %d, %v", count, err)
	}
	if _, err := os.Stat(old.AudioPath); !os.IsNotExist(err) {
		t.Fatalf("audio was not pruned: %v", err)
	}
	pruned, err := LoadSession(SessionDir(old))
	if err != nil || pruned == nil || pruned.AudioPrunedAt == nil {
		t.Fatalf("pruned metadata = %#v, %v", pruned, err)
	}
	pruned.Status = "complete"
	if err := SaveMetadata(pruned); err != nil {
		t.Fatal(err)
	}

	recovered, err := CreateAt(cfg, now)
	if err != nil {
		t.Fatal(err)
	}
	count, err = MarkIncompleteRecordingsRecoveredAt(cfg, now)
	if err != nil || count != 1 {
		t.Fatalf("recovery = %d, %v", count, err)
	}
	reloaded, err := LoadSession(SessionDir(recovered))
	if err != nil || reloaded == nil || reloaded.Status != "recovered" || reloaded.EndedAt == nil || reloaded.Errors[0] != RecoveryMessage() {
		t.Fatalf("recovered metadata = %#v, %v", reloaded, err)
	}
}
