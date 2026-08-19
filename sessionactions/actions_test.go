package sessionactions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rob137/risper/session"
)

func TestTranscriptPreviewPrefersCleanAndFallsBackToLegacyError(t *testing.T) {
	root := t.TempDir()
	clean := filepath.Join(root, "transcript.clean.txt")
	if err := os.WriteFile(clean, []byte(" hello\n\nfrom\tRisper "), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := &session.Metadata{
		TranscriptCleanPath: clean,
		TranscriptRawPath:   filepath.Join(root, "missing.raw.txt"),
		Errors:              []string{"old error"},
	}
	if got := TranscriptPreview(metadata, 12); got != "hello from R" {
		t.Fatalf("preview = %q", got)
	}
	if got := TranscriptPreview(&session.Metadata{Errors: []string{"something broke loudly"}}, 9); got != "something" {
		t.Fatalf("error preview = %q", got)
	}
}

func TestTranscriptPathLoadsOldMetadataWithoutPasteFields(t *testing.T) {
	root := t.TempDir()
	raw := filepath.Join(root, "transcript.raw.txt")
	if err := os.WriteFile(raw, []byte("legacy transcript"), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := &session.Metadata{TranscriptRawPath: raw}
	if got := TranscriptPath(metadata); got != raw {
		t.Fatalf("path = %q, want %q", got, raw)
	}
}
