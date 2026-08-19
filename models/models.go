// Package models owns the profile registry in models.toml.
package models

import (
	"fmt"
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
# command = "/path/to/whisper-cli -m /path/to/model.bin -f {audio} -l {language} --prompt \"{prompt}\" -nt -otxt -of {raw_no_txt}"
#
# An optional ` + "`prompt`" + ` biases decoding toward the words it lists (proper nouns,
# names, jargon). It is rendered into the command's {prompt} placeholder. Keep it
# a short comma list, not a paragraph. Only whisper.cpp uses it.
`

type Profile struct {
	ID       string
	Engine   string
	Model    string
	Language string
	Command  string
	Prompt   string
}

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
			if command == "" {
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
			engine := valueString(data["engine"])
			if engine == "" {
				engine = "external"
			}
			profiles[profileID] = Profile{
				ID:       profileID,
				Engine:   engine,
				Model:    model,
				Language: language,
				Command:  command,
				Prompt:   valueString(data["prompt"]),
			}
		}
	}
	if len(profiles) == 0 && strings.TrimSpace(cfg.TranscriptionCommand) != "" {
		profiles["default"] = Profile{
			ID:       "default",
			Engine:   cfg.TranscriptionEngine,
			Model:    cfg.Model,
			Language: cfg.Language,
			Command:  cfg.TranscriptionCommand,
		}
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
		output.WriteString("\ncommand = ")
		output.WriteString(strconv.Quote(item.Command))
		output.WriteString("\n")
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
