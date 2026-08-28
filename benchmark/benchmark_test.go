package benchmark

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob137/risper/models"
)

func TestRunProfileReportsExternalTranscriptAndFailure(t *testing.T) {
	audio := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(audio, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := runProfile(models.Profile{
		ID: "raw", Engine: "stub", Model: "m", Language: "en",
		Command: "printf 'external transcript\\n'",
	}, audio)
	if err != nil {
		t.Fatal(err)
	}
	if result.ReturnCode != 0 || result.TranscriptPreview != "external transcript" || result.TranscriptChars != len("external transcript") {
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

func TestRunProfileBoundsFailureDetails(t *testing.T) {
	audio := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(audio, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := runProfile(models.Profile{
		ID: "failed", Engine: "stub", Model: "m", Language: "en",
		Command: "i=1; while [ $i -le 20 ]; do printf 'failure line %d\\n' $i >&2; i=$((i + 1)); done; exit 9",
	}, audio)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.StderrTail) != 5 {
		t.Fatalf("stderr tail length = %d, want 5: %#v", len(result.StderrTail), result.StderrTail)
	}
	for _, line := range result.StderrTail {
		if len([]rune(line)) > 512 {
			t.Fatalf("stderr tail line is too long: %d", len([]rune(line)))
		}
	}
	if !strings.Contains(result.StderrTail[len(result.StderrTail)-1], "failure line 20") {
		t.Fatalf("stderr tail = %#v", result.StderrTail)
	}
}

func TestErrorTailBoundsEngineFailureDetails(t *testing.T) {
	result := errorTail(errors.New(strings.Repeat("x", 2048)), 5)
	if len(result) != 1 {
		t.Fatalf("error tail = %#v", result)
	}
	if len([]rune(result[0])) != 512 {
		t.Fatalf("error tail line length = %d, want 512", len([]rune(result[0])))
	}
}

// Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
