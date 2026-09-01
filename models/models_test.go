package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestWriteAndSelectProfile(t *testing.T) {
	cfg := testConfig(t)
	if err := Write(cfg, Profile{ID: "fast", Engine: "engine", Model: "m", Language: "en", Command: " /bin/echo fast ", Prompt: "names", APIKeyFile: "~/.config/openai/key"}, true); err != nil {
		t.Fatal(err)
	}
	profiles, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if profiles["fast"].Command != "/bin/echo fast" || profiles["fast"].Prompt != "names" || profiles["fast"].APIKeyFile != "~/.config/openai/key" {
		t.Fatalf("unexpected profile: %#v", profiles["fast"])
	}
	contents, err := os.ReadFile(cfg.ModelsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), `api_key_file = "~/.config/openai/key"`) {
		t.Fatalf("written profile omitted api_key_file: %s", contents)
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.SelectedModel != "fast" {
		t.Fatalf("selected model = %q", reloaded.SelectedModel)
	}
	active, err := Active(reloaded)
	if err != nil || active.ID != "fast" {
		t.Fatalf("active profile = %#v, %v", active, err)
	}
}

func TestWriteCommandlessOpenAIProfile(t *testing.T) {
	cfg := testConfig(t)
	profile := Profile{
		ID: "cloud", Engine: "openai", Model: "gpt-transcribe", Language: "en",
		APIKeyFile: "~/.config/openai/key", Prompt: "proper nouns",
	}
	if err := Write(cfg, profile, false); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(cfg.ModelsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), `command = ""`) {
		t.Fatalf("commandless OpenAI profile serialized an empty command: %s", contents)
	}
	profiles, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := profiles["cloud"]; got.Engine != profile.Engine || got.Model != profile.Model || got.APIKeyFile != profile.APIKeyFile || got.Prompt != profile.Prompt {
		t.Fatalf("loaded OpenAI profile = %#v, want %#v", got, profile)
	}
}

func TestDefaultModelsEndsWithProvenanceFooter(t *testing.T) {
	const footer = "# Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby\n"
	if !strings.HasSuffix(DefaultModels, footer) {
		t.Fatalf("default models file does not end with provenance footer: %q", DefaultModels)
	}
}

func TestLoadOpenAIProfileMayOmitCommandAndDefaultsKeyFile(t *testing.T) {
	cfg := testConfig(t)
	if err := os.WriteFile(cfg.ModelsPath, []byte("[models.openai]\nengine = \"openai\"\nmodel = \"gpt-transcribe\"\nlanguage = \"en\"\nprompt = \"proper nouns\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profiles, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := profiles["openai"]
	if !ok || profile.Command != "" || profile.Engine != "openai" || profile.APIKeyFile != "~/.config/openai/key" || profile.Prompt != "proper nouns" {
		t.Fatalf("openai profile = %#v, want commandless profile with default key path", profile)
	}
}

func TestLoadSkipsNonOpenAIProfileWithoutCommand(t *testing.T) {
	cfg := testConfig(t)
	if err := os.WriteFile(cfg.ModelsPath, []byte("[models.missing]\nengine = \"external\"\nmodel = \"m\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profiles, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Fatalf("profiles = %#v, want commandless external profile skipped", profiles)
	}
}

func TestLoadSkipsInvalidProfilesAndUsesLegacyCommand(t *testing.T) {
	cfg := testConfig(t)
	if err := os.WriteFile(cfg.ModelsPath, []byte("[models]\nbroken = \"not a table\"\n\n[models.valid]\ncommand = \"  /bin/echo ok  \"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profiles, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles["valid"].Command != "/bin/echo ok" || profiles["valid"].Language != "en" {
		t.Fatalf("profiles = %#v", profiles)
	}

	if err := os.WriteFile(cfg.ModelsPath, []byte("# no profiles\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.ConfigPath, []byte("transcription_engine = \"legacy\"\ntranscription_command = \"/bin/echo legacy\"\nmodel = \"legacy-model\"\nlanguage = \"cy\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyCfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	active, err := Active(legacyCfg)
	if err != nil || active.ID != "default" || active.Engine != "legacy" || active.Model != "legacy-model" || active.Language != "cy" {
		t.Fatalf("legacy active profile = %#v, %v", active, err)
	}
}

func TestActiveProfileFallbackOrder(t *testing.T) {
	cfg := testConfig(t)
	if err := os.WriteFile(cfg.ModelsPath, []byte("[models.zzz]\ncommand = \"/bin/echo z\"\n\n[models.aaa]\ncommand = \"/bin/echo a\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	active, err := Active(cfg)
	if err != nil || active.ID != "aaa" {
		t.Fatalf("sorted fallback = %#v, %v", active, err)
	}
	if err := os.WriteFile(cfg.ModelsPath, []byte("[models.zzz]\ncommand = \"/bin/echo z\"\n\n[models.default]\ncommand = \"/bin/echo default\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	active, err = Active(cfg)
	if err != nil || active.ID != "default" {
		t.Fatalf("default fallback = %#v, %v", active, err)
	}
}

func TestOpenAIProfileDefaultsAndRoundTripBillingFallbackFields(t *testing.T) {
	cfg := testConfig(t)
	if err := os.WriteFile(cfg.ModelsPath, []byte(`[models.cloud]
engine = "openai"
model = "gpt-transcribe"
language = "en"
fallback_profile = "whispercpp-small-en"

[models.whispercpp-small-en]
engine = "whisper.cpp"
model = "small.en"
command = "/bin/echo {audio}"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	profiles, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cloud := profiles["cloud"]
	if cloud.BillingPricePerMinute != 0.0045 || cloud.BillingCurrency != "USD" || cloud.FallbackTimeoutSeconds != 15 || cloud.FallbackProfile != "whispercpp-small-en" {
		t.Fatalf("defaults = %#v", cloud)
	}
	if err := Write(cfg, cloud, false); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded["cloud"]; got.BillingPricePerMinute != 0.0045 || got.BillingCurrency != "USD" || got.FallbackTimeoutSeconds != 15 || got.FallbackProfile != "whispercpp-small-en" {
		t.Fatalf("round-tripped profile = %#v", got)
	}
	contents, err := os.ReadFile(cfg.ModelsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`billing_price_per_minute = 0.0045`, `billing_currency = "USD"`, `fallback_profile = "whispercpp-small-en"`, `fallback_timeout_seconds = 15`} {
		if !strings.Contains(string(contents), field) {
			t.Fatalf("written profile omitted %q: %s", field, contents)
		}
	}
}

func TestUnknownOpenAIModelDoesNotInventBillingPrice(t *testing.T) {
	cfg := testConfig(t)
	if err := os.WriteFile(cfg.ModelsPath, []byte(`[models.cloud]
engine = "openai"
model = "future-model"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	profiles, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cloud := profiles["cloud"]
	if cloud.BillingPricePerMinute != 0 || cloud.BillingCurrency != "" || cloud.FallbackTimeoutSeconds != 15 {
		t.Fatalf("unknown-model defaults = %#v", cloud)
	}
}

func TestSelectFallbackPolicy(t *testing.T) {
	primary := Profile{ID: "cloud", Engine: "openai"}
	profiles := map[string]Profile{
		"zzz-small": {ID: "zzz-small", Engine: "whisper.cpp", Model: "small.en"},
		"aaa-small": {ID: "aaa-small", Engine: "whisper.cpp", Model: "small.en"},
		"voice":     {ID: "voice", Engine: "whisper.cpp", Model: "base.en"},
		"remote":    {ID: "remote", Engine: "openai", Model: "other"},
	}
	if got, err := SelectFallback(primary, profiles, "voice"); err != nil || got.ID != "aaa-small" {
		t.Fatalf("sorted small fallback = %#v, %v", got, err)
	}
	profiles["whispercpp-small-en"] = Profile{ID: "whispercpp-small-en", Engine: "whisper.cpp", Model: "small.en"}
	if got, err := SelectFallback(primary, profiles, "voice"); err != nil || got.ID != "whispercpp-small-en" {
		t.Fatalf("canonical small fallback = %#v, %v", got, err)
	}
	primary.FallbackProfile = "voice"
	if got, err := SelectFallback(primary, profiles, "voice"); err != nil || got.ID != "voice" {
		t.Fatalf("explicit fallback = %#v, %v", got, err)
	}
	primary.FallbackProfile = "remote"
	if _, err := SelectFallback(primary, profiles, "voice"); err == nil || !strings.Contains(err.Error(), "must use whisper.cpp") {
		t.Fatalf("invalid explicit fallback error = %v", err)
	}
	primary.FallbackProfile = "missing"
	if _, err := SelectFallback(primary, profiles, "voice"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("missing explicit fallback error = %v", err)
	}
	if got, err := SelectFallback(Profile{Engine: "whisper.cpp"}, profiles, "voice"); err != nil || got.ID != "" {
		t.Fatalf("non-cloud fallback = %#v, %v", got, err)
	}
}

func TestSelectFallbackVoiceThenSortedAny(t *testing.T) {
	primary := Profile{ID: "cloud", Engine: "openai"}
	profiles := map[string]Profile{
		"z-base": {ID: "z-base", Engine: "whisper.cpp", Model: "base.en"},
		"a-base": {ID: "a-base", Engine: "whisper.cpp", Model: "base.en"},
		"voice":  {ID: "voice", Engine: "whisper.cpp", Model: "tiny.en"},
	}
	if got, err := SelectFallback(primary, profiles, "voice"); err != nil || got.ID != "voice" {
		t.Fatalf("voice fallback = %#v, %v", got, err)
	}
	delete(profiles, "voice")
	if got, err := SelectFallback(primary, profiles, "voice"); err != nil || got.ID != "a-base" {
		t.Fatalf("sorted whisper fallback = %#v, %v", got, err)
	}
}

func TestLoadRejectsInvalidBillingAndFallbackTimeout(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"negative price", "[models.cloud]\nengine = \"openai\"\nmodel = \"x\"\nbilling_price_per_minute = -1\n", "billing_price_per_minute must be"},
		{"currency without price", "[models.cloud]\nengine = \"openai\"\nmodel = \"x\"\nbilling_price_per_minute = 1\n", "billing_currency is required"},
		{"negative timeout", "[models.cloud]\nengine = \"openai\"\nmodel = \"x\"\nfallback_timeout_seconds = -1\n", "fallback_timeout_seconds must be a positive whole number"},
		{"zero timeout", "[models.cloud]\nengine = \"openai\"\nmodel = \"x\"\nfallback_timeout_seconds = 0\n", "fallback_timeout_seconds must be a positive whole number"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testConfig(t)
			if err := os.WriteFile(cfg.ModelsPath, []byte(test.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(cfg); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load error = %v, want text %q", err, test.want)
			}
		})
	}
}

func TestLoadRejectsInvalidExplicitFallback(t *testing.T) {
	cfg := testConfig(t)
	if err := os.WriteFile(cfg.ModelsPath, []byte(`[models.cloud]
engine = "openai"
model = "future-model"
fallback_profile = "remote"

[models.remote]
engine = "openai"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(cfg); err == nil || !strings.Contains(err.Error(), `fallback profile "remote" for "cloud" must use whisper.cpp`) {
		t.Fatalf("Load error = %v", err)
	}
}

// Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
