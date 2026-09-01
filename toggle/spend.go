package toggle

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/models"
	"github.com/rob137/risper/spend"
)

func completionSpendLine(cfg config.Config, profiles map[string]models.Profile, now time.Time) (string, error) {
	rates := make([]spend.Rate, 0, len(profiles))
	currencies := make(map[string]struct{})
	hasOpenAI := false
	for _, profile := range profiles {
		if !strings.EqualFold(strings.TrimSpace(profile.Engine), "openai") {
			continue
		}
		hasOpenAI = true
		currency := strings.ToUpper(strings.TrimSpace(profile.BillingCurrency))
		if profile.BillingPricePerMinute <= 0 || currency == "" {
			continue
		}
		rates = append(rates, spend.Rate{
			Model: profile.Model, Currency: currency, PerMinute: profile.BillingPricePerMinute,
		})
		currencies[currency] = struct{}{}
	}
	if !hasOpenAI {
		return "", nil
	}
	if len(rates) == 0 {
		return "OpenAI spend estimate unavailable; configure billing price and currency.", nil
	}
	summary, err := spend.CalculateWithRates(cfg, now, rates...)
	if err != nil {
		return "OpenAI spend estimate unavailable.", err
	}
	for currency := range summary.ByCurrency {
		currencies[currency] = struct{}{}
	}
	codes := make([]string, 0, len(currencies))
	for currency := range currencies {
		codes = append(codes, currency)
	}
	sort.Strings(codes)
	parts := make([]string, 0, len(codes))
	for _, currency := range codes {
		totals := summary.ForCurrency(currency)
		parts = append(parts, fmt.Sprintf("%s %.4f today · %.4f this week · %.4f this month", currency, totals.Today, totals.Week, totals.Month))
	}
	return "Estimated OpenAI spend: " + strings.Join(parts, "; ") + ".", nil
}

// Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
