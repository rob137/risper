// Package spend calculates estimated OpenAI transcription spend from durable
// session metadata. It deliberately does not contact the billing API: the
// transcription API does not expose the per-minute price used for estimates.
package spend

import (
	"math"
	"strings"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/session"
)

// Rate is a caller-supplied legacy estimate. It is used only when a complete
// OpenAI session has no recorded cost or rate, and only for the exact model
// named here. Currency is retained as a separate total; no conversion occurs.
type Rate struct {
	Model     string
	Currency  string
	PerMinute float64
}

// FallbackRate is an expressive alias for callers that want to make the
// legacy nature of a rate explicit.
type FallbackRate = Rate

// Totals contains spend in one billing currency for the three useful windows.
type Totals struct {
	Today float64
	Week  float64
	Month float64
}

// Summary keeps currencies separate. A map is used because metadata can span
// a currency change over time, and silently converting would imply an
// exchange rate that Risper does not know.
type Summary struct {
	ByCurrency map[string]Totals
}

// RecordEstimate snapshots the rate and estimated cost for the transcription
// result currently represented by metadata. A local result clears any stale
// cloud estimate left by an earlier attempt or retranscription.
func RecordEstimate(metadata *session.Metadata, engine string, ratePerMinute float64, currency string) {
	if metadata == nil {
		return
	}
	metadata.TranscriptionCost = nil
	metadata.TranscriptionCurrency = ""
	metadata.TranscriptionRatePerMinute = nil
	currency = normalizeCurrency(currency)
	if !strings.EqualFold(strings.TrimSpace(engine), "openai") || !validRate(ratePerMinute) || currency == "" || metadata.DurationSeconds == nil || !validDuration(*metadata.DurationSeconds) {
		return
	}
	cost := ratePerMinute * (*metadata.DurationSeconds / 60)
	metadata.TranscriptionRatePerMinute = &ratePerMinute
	metadata.TranscriptionCost = &cost
	metadata.TranscriptionCurrency = currency
}

// ForCurrency returns totals for currency, or zero totals when none exist.
// Currency matching is case-insensitive and ignores surrounding whitespace.
func (summary Summary) ForCurrency(currency string) Totals {
	return summary.ByCurrency[normalizeCurrency(currency)]
}

// SingleCurrency returns the only currency and its totals. It is convenient
// for the normal case where Rob has one OpenAI billing currency. ok is false
// when there are no totals or when more than one currency is present.
func (summary Summary) SingleCurrency() (currency string, totals Totals, ok bool) {
	if len(summary.ByCurrency) != 1 {
		return "", Totals{}, false
	}
	for currency, totals := range summary.ByCurrency {
		return currency, totals, true
	}
	return "", Totals{}, false
}

// Calculate scans all durable sessions and returns spend windows relative to
// now. now's location defines the day, Monday-start week, and calendar month.
// A nil fallback means legacy sessions without recorded pricing are ignored.
func Calculate(cfg config.Config, now time.Time, fallback *Rate) (Summary, error) {
	if fallback == nil {
		return CalculateWithRates(cfg, now)
	}
	return CalculateWithRates(cfg, now, *fallback)
}

// CalculateWithRates is the multi-profile form of Calculate. Each valid rate
// is scoped to its exact (model, currency) pair. For duplicate pairs, the
// first valid rate wins; invalid entries are ignored. This makes the result
// deterministic while allowing a caller to pass rates from several profiles.
func CalculateWithRates(cfg config.Config, now time.Time, fallbacks ...Rate) (Summary, error) {
	summary := Summary{ByCurrency: make(map[string]Totals)}
	sessions, err := session.All(cfg)
	if err != nil {
		return summary, err
	}
	if now.IsZero() {
		return summary, nil
	}
	localNow := now.In(now.Location())
	todayStart := startOfDay(localNow)
	weekStart := todayStart.AddDate(0, 0, -mondayOffset(localNow.Weekday()))
	monthStart := time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, localNow.Location())
	legacyRates := indexRates(fallbacks)

	for _, metadata := range sessions {
		amount, currency, at, ok := sessionAmount(metadata, legacyRates)
		if !ok {
			continue
		}
		at = at.In(localNow.Location())
		if at.After(localNow) {
			continue
		}
		totals := summary.ByCurrency[currency]
		if !at.Before(todayStart) {
			totals.Today += amount
		}
		if !at.Before(weekStart) {
			totals.Week += amount
		}
		if !at.Before(monthStart) {
			totals.Month += amount
		}
		summary.ByCurrency[currency] = totals
	}
	return summary, nil
}

// CalculateWithRate is a value-taking convenience for callers with a fixed
// fallback rate. A zero Rate has the same effect as no fallback.
func CalculateWithRate(cfg config.Config, now time.Time, fallback Rate) (Summary, error) {
	return CalculateWithRates(cfg, now, fallback)
}

// sessionAmount validates a session and computes its amount. The timestamp
// is ended_at when it parses successfully, otherwise started_at. This lets a
// partially rewritten legacy record remain useful without trusting a bad end
// marker.
func sessionAmount(metadata *session.Metadata, fallback map[string]Rate) (float64, string, time.Time, bool) {
	if metadata == nil || strings.ToLower(strings.TrimSpace(metadata.Status)) != "complete" || strings.ToLower(strings.TrimSpace(metadata.TranscriptionEngine)) != "openai" {
		return 0, "", time.Time{}, false
	}
	if metadata.DurationSeconds == nil || !validDuration(*metadata.DurationSeconds) {
		return 0, "", time.Time{}, false
	}
	at, ok := metadataTime(metadata.EndedAt)
	if !ok {
		at, ok = metadataTime(metadataStringPtr(metadata.StartedAt))
	}
	if !ok {
		return 0, "", time.Time{}, false
	}

	currency := normalizeCurrency(metadata.TranscriptionCurrency)
	if metadata.TranscriptionCost != nil {
		if validCost(*metadata.TranscriptionCost) && currency != "" {
			return *metadata.TranscriptionCost, currency, at, true
		}
		// A malformed recorded cost should not be silently replaced by an
		// unrelated fallback. A valid recorded rate can still be used.
	}
	if metadata.TranscriptionRatePerMinute != nil && validRate(*metadata.TranscriptionRatePerMinute) && currency != "" {
		return *metadata.TranscriptionRatePerMinute * (*metadata.DurationSeconds / 60), currency, at, true
	}

	// A fallback is intentionally restricted to sessions with no recorded
	// pricing fields, and to the same model and billing currency. This prevents
	// a price change from retroactively changing sessions that have a snapshot.
	if metadata.TranscriptionCost != nil || metadata.TranscriptionRatePerMinute != nil {
		return 0, "", time.Time{}, false
	}
	fallbackCurrency := normalizeCurrency(metadata.TranscriptionCurrency)
	rate, found := legacyRate(fallback, strings.TrimSpace(metadata.Model), fallbackCurrency)
	if !found {
		return 0, "", time.Time{}, false
	}
	return rate.PerMinute * (*metadata.DurationSeconds / 60), normalizeCurrency(rate.Currency), at, true
}

func legacyRate(rates map[string]Rate, model, currency string) (Rate, bool) {
	if currency != "" {
		rate, found := rates[rateKey(model, currency)]
		return rate, found
	}
	// Very old metadata has no currency. It is safe to use a fallback only
	// when that model has one unambiguous currency; multiple currencies would
	// make the historical amount unknowable without inventing a conversion.
	var match Rate
	found := false
	for key, rate := range rates {
		if !strings.HasPrefix(key, model+"\x00") {
			continue
		}
		if found {
			return Rate{}, false
		}
		match, found = rate, true
	}
	return match, found
}

func indexRates(rates []Rate) map[string]Rate {
	indexed := make(map[string]Rate)
	for _, rate := range rates {
		model := strings.TrimSpace(rate.Model)
		currency := normalizeCurrency(rate.Currency)
		if model == "" || currency == "" || !validRate(rate.PerMinute) {
			continue
		}
		key := rateKey(model, currency)
		if _, exists := indexed[key]; !exists {
			rate.Model = model
			rate.Currency = currency
			indexed[key] = rate
		}
	}
	return indexed
}

func rateKey(model, currency string) string { return model + "\x00" + currency }

func metadataTime(value *string) (time.Time, bool) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*value))
	return parsed, err == nil
}

func metadataStringPtr(value string) *string { return &value }

func validDuration(value float64) bool {
	return value > 0 && math.IsNaN(value) == false && math.IsInf(value, 0) == false
}

func validCost(value float64) bool {
	return value >= 0 && math.IsNaN(value) == false && math.IsInf(value, 0) == false
}

func validRate(value float64) bool {
	return value > 0 && math.IsNaN(value) == false && math.IsInf(value, 0) == false
}

func normalizeCurrency(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }

func startOfDay(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func mondayOffset(day time.Weekday) int {
	if day == time.Sunday {
		return 6
	}
	return int(day - time.Monday)
}

// Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
