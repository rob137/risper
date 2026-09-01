// Package models owns the profile registry in models.toml.
package models

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/rob137/risper/config"
)

const DefaultModels = `# Risper model profiles.
#
# Add profiles like:
#
# [models.whispercpp-base-en]
# engine = "whisper.cpp"
# model = "base.en"
# language = "en"
# command = "/path/to/whisper-cli -m /path/to/model.bin -f {audio} -l {language} --prompt \"{prompt}\" -nt -otxt -of {raw_no_txt} -mc 0"
#
# An optional ` + "`prompt`" + ` biases decoding toward the words it lists (proper nouns,
# names, jargon). It is rendered into the command's {prompt} placeholder. Keep it
# a short comma list, not a paragraph. Local engines and supported cloud engines
# may use it.
#
# Cloud transcription is opt-in. The engine implementation reads the key file
# locally and sends the audio to OpenAI; it does not make OpenAI the default.
# [models.openai-gpt-transcribe]
# engine = "openai"
# model = "gpt-transcribe"
# language = "en"
# api_key_file = "~/.config/openai/key"
# billing_price_per_minute = 0.0045
# billing_currency = "USD"
# fallback_profile = "whispercpp-small-en"
# fallback_timeout_seconds = 15
# prompt = "A short comma-separated list of names and terms to recognize."
#
# Keep the key file outside this registry, mode 0600, and never commit it.
# See README.md and docs/models.md for the opt-in privacy and cost trade-off.
#
# Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
`

type Profile struct {
	ID                     string
	Engine                 string
	Model                  string
	Language               string
	Command                string
	Prompt                 string
	APIKeyFile             string
	BillingPricePerMinute  float64
	BillingCurrency        string
	FallbackProfile        string
	FallbackTimeoutSeconds int
}

const (
	defaultOpenAIFallbackTimeoutSeconds = 15
	defaultGPTTranscribePricePerMinute  = 0.0045
	defaultGPTTranscribeCurrency        = "USD"
)

func EnsureFile(cfg config.Config) error {
	if err := os.MkdirAll(filepath.Dir(cfg.ModelsPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(cfg.ModelsPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(cfg.ModelsPath, []byte(DefaultModels), 0o644)
}

func Load(cfg config.Config) (map[string]Profile, error) {
	if err := EnsureFile(cfg); err != nil {
		return nil, err
	}
	var raw map[string]any
	if _, err := toml.DecodeFile(cfg.ModelsPath, &raw); err != nil {
		return nil, fmt.Errorf("decode %s: %w", cfg.ModelsPath, err)
	}
	profiles := make(map[string]Profile)
	modelsTable, ok := table(raw["models"])
	if ok {
		for profileID, value := range modelsTable {
			data, ok := table(value)
			if !ok {
				continue
			}
			command := strings.TrimSpace(valueString(data["command"]))
			engine := valueString(data["engine"])
			if engine == "" {
				engine = "external"
			}
			if command == "" && engine != "openai" {
				continue
			}
			language := valueString(data["language"])
			if language == "" {
				language = cfg.Language
			}
			model := valueString(data["model"])
			if model == "" {
				model = profileID
			}
			apiKeyFile := strings.TrimSpace(valueString(data["api_key_file"]))
			if engine == "openai" && apiKeyFile == "" {
				apiKeyFile = "~/.config/openai/key"
			}
			billingPrice, billingPriceSet, err := valueFloat(data, "billing_price_per_minute")
			if err != nil {
				return nil, fmt.Errorf("profile %q billing_price_per_minute: %w", profileID, err)
			}
			if billingPrice < 0 || math.IsNaN(billingPrice) || math.IsInf(billingPrice, 0) {
				return nil, fmt.Errorf("profile %q billing_price_per_minute must be a finite non-negative number", profileID)
			}
			billingCurrency := strings.ToUpper(strings.TrimSpace(valueString(data["billing_currency"])))
			if billingCurrency != "" && !validCurrency(billingCurrency) {
				return nil, fmt.Errorf("profile %q billing_currency must be a three-letter currency code", profileID)
			}
			fallbackProfile := strings.TrimSpace(valueString(data["fallback_profile"]))
			if fallbackProfile == "" {
				// Accept the more verbose spelling when reading hand-edited
				// registries, but always write the documented short spelling.
				fallbackProfile = strings.TrimSpace(valueString(data["local_fallback_profile"]))
			}
			fallbackTimeout, fallbackTimeoutSet, err := valueInt(data, "fallback_timeout_seconds")
			if err != nil {
				return nil, fmt.Errorf("profile %q fallback_timeout_seconds: %w", profileID, err)
			}
			if fallbackTimeoutSet && fallbackTimeout <= 0 {
				return nil, fmt.Errorf("profile %q fallback_timeout_seconds must be a positive whole number", profileID)
			}
			if strings.EqualFold(engine, "openai") {
				if !billingPriceSet && strings.EqualFold(model, "gpt-transcribe") {
					billingPrice = defaultGPTTranscribePricePerMinute
				}
				if billingCurrency == "" && strings.EqualFold(model, "gpt-transcribe") {
					billingCurrency = defaultGPTTranscribeCurrency
				}
				if !fallbackTimeoutSet {
					fallbackTimeout = defaultOpenAIFallbackTimeoutSeconds
				}
			}
			if billingPrice > 0 && billingCurrency == "" {
				return nil, fmt.Errorf("profile %q billing_currency is required when billing_price_per_minute is set", profileID)
			}
			profiles[profileID] = Profile{
				ID:                     profileID,
				Engine:                 engine,
				Model:                  model,
				Language:               language,
				Command:                command,
				Prompt:                 valueString(data["prompt"]),
				APIKeyFile:             apiKeyFile,
				BillingPricePerMinute:  billingPrice,
				BillingCurrency:        billingCurrency,
				FallbackProfile:        fallbackProfile,
				FallbackTimeoutSeconds: fallbackTimeout,
			}
		}
	}
	for _, profile := range profiles {
		if !strings.EqualFold(strings.TrimSpace(profile.Engine), "openai") {
			continue
		}
		explicit := strings.TrimSpace(profile.FallbackProfile)
		if explicit == "" {
			continue
		}
		candidate, ok := profiles[explicit]
		if !ok {
			return nil, fmt.Errorf("fallback profile %q for %q is not configured", explicit, profile.ID)
		}
		if !isWhisperProfile(candidate) {
			return nil, fmt.Errorf("fallback profile %q for %q must use whisper.cpp", explicit, profile.ID)
		}
	}
	if len(profiles) == 0 && strings.TrimSpace(cfg.TranscriptionCommand) != "" {
		legacy := Profile{
			ID:       "default",
			Engine:   cfg.TranscriptionEngine,
			Model:    cfg.Model,
			Language: cfg.Language,
			Command:  cfg.TranscriptionCommand,
		}
		if err := validateProfile(&legacy); err != nil {
			return nil, err
		}
		profiles["default"] = legacy
	}
	return profiles, nil
}

func Active(cfg config.Config) (Profile, error) {
	profiles, err := Load(cfg)
	if err != nil {
		return Profile{}, err
	}
	if len(profiles) == 0 {
		return Profile{}, fmt.Errorf("no transcription model configured; add a profile to %s or set transcription_command in %s", cfg.ModelsPath, cfg.ConfigPath)
	}
	if profile, ok := profiles[cfg.SelectedModel]; ok {
		return profile, nil
	}
	if profile, ok := profiles["default"]; ok {
		return profile, nil
	}
	ids := make([]string, 0, len(profiles))
	for id := range profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return profiles[ids[0]], nil
}

func Write(cfg config.Config, profile Profile, selectProfile bool) error {
	profiles, err := Load(cfg)
	if err != nil {
		return err
	}
	if profile.ID == "" {
		return fmt.Errorf("model profile id cannot be empty")
	}
	if err := validateProfile(&profile); err != nil {
		return err
	}
	profiles[profile.ID] = profile
	ids := make([]string, 0, len(profiles))
	for id := range profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var output strings.Builder
	output.WriteString(strings.TrimRight(DefaultModels, "\n"))
	output.WriteString("\n\n")
	for _, id := range ids {
		item := profiles[id]
		output.WriteString("[models.")
		output.WriteString(id)
		output.WriteString("]\n")
		output.WriteString("engine = ")
		output.WriteString(strconv.Quote(item.Engine))
		output.WriteString("\nmodel = ")
		output.WriteString(strconv.Quote(item.Model))
		output.WriteString("\nlanguage = ")
		output.WriteString(strconv.Quote(item.Language))
		if item.Command != "" {
			output.WriteString("\ncommand = ")
			output.WriteString(strconv.Quote(item.Command))
			output.WriteString("\n")
		} else {
			output.WriteString("\n")
		}
		if item.APIKeyFile != "" {
			output.WriteString("api_key_file = ")
			output.WriteString(strconv.Quote(item.APIKeyFile))
			output.WriteString("\n")
		}
		if strings.EqualFold(item.Engine, "openai") && item.BillingPricePerMinute > 0 {
			output.WriteString("billing_price_per_minute = ")
			output.WriteString(strconv.FormatFloat(item.BillingPricePerMinute, 'f', -1, 64))
			output.WriteString("\n")
		}
		if item.BillingCurrency != "" {
			output.WriteString("billing_currency = ")
			output.WriteString(strconv.Quote(item.BillingCurrency))
			output.WriteString("\n")
		}
		fallbackProfile := item.FallbackProfile
		if fallbackProfile != "" {
			output.WriteString("fallback_profile = ")
			output.WriteString(strconv.Quote(fallbackProfile))
			output.WriteString("\n")
		}
		if strings.EqualFold(item.Engine, "openai") && item.FallbackTimeoutSeconds > 0 {
			output.WriteString("fallback_timeout_seconds = ")
			output.WriteString(strconv.Itoa(item.FallbackTimeoutSeconds))
			output.WriteString("\n")
		}
		if item.Prompt != "" {
			output.WriteString("prompt = ")
			output.WriteString(strconv.Quote(item.Prompt))
			output.WriteString("\n")
		}
		output.WriteString("\n")
	}
	if err := os.WriteFile(cfg.ModelsPath, []byte(strings.TrimRight(output.String(), "\n")+"\n"), 0o644); err != nil {
		return err
	}
	if selectProfile {
		return config.UpdateConfigValueAt(cfg.ConfigPath, "selected_model", profile.ID)
	}
	return nil
}

// SelectFallback chooses the local profile to use when an OpenAI request
// fails. An empty profile and nil error mean that no local profile exists.
// voiceTriggerProfile is passed explicitly so this package does not need to
// own the application configuration policy.
func SelectFallback(primary Profile, profiles map[string]Profile, voiceTriggerProfile string) (Profile, error) {
	if !strings.EqualFold(strings.TrimSpace(primary.Engine), "openai") {
		return Profile{}, nil
	}
	explicit := strings.TrimSpace(primary.FallbackProfile)
	if explicit != "" {
		candidate, ok := profiles[explicit]
		if !ok {
			return Profile{}, fmt.Errorf("fallback profile %q for %q is not configured", explicit, primary.ID)
		}
		if !isWhisperProfile(candidate) {
			return Profile{}, fmt.Errorf("fallback profile %q for %q must use whisper.cpp", explicit, primary.ID)
		}
		return candidate, nil
	}
	if candidate, ok := profiles["whispercpp-small-en"]; ok && isWhisperProfile(candidate) {
		return candidate, nil
	}
	ids := sortedWhisperProfiles(profiles, func(profile Profile) bool {
		return strings.EqualFold(strings.TrimSpace(profile.Model), "small.en")
	})
	if len(ids) > 0 {
		return profiles[ids[0]], nil
	}
	if candidate, ok := profiles[strings.TrimSpace(voiceTriggerProfile)]; ok && isWhisperProfile(candidate) {
		return candidate, nil
	}
	ids = sortedWhisperProfiles(profiles, nil)
	if len(ids) == 0 {
		return Profile{}, nil
	}
	return profiles[ids[0]], nil
}

func isWhisperProfile(profile Profile) bool {
	return strings.EqualFold(strings.TrimSpace(profile.Engine), "whisper.cpp")
}

func sortedWhisperProfiles(profiles map[string]Profile, match func(Profile) bool) []string {
	ids := make([]string, 0, len(profiles))
	for id, profile := range profiles {
		if isWhisperProfile(profile) && (match == nil || match(profile)) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func validateProfile(profile *Profile) error {
	if profile.BillingPricePerMinute < 0 || math.IsNaN(profile.BillingPricePerMinute) || math.IsInf(profile.BillingPricePerMinute, 0) {
		return fmt.Errorf("profile %q billing_price_per_minute must be a finite non-negative number", profile.ID)
	}
	profile.BillingCurrency = strings.ToUpper(strings.TrimSpace(profile.BillingCurrency))
	if profile.BillingCurrency != "" && !validCurrency(profile.BillingCurrency) {
		return fmt.Errorf("profile %q billing_currency must be a three-letter currency code", profile.ID)
	}
	if profile.BillingPricePerMinute > 0 && profile.BillingCurrency == "" {
		return fmt.Errorf("profile %q billing_currency is required when billing_price_per_minute is set", profile.ID)
	}
	if profile.FallbackTimeoutSeconds < 0 {
		return fmt.Errorf("profile %q fallback_timeout_seconds must not be negative", profile.ID)
	}
	if strings.EqualFold(profile.Engine, "openai") {
		if profile.Model == "" {
			profile.Model = profile.ID
		}
		if profile.BillingPricePerMinute == 0 && strings.EqualFold(profile.Model, "gpt-transcribe") {
			profile.BillingPricePerMinute = defaultGPTTranscribePricePerMinute
		}
		if profile.BillingCurrency == "" && strings.EqualFold(profile.Model, "gpt-transcribe") {
			profile.BillingCurrency = defaultGPTTranscribeCurrency
		}
		if profile.FallbackTimeoutSeconds == 0 {
			profile.FallbackTimeoutSeconds = defaultOpenAIFallbackTimeoutSeconds
		}
	}
	profile.FallbackProfile = strings.TrimSpace(profile.FallbackProfile)
	return nil
}

func validCurrency(currency string) bool {
	if len(currency) != 3 {
		return false
	}
	for _, r := range currency {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func Select(profileID string) error {
	return config.UpdateConfigValue("selected_model", profileID)
}

func table(value any) (map[string]any, bool) {
	result, ok := value.(map[string]any)
	return result, ok
}

func valueString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func valueFloat(data map[string]any, key string) (float64, bool, error) {
	value, ok := data[key]
	if !ok {
		return 0, false, nil
	}
	switch number := value.(type) {
	case float64:
		return number, true, nil
	case float32:
		return float64(number), true, nil
	case int:
		return float64(number), true, nil
	case int8:
		return float64(number), true, nil
	case int16:
		return float64(number), true, nil
	case int32:
		return float64(number), true, nil
	case int64:
		return float64(number), true, nil
	case uint:
		return float64(number), true, nil
	case uint8:
		return float64(number), true, nil
	case uint16:
		return float64(number), true, nil
	case uint32:
		return float64(number), true, nil
	case uint64:
		return float64(number), true, nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(number), 64)
		if err != nil {
			return 0, true, fmt.Errorf("must be a number")
		}
		return parsed, true, nil
	default:
		return 0, true, fmt.Errorf("must be a number")
	}
}

func valueInt(data map[string]any, key string) (int, bool, error) {
	_, ok := data[key]
	if !ok {
		return 0, false, nil
	}
	number, numberSet, err := valueFloat(data, key)
	if err != nil {
		return 0, numberSet, err
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || number != math.Trunc(number) || number > float64(int(^uint(0)>>1)) || number < float64(-int(^uint(0)>>1)-1) {
		return 0, true, fmt.Errorf("must be a whole number")
	}
	return int(number), true, nil
}

// Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
