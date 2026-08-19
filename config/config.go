package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/rob137/risper/internal/files"
)

const AppName = "risper"

const DefaultConfig = `# Risper user config.
# Paths support ~ expansion.
sessions_dir = "~/.local/share/risper/sessions"
selected_model = "default"
transcription_engine = "external"
transcription_command = ""
model = "base.en"
language = "en"
paste_mode = "clipboard_only" # clipboard_only | auto | xdotool | wtype | ydotool | dotool
play_sounds = true
double_alt_enabled = false
double_alt_window_ms = 350
audio_retention = "never" # never | <count>h | <count>d | <count>w; transcripts are always kept
`

var allowedPasteModes = map[string]struct{}{
	"auto": {}, "clipboard_only": {}, "xdotool": {}, "wtype": {}, "ydotool": {}, "dotool": {},
}

type Config struct {
	ConfigPath               string
	ModelsPath               string
	DataDir                  string
	SessionsDir              string
	StateDir                 string
	CurrentStatePath         string
	CurrentTranscriptionPath string
	LogPath                  string
	SelectedModel            string
	TranscriptionEngine      string
	TranscriptionCommand     string
	Model                    string
	Language                 string
	PasteMode                string
	PlaySounds               bool
	DoubleAltEnabled         bool
	DoubleAltWindowMS        int
	AudioRetentionSeconds    *float64
}

type rawConfig struct {
	SessionsDir          string `toml:"sessions_dir"`
	SelectedModel        string `toml:"selected_model"`
	TranscriptionEngine  string `toml:"transcription_engine"`
	TranscriptionCommand string `toml:"transcription_command"`
	Model                string `toml:"model"`
	Language             string `toml:"language"`
	PasteMode            string `toml:"paste_mode"`
	PlaySounds           bool   `toml:"play_sounds"`
	DoubleAltEnabled     bool   `toml:"double_alt_enabled"`
	DoubleAltWindowMS    int    `toml:"double_alt_window_ms"`
	AudioRetention       string `toml:"audio_retention"`
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

func xdgDir(environment, fallback string) string {
	if value := os.Getenv(environment); value != "" {
		return filepath.Clean(value)
	}
	return filepath.Join(homeDir(), fallback)
}

func ConfigHome() string { return xdgDir("XDG_CONFIG_HOME", ".config") }
func DataHome() string   { return xdgDir("XDG_DATA_HOME", filepath.Join(".local", "share")) }
func StateHome() string  { return xdgDir("XDG_STATE_HOME", filepath.Join(".local", "state")) }

func ConfigPath() string { return filepath.Join(ConfigHome(), AppName, "config.toml") }
func ModelsPath() string { return filepath.Join(ConfigHome(), AppName, "models.toml") }

func CommandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func EnsureDefaultConfig() (string, error) {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := files.AtomicWriteText(path, DefaultConfig); err != nil {
		return "", err
	}
	return path, nil
}

func ParseAudioRetention(value string) (float64, bool) {
	text := strings.ToLower(strings.TrimSpace(value))
	if len(text) < 2 {
		return 0, false
	}
	units := map[byte]float64{'h': 3600, 'd': 86400, 'w': 604800}
	multiplier, ok := units[text[len(text)-1]]
	if !ok {
		return 0, false
	}
	count, err := strconv.ParseFloat(text[:len(text)-1], 64)
	if err != nil || count <= 0 {
		return 0, false
	}
	return count * multiplier, true
}

func expandHome(path string) string {
	if path == "~" {
		return homeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir(), path[2:])
	}
	return path
}

func Load() (Config, error) {
	path, err := EnsureDefaultConfig()
	if err != nil {
		return Config{}, err
	}
	raw := rawConfig{
		SessionsDir:         filepath.Join(DataHome(), AppName, "sessions"),
		SelectedModel:       "default",
		TranscriptionEngine: "external",
		Model:               "base.en",
		Language:            "en",
		PasteMode:           "clipboard_only",
		PlaySounds:          true,
		DoubleAltWindowMS:   350,
		AudioRetention:      "never",
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return Config{}, fmt.Errorf("decode config %s: %w", path, err)
	}

	pasteMode := raw.PasteMode
	if _, ok := allowedPasteModes[pasteMode]; !ok {
		pasteMode = "clipboard_only"
	}
	window := raw.DoubleAltWindowMS
	if window < 100 {
		window = 100
	}
	retention, hasRetention := ParseAudioRetention(raw.AudioRetention)

	dataDir := filepath.Join(DataHome(), AppName)
	stateDir := filepath.Join(StateHome(), AppName)
	sessionsDir := expandHome(raw.SessionsDir)
	for _, dir := range []string{dataDir, stateDir, sessionsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Config{}, err
		}
	}

	var retentionPtr *float64
	if hasRetention {
		retentionPtr = &retention
	}
	return Config{
		ConfigPath:               path,
		ModelsPath:               ModelsPath(),
		DataDir:                  dataDir,
		SessionsDir:              sessionsDir,
		StateDir:                 stateDir,
		CurrentStatePath:         filepath.Join(stateDir, "current.json"),
		CurrentTranscriptionPath: filepath.Join(stateDir, "current-transcription.json"),
		LogPath:                  filepath.Join(stateDir, "risper.log"),
		SelectedModel:            raw.SelectedModel,
		TranscriptionEngine:      raw.TranscriptionEngine,
		TranscriptionCommand:     raw.TranscriptionCommand,
		Model:                    raw.Model,
		Language:                 raw.Language,
		PasteMode:                pasteMode,
		PlaySounds:               raw.PlaySounds,
		DoubleAltEnabled:         raw.DoubleAltEnabled,
		DoubleAltWindowMS:        window,
		AudioRetentionSeconds:    retentionPtr,
	}, nil
}

func UpdateConfigValue(key, value string) error {
	path, err := EnsureDefaultConfig()
	if err != nil {
		return err
	}
	return UpdateConfigValueAt(path, key, value)
}

// UpdateConfigValueAt is the path-explicit form used by callers that already
// have a loaded Config, which also makes temporary configurations easy to test.
func UpdateConfigValueAt(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	replacement := key + " = " + strconv.Quote(value)
	found := false
	for i, line := range lines {
		if strings.TrimSpace(strings.SplitN(line, "=", 2)[0]) == key {
			lines[i] = replacement
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, replacement)
	}
	return files.AtomicWriteText(path, strings.TrimRight(strings.Join(lines, "\n"), "\n")+"\n")
}

// UpdateValue is the short form used by the profile package.
func UpdateValue(key, value string) error {
	return UpdateConfigValue(key, value)
}
