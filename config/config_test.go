package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAudioRetention(t *testing.T) {
	tests := []struct {
		value   string
		seconds float64
		valid   bool
	}{
		{"12h", 12 * 3600, true},
		{"7D", 7 * 86400, true},
		{"0.5d", 12 * 3600, true},
		{"never", 0, false},
		{"7", 0, false},
		{"0d", 0, false},
		{"-3d", 0, false},
	}
	for _, test := range tests {
		seconds, valid := ParseAudioRetention(test.value)
		if valid != test.valid || valid && seconds != test.seconds {
			t.Errorf("ParseAudioRetention(%q) = (%v, %v), want (%v, %v)", test.value, seconds, valid, test.seconds, test.valid)
		}
	}
}

func TestCommandExistsUsesPath(t *testing.T) {
	dir := t.TempDir()
	command := filepath.Join(dir, "available")
	if err := os.WriteFile(command, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if !CommandExists("available") {
		t.Fatal("expected executable on PATH to be found")
	}
	if CommandExists("missing") {
		t.Fatal("did not expect missing executable to be found")
	}
}

func TestLoadUsesDefaultsAndCreatesDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Model != "base.en" || loaded.Language != "en" || loaded.PasteMode != "clipboard_only" {
		t.Fatalf("unexpected defaults: %+v", loaded)
	}
	if loaded.AudioRetentionSeconds != nil {
		t.Fatalf("default retention should be unlimited: %v", *loaded.AudioRetentionSeconds)
	}
	for _, path := range []string{loaded.ConfigPath, loaded.DataDir, loaded.SessionsDir, loaded.StateDir} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to exist: %v", path, err)
		}
	}
	contents, err := os.ReadFile(loaded.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != DefaultConfig {
		t.Fatalf("default config differs from template:\n%s", contents)
	}
}

func TestLoadNormalizesCompatibilityValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	path, err := EnsureDefaultConfig()
	if err != nil {
		t.Fatal(err)
	}
	customSessions := filepath.Join(home, "custom-sessions")
	if err := os.WriteFile(path, []byte("sessions_dir = \"~/custom-sessions\"\npaste_mode = \"not-real\"\ndouble_alt_window_ms = 20\naudio_retention = \"7d\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SessionsDir != customSessions || loaded.PasteMode != "clipboard_only" || !loaded.PlaySounds || loaded.DoubleAltWindowMS != 100 {
		t.Fatalf("unexpected normalized config: %+v", loaded)
	}
	if loaded.AudioRetentionSeconds == nil || *loaded.AudioRetentionSeconds != 7*86400 {
		t.Fatalf("unexpected retention: %v", loaded.AudioRetentionSeconds)
	}
}

func TestUpdateConfigValuePreservesCommentsAndQuotes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	path, err := EnsureDefaultConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateConfigValue("selected_model", `name "with" quotes`); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if !strings.Contains(text, "selected_model = \"name \\\"with\\\" quotes\"") || !strings.Contains(text, "# Paths support ~ expansion.") {
		t.Fatalf("config update did not preserve expected text:\n%s", text)
	}
}
