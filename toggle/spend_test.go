package toggle

import (
	"strings"
	"testing"
	"time"

	"github.com/rob137/risper/models"
	"github.com/rob137/risper/session"
)

func TestCompletionSpendLineIncludesLegacyOpenAISessions(t *testing.T) {
	_, cfg := stubEnvironment(t)
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.FixedZone("BST", 3600))
	metadata, err := session.CreateAt(cfg, now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	duration := 120.0
	ended := now.Add(-30 * time.Second).Format(time.RFC3339)
	metadata.DurationSeconds = &duration
	metadata.EndedAt = &ended
	metadata.Status = "complete"
	metadata.TranscriptionEngine = "openai"
	metadata.Model = "gpt-transcribe"
	if err := session.SaveMetadata(metadata); err != nil {
		t.Fatal(err)
	}
	line, err := completionSpendLine(cfg, map[string]models.Profile{
		"cloud": {Engine: "openai", Model: "gpt-transcribe", BillingPricePerMinute: 0.0045, BillingCurrency: "USD"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"USD 0.0090 today", "0.0090 this week", "0.0090 this month"} {
		if !strings.Contains(line, want) {
			t.Fatalf("spend line %q omitted %q", line, want)
		}
	}
}

func TestCompletionNotificationAddsSpendToEveryClipboardVariant(t *testing.T) {
	const spendLine = "Estimated OpenAI spend: USD 0.0010 today · 0.0020 this week · 0.0030 this month."
	tests := []struct {
		name    string
		request finish
		pasted  bool
		want    string
	}{
		{name: "pasted", request: finish{paste: true}, pasted: true, want: "Sent to the focused window"},
		{name: "paste unavailable", request: finish{paste: true}, want: "Paste unavailable"},
		{name: "copied", request: finish{}, want: "Transcript is on the clipboard"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, body := completionNotification(test.request, test.pasted, false, "", spendLine)
			if !strings.Contains(body, test.want) || !strings.Contains(body, spendLine) {
				t.Fatalf("notification body = %q", body)
			}
		})
	}
}

func TestCompletionNotificationReportsLocalFallback(t *testing.T) {
	_, body := completionNotification(finish{}, false, true, "whispercpp-small-en", "spend")
	if !strings.Contains(body, "Used whispercpp-small-en locally") || !strings.HasSuffix(body, "spend") {
		t.Fatalf("notification body = %q", body)
	}
}

// Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
