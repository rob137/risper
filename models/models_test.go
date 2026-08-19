package models

import (
	"os"
	"path/filepath"
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
	if err := Write(cfg, Profile{ID: "fast", Engine: "engine", Model: "m", Language: "en", Command: " /bin/echo fast ", Prompt: "names"}, true); err != nil {
		t.Fatal(err)
	}
	profiles, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if profiles["fast"].Command != "/bin/echo fast" || profiles["fast"].Prompt != "names" {
		t.Fatalf("unexpected profile: %#v", profiles["fast"])
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
