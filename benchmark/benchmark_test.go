package benchmark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob137/risper/models"
)

func TestRunProfilePrefersRawFileAndReportsFailure(t *testing.T) {
	audio := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(audio, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := runProfile(models.Profile{
		ID: "raw", Engine: "stub", Model: "m", Language: "en",
		Command: "printf 'raw transcript\\n' > {raw}; printf 'stdout transcript\\n'",
	}, audio)
	if err != nil {
		t.Fatal(err)
	}
	if result.ReturnCode != 0 || result.TranscriptPreview != "raw transcript" || result.TranscriptChars != len("raw transcript") {
		t.Fatalf("result = %#v", result)
	}

	failed, err := runProfile(models.Profile{
		ID: "failed", Engine: "stub", Model: "m", Language: "en",
		Command: "printf 'line 1\\nline 2\\n' >&2; exit 9",
	}, audio)
	if err != nil {
		t.Fatal(err)
	}
	if failed.ReturnCode != 9 || !strings.Contains(strings.Join(failed.StderrTail, "\n"), "line 2") {
		t.Fatalf("failed result = %#v", failed)
	}
}
