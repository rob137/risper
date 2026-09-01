package spend

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/session"
)

func spendConfig(t *testing.T) config.Config {
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

func TestCalculateUsesLocalDayMondayWeekAndCalendarMonth(t *testing.T) {
	cfg := spendConfig(t)
	zone := time.FixedZone("BST", 3600)
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, zone) // Wednesday.
	makePricedSession(t, cfg, time.Date(2026, 9, 2, 9, 0, 0, 0, zone), "2026-09-02T09:01:00+01:00", 1, 0.10, "USD")
	makePricedSession(t, cfg, time.Date(2026, 9, 1, 23, 0, 0, 0, zone), "2026-09-01T23:01:00+01:00", 1, 0.20, "USD")
	makePricedSession(t, cfg, time.Date(2026, 8, 31, 23, 0, 0, 0, zone), "2026-08-31T23:01:00+01:00", 1, 0.30, "USD")
	makePricedSession(t, cfg, time.Date(2026, 8, 30, 23, 0, 0, 0, zone), "2026-08-30T23:01:00+01:00", 1, 0.40, "USD")
	makePricedSession(t, cfg, time.Date(2026, 8, 1, 12, 0, 0, 0, zone), "2026-08-01T12:01:00+01:00", 1, 0.50, "USD")
	makePricedSession(t, cfg, time.Date(2026, 7, 31, 12, 0, 0, 0, zone), "2026-07-31T12:01:00+01:00", 1, 0.60, "USD")

	summary, err := Calculate(cfg, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := summary.ForCurrency("usd")
	if !closeEnough(got.Today, 0.10) || !closeEnough(got.Week, 0.60) || !closeEnough(got.Month, 0.30) {
		t.Fatalf("totals = %#v, want today=.10 week=.60 month=.30", got)
	}
}

func TestCalculateIgnoresMalformedIncompleteFutureAndNonOpenAISessions(t *testing.T) {
	cfg := spendConfig(t)
	zone := time.FixedZone("BST", 3600)
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, zone)
	valid := makePricedSession(t, cfg, now.Add(-time.Hour), now.Add(-time.Hour).Format(time.RFC3339), 1, 1, "USD")
	valid.Status = "recording"
	if err := session.SaveMetadata(valid); err != nil {
		t.Fatal(err)
	}
	failed := makePricedSession(t, cfg, now.Add(-2*time.Hour), now.Add(-2*time.Hour).Format(time.RFC3339), 1, 2, "USD")
	failed.Status = "failed"
	if err := session.SaveMetadata(failed); err != nil {
		t.Fatal(err)
	}
	external := makePricedSession(t, cfg, now.Add(-3*time.Hour), now.Add(-3*time.Hour).Format(time.RFC3339), 1, 3, "USD")
	external.TranscriptionEngine = "external"
	if err := session.SaveMetadata(external); err != nil {
		t.Fatal(err)
	}
	future := makePricedSession(t, cfg, now.Add(time.Hour), now.Add(time.Hour).Format(time.RFC3339), 1, 4, "USD")
	if err := session.SaveMetadata(future); err != nil {
		t.Fatal(err)
	}
	badDuration := makePricedSession(t, cfg, now.Add(-4*time.Hour), now.Add(-4*time.Hour).Format(time.RFC3339), -1, 5, "USD")
	if err := session.SaveMetadata(badDuration); err != nil {
		t.Fatal(err)
	}
	badTimestamp := makePricedSession(t, cfg, now.Add(-5*time.Hour), "not-a-timestamp", 1, 6, "USD")
	badTimestamp.StartedAt = "also-not-a-timestamp"
	if err := session.SaveMetadata(badTimestamp); err != nil {
		t.Fatal(err)
	}
	malformedDir := filepath.Join(cfg.SessionsDir, "malformed")
	if err := os.MkdirAll(malformedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(malformedDir, session.MetadataFile), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err := Calculate(cfg, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := summary.ForCurrency("USD"); got != (Totals{}) {
		t.Fatalf("ignored-session totals = %#v, want zero", got)
	}
}

func TestCalculateUsesLegacyFallbackOnlyForMatchingModelAndCurrency(t *testing.T) {
	cfg := spendConfig(t)
	zone := time.FixedZone("BST", 3600)
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, zone)
	legacy := makeSession(t, cfg, now.Add(-time.Minute), 90)
	legacy.Model = "gpt-transcribe"
	legacy.TranscriptionEngine = "openai"
	legacy.TranscriptionCost = nil
	legacy.TranscriptionRatePerMinute = nil
	legacy.TranscriptionCurrency = ""
	legacy.EndedAt = nil
	if err := session.SaveMetadata(legacy); err != nil {
		t.Fatal(err)
	}
	wrongModel := makeSession(t, cfg, now.Add(-time.Minute), 60)
	wrongModel.Model = "other-model"
	wrongModel.TranscriptionEngine = "openai"
	wrongModel.TranscriptionCost = nil
	wrongModel.TranscriptionRatePerMinute = nil
	wrongModel.TranscriptionCurrency = ""
	if err := session.SaveMetadata(wrongModel); err != nil {
		t.Fatal(err)
	}

	fallback := Rate{Model: "gpt-transcribe", Currency: "USD", PerMinute: 0.20}
	summary, err := Calculate(cfg, now, &fallback)
	if err != nil {
		t.Fatal(err)
	}
	got := summary.ForCurrency("USD")
	if !closeEnough(got.Today, 0.30) || !closeEnough(got.Week, 0.30) || !closeEnough(got.Month, 0.30) {
		t.Fatalf("legacy fallback totals = %#v, want .30 in each period", got)
	}
}

func TestCalculateWithRatesSupportsMultipleLegacyModelsAndCurrencies(t *testing.T) {
	cfg := spendConfig(t)
	zone := time.FixedZone("BST", 3600)
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, zone)
	usd := makeSession(t, cfg, now.Add(-time.Minute), 60)
	usd.Model = "gpt-transcribe"
	usd.TranscriptionEngine = "openai"
	gbp := makeSession(t, cfg, now.Add(-2*time.Minute), 120)
	gbp.Model = "gpt-legacy"
	gbp.TranscriptionEngine = "openai"
	gbp.TranscriptionCurrency = "GBP"
	for _, metadata := range []*session.Metadata{usd, gbp} {
		if err := session.SaveMetadata(metadata); err != nil {
			t.Fatal(err)
		}
	}

	summary, err := CalculateWithRates(cfg, now,
		Rate{Model: "gpt-transcribe", Currency: "USD", PerMinute: math.NaN()},
		Rate{Model: "gpt-transcribe", Currency: "USD", PerMinute: 0.20},
		Rate{Model: "gpt-transcribe", Currency: "USD", PerMinute: 0.90}, // duplicate: first valid wins
		Rate{Model: "gpt-legacy", Currency: "GBP", PerMinute: 0.30},
		Rate{Model: "gpt-legacy", Currency: "GBP", PerMinute: math.Inf(1)}, // invalid duplicate
		Rate{Model: "gpt-legacy", Currency: "", PerMinute: 0.40},           // invalid scope
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := summary.ForCurrency("USD"); !closeEnough(got.Today, 0.20) {
		t.Fatalf("USD legacy total = %#v, want .20", got)
	}
	if got := summary.ForCurrency("GBP"); !closeEnough(got.Today, 0.60) {
		t.Fatalf("GBP legacy total = %#v, want .60", got)
	}
}

func TestCalculateWithRatesDoesNotGuessCurrencyForAmbiguousLegacyMetadata(t *testing.T) {
	cfg := spendConfig(t)
	zone := time.FixedZone("BST", 3600)
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, zone)
	legacy := makeSession(t, cfg, now.Add(-time.Minute), 60)
	legacy.Model = "gpt-transcribe"
	legacy.TranscriptionEngine = "openai"
	legacy.TranscriptionCurrency = ""
	if err := session.SaveMetadata(legacy); err != nil {
		t.Fatal(err)
	}
	summary, err := CalculateWithRates(cfg, now,
		Rate{Model: "gpt-transcribe", Currency: "USD", PerMinute: 0.20},
		Rate{Model: "gpt-transcribe", Currency: "GBP", PerMinute: 0.30},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.ByCurrency) != 0 {
		t.Fatalf("ambiguous legacy currency totals = %#v, want empty", summary.ByCurrency)
	}
}

func TestCalculatePrefersRecordedCostAndRateAcrossPriceChanges(t *testing.T) {
	cfg := spendConfig(t)
	zone := time.FixedZone("BST", 3600)
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, zone)
	oldCost := makePricedSession(t, cfg, now.Add(-time.Hour), now.Add(-time.Hour).Format(time.RFC3339), 60, 0.01, "USD")
	oldCost.TranscriptionRatePerMinute = floatPtr(0.99)
	if err := session.SaveMetadata(oldCost); err != nil {
		t.Fatal(err)
	}
	rateOnly := makeSession(t, cfg, now.Add(-2*time.Hour), 120)
	rateOnly.TranscriptionEngine = "openai"
	rateOnly.Model = "gpt-transcribe"
	rateOnly.TranscriptionCost = nil
	rateOnly.TranscriptionRatePerMinute = floatPtr(0.03)
	rateOnly.TranscriptionCurrency = "USD"
	if err := session.SaveMetadata(rateOnly); err != nil {
		t.Fatal(err)
	}

	fallback := Rate{Model: "gpt-transcribe", Currency: "USD", PerMinute: 0.50}
	summary, err := Calculate(cfg, now, &fallback)
	if err != nil {
		t.Fatal(err)
	}
	if got := summary.ForCurrency("USD"); !closeEnough(got.Today, 0.07) || !closeEnough(got.Week, 0.07) || !closeEnough(got.Month, 0.07) {
		t.Fatalf("recorded pricing totals = %#v, want .07", got)
	}
}

func TestCalculateKeepsCurrenciesSeparate(t *testing.T) {
	cfg := spendConfig(t)
	zone := time.FixedZone("BST", 3600)
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, zone)
	makePricedSession(t, cfg, now.Add(-time.Minute), now.Add(-time.Minute).Format(time.RFC3339), 60, 1, "USD")
	makePricedSession(t, cfg, now.Add(-2*time.Minute), now.Add(-2*time.Minute).Format(time.RFC3339), 60, 2, "GBP")
	summary, err := Calculate(cfg, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ForCurrency("USD").Today != 1 || summary.ForCurrency("GBP").Today != 2 {
		t.Fatalf("currency totals = %#v", summary.ByCurrency)
	}
	if _, _, ok := summary.SingleCurrency(); ok {
		t.Fatal("SingleCurrency unexpectedly succeeded with two currencies")
	}
}

func TestRecordEstimateSnapshotsCloudPricingAndClearsItForLocalResults(t *testing.T) {
	duration := 90.0
	metadata := &session.Metadata{DurationSeconds: &duration}
	RecordEstimate(metadata, "openai", 0.0045, "usd")
	if metadata.TranscriptionCost == nil || !closeEnough(*metadata.TranscriptionCost, 0.00675) {
		t.Fatalf("cost = %#v, want 0.00675", metadata.TranscriptionCost)
	}
	if metadata.TranscriptionRatePerMinute == nil || *metadata.TranscriptionRatePerMinute != 0.0045 || metadata.TranscriptionCurrency != "USD" {
		t.Fatalf("pricing snapshot = %#v", metadata)
	}
	RecordEstimate(metadata, "whisper.cpp", 0, "")
	if metadata.TranscriptionCost != nil || metadata.TranscriptionRatePerMinute != nil || metadata.TranscriptionCurrency != "" {
		t.Fatalf("local result kept cloud pricing = %#v", metadata)
	}
}

func makePricedSession(t *testing.T, cfg config.Config, started time.Time, ended string, duration, cost float64, currency string) *session.Metadata {
	metadata := makeSession(t, cfg, started, duration)
	metadata.Status = "complete"
	metadata.TranscriptionEngine = "openai"
	metadata.Model = "gpt-transcribe"
	metadata.TranscriptionCost = floatPtr(cost)
	metadata.TranscriptionCurrency = currency
	metadata.EndedAt = stringPtr(ended)
	if err := session.SaveMetadata(metadata); err != nil {
		t.Fatal(err)
	}
	return metadata
}

func makeSession(t *testing.T, cfg config.Config, started time.Time, duration float64) *session.Metadata {
	metadata, err := session.CreateAt(cfg, started)
	if err != nil {
		t.Fatal(err)
	}
	metadata.DurationSeconds = floatPtr(duration)
	metadata.Status = "complete"
	if err := session.SaveMetadata(metadata); err != nil {
		t.Fatal(err)
	}
	return metadata
}

func floatPtr(value float64) *float64 { return &value }
func stringPtr(value string) *string  { return &value }

func closeEnough(got, want float64) bool { return math.Abs(got-want) < 1e-9 }

// Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
