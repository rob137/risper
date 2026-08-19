package events

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndReadPreserveFieldsAndLimit(t *testing.T) {
	dir := t.TempDir()
	when := time.Date(2026, 8, 19, 10, 20, 30, 0, time.FixedZone("BST", 3600))
	if _, err := AppendAt(dir, "session.created", when, map[string]any{
		"session_id": "one",
		"nested":     map[string]any{"source": "mic"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := AppendAt(dir, "paste.result", when.Add(time.Second), map[string]any{"ok": false, "transcript_chars": 42}); err != nil {
		t.Fatal(err)
	}
	records, err := Read(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0]["event"] != "paste.result" || records[0]["ok"] != false {
		t.Fatalf("unexpected limited records: %#v", records)
	}
	all, err := Read(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if all[0]["timestamp"] != "2026-08-19T10:20:30+01:00" || all[0]["nested"].(map[string]any)["source"] != "mic" {
		t.Fatalf("unexpected first record: %#v", all[0])
	}
}

func TestReadRetainsMalformedLinesAsDiagnostics(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{not json\nnull\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := Read(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0]["event"] != "diagnostic.invalid_event_line" || records[1]["raw"] != "null" {
		t.Fatalf("unexpected diagnostics: %#v", records)
	}
}

func TestReadMissingTrailIsEmpty(t *testing.T) {
	records, err := Read(filepath.Join(t.TempDir(), "missing"), 0)
	if err != nil || len(records) != 0 {
		t.Fatalf("Read missing trail = %#v, %v", records, err)
	}
}
