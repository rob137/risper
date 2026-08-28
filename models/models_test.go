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

// Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
