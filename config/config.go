package config

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/BurntSushi/toml"
	"github.com/rob137/risper/internal/files"
)

const AppName = "risper"

const defaultSessionsDir = "~/.local/share/risper/sessions"

const DefaultConfig = `# Risper user config.
# Paths support ~ expansion.
sessions_dir = "` + defaultSessionsDir + `"
selected_model = "default"
transcription_engine = "external"
transcription_command = ""
model = "base.en"
language = "en"
paste_mode = "clipboard_only" # the transcript is always left on the clipboard
paste_keys = "ctrl+v" # ydotool sequence used by shift double Alt; terminals usually want ctrl+shift+v
play_sounds = true
double_alt_enabled = false
double_alt_window_ms = 350
voice_triggers_enabled = false
voice_start_word = "quasar"
voice_stop_word = "marzipan"
voice_send_word = "tangerine"
voice_trigger_profile = "whispercpp-base-en"
voice_noise_gate_db = 10.0 # speech must exceed the rolling mic floor by this much
voice_silence_ms = 400 # silence that ends one trigger utterance
audio_retention = "never" # never | <count>h | <count>d | <count>w; transcripts are always kept
`

var allowedPasteModes = map[string]struct{}{"clipboard_only": {}}

const defaultPasteKeys = "ctrl+v"

// validPasteKeys keeps the configured sequence to the shape ydotool accepts,
// so a typo cannot turn into an unexpected argument.
func validPasteKeys(sequence string) bool {
	if sequence == "" {
		return false
	}
	for _, r := range sequence {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '+', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
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
	PasteKeys                string
	PlaySounds               bool
	DoubleAltEnabled         bool
	DoubleAltWindowMS        int
	VoiceTriggersEnabled     bool
	VoiceStartWord           string
	VoiceStopWord            string
	VoiceSendWord            string
	VoiceTriggerProfile      string
	VoiceNoiseGateDB         float64
	VoiceSilenceMS           int
	AudioRetentionSeconds    *float64
}

type rawConfig struct {
	SessionsDir          string  `toml:"sessions_dir"`
	SelectedModel        string  `toml:"selected_model"`
	TranscriptionEngine  string  `toml:"transcription_engine"`
	TranscriptionCommand string  `toml:"transcription_command"`
	Model                string  `toml:"model"`
	Language             string  `toml:"language"`
	PasteMode            string  `toml:"paste_mode"`
	PasteKeys            string  `toml:"paste_keys"`
	PlaySounds           bool    `toml:"play_sounds"`
	DoubleAltEnabled     bool    `toml:"double_alt_enabled"`
	DoubleAltWindowMS    int     `toml:"double_alt_window_ms"`
	VoiceTriggersEnabled bool    `toml:"voice_triggers_enabled"`
	VoiceStartWord       string  `toml:"voice_start_word"`
	VoiceStopWord        string  `toml:"voice_stop_word"`
	VoiceSendWord        string  `toml:"voice_send_word"`
	VoiceTriggerProfile  string  `toml:"voice_trigger_profile"`
	VoiceNoiseGateDB     float64 `toml:"voice_noise_gate_db"`
	VoiceSilenceMS       int     `toml:"voice_silence_ms"`
	AudioRetention       string  `toml:"audio_retention"`
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func requireTestIsolation() error {
	// A test process must not silently inherit any of Rob's XDG roots. The
	// individual package tests set all three explicitly; rejecting a missing
	// root makes a newly added test fail before it can create live data.
	if flag.Lookup("test.v") == nil {
		return nil
	}
	missing := make([]string, 0, 3)
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("test isolation is incomplete; set %s before loading Risper config", strings.Join(missing, ", "))
	}
	return nil
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
	home := homeDir()
	if strings.HasPrefix(path, "~") && home == "" {
		return ""
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func Load() (Config, error) {
	if err := requireTestIsolation(); err != nil {
		return Config{}, err
	}
	path, err := EnsureDefaultConfig()
	if err != nil {
		return Config{}, err
	}
	dataHome := DataHome()
	stateHome := StateHome()
	raw := rawConfig{
		SessionsDir:         filepath.Join(dataHome, AppName, "sessions"),
		SelectedModel:       "default",
		TranscriptionEngine: "external",
		Model:               "base.en",
		Language:            "en",
		PasteMode:           "clipboard_only",
		PasteKeys:           defaultPasteKeys,
		PlaySounds:          true,
		DoubleAltWindowMS:   350,
		VoiceStartWord:      "quasar",
		VoiceStopWord:       "marzipan",
		VoiceSendWord:       "tangerine",
		VoiceTriggerProfile: "whispercpp-base-en",
		VoiceNoiseGateDB:    10,
		VoiceSilenceMS:      400,
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
	pasteKeys := raw.PasteKeys
	if !validPasteKeys(pasteKeys) {
		pasteKeys = defaultPasteKeys
	}
	window := raw.DoubleAltWindowMS
	if window < 100 {
		window = 100
	}
	startWord, stopWord, sendWord := normalizedVoiceWords(raw.VoiceStartWord, raw.VoiceStopWord, raw.VoiceSendWord)
	voiceProfile := strings.TrimSpace(raw.VoiceTriggerProfile)
	if voiceProfile == "" {
		voiceProfile = "whispercpp-base-en"
	}
	noiseGateDB := raw.VoiceNoiseGateDB
	if noiseGateDB < 3 || noiseGateDB > 40 {
		noiseGateDB = 10
	}
	voiceSilenceMS := raw.VoiceSilenceMS
	if voiceSilenceMS < 200 {
		voiceSilenceMS = 200
	} else if voiceSilenceMS > 1500 {
		voiceSilenceMS = 1500
	}
	retention, hasRetention := ParseAudioRetention(raw.AudioRetention)

	dataDir := filepath.Join(dataHome, AppName)
	stateDir := filepath.Join(stateHome, AppName)
	sessionsDir := raw.SessionsDir
	if sessionsDir == "" || sessionsDir == defaultSessionsDir {
		sessionsDir = filepath.Join(dataHome, AppName, "sessions")
	} else {
		sessionsDir = expandHome(sessionsDir)
		if sessionsDir == "" && strings.HasPrefix(raw.SessionsDir, "~") {
			return Config{}, fmt.Errorf("cannot expand sessions_dir: HOME is not set")
		}
	}
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
		PasteKeys:                pasteKeys,
		PlaySounds:               raw.PlaySounds,
		DoubleAltEnabled:         raw.DoubleAltEnabled,
		DoubleAltWindowMS:        window,
		VoiceTriggersEnabled:     raw.VoiceTriggersEnabled,
		VoiceStartWord:           startWord,
		VoiceStopWord:            stopWord,
		VoiceSendWord:            sendWord,
		VoiceTriggerProfile:      voiceProfile,
		VoiceNoiseGateDB:         noiseGateDB,
		VoiceSilenceMS:           voiceSilenceMS,
		AudioRetentionSeconds:    retentionPtr,
	}, nil
}

func normalizedVoiceWords(start, stop, send string) (string, string, string) {
	roleDefaults := []string{"quasar", "marzipan", "tangerine"}
	fallbacks := []string{"quasar", "marzipan", "tangerine", "blueberry", "cinnamon"}
	values := []string{start, stop, send}
	used := make(map[string]struct{}, len(values))
	for i, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if !validVoiceWord(value) {
			value = ""
		}
		if _, exists := used[value]; value == "" || exists {
			if _, exists := used[roleDefaults[i]]; !exists {
				value = roleDefaults[i]
			} else {
				for _, fallback := range fallbacks {
					if _, exists := used[fallback]; !exists {
						value = fallback
						break
					}
				}
			}
		}
		if value == "" {
			for _, fallback := range fallbacks {
				if _, exists := used[fallback]; !exists {
					value = fallback
					break
				}
			}
		}
		values[i] = value
		used[value] = struct{}{}
	}
	return values[0], values[1], values[2]
}

func validVoiceWord(value string) bool {
	if len(strings.Fields(value)) != 1 || value == "" {
		return false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
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
