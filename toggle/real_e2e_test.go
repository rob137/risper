//go:build real_e2e

package toggle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/events"
	"github.com/rob137/risper/session"
)

// TestRealSystemAudioCycle deliberately crosses the real audio boundary. It
// plays a known recording through the default sink, captures that monitor with
// pw-record, mixes the source tracks with ffmpeg, and sends the mixed file to
// whisper.cpp. The microphone recorder is also started by the production
// workflow, but the assertion is made against the system track so ambient
// sound and microphone contents cannot supply the words.
func TestRealSystemAudioCycle(t *testing.T) {
	originalHome, err := os.UserHomeDir()
	if err != nil || originalHome == "" {
		t.Fatalf("could not determine the live home directory: %v", err)
	}

	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	stateHome := filepath.Join(root, "state")
	sessionsDir := filepath.Join(dataHome, config.AppName, "sessions")
	clipboardPath := filepath.Join(root, "clipboard.txt")

	for name, value := range map[string]string{
		"HOME":                 root,
		"XDG_CONFIG_HOME":      configHome,
		"XDG_DATA_HOME":        dataHome,
		"XDG_STATE_HOME":       stateHome,
		"XDG_SESSION_TYPE":     "wayland",
		"RISPER_E2E_CLIPBOARD": clipboardPath,
	} {
		t.Setenv(name, value)
	}
	originalPath := os.Getenv("PATH")
	writeRealE2EExecutable(t, filepath.Join(bin, "wl-copy"), "#!/bin/sh\ncat > \"$RISPER_E2E_CLIPBOARD\"\n")
	writeRealE2EExecutable(t, filepath.Join(bin, "notify-send"), "#!/bin/sh\nprintf '1\\n'\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+originalPath)

	for _, name := range []string{"pw-record", "ffmpeg", "pw-play"} {
		path, lookErr := exec.LookPath(name)
		if lookErr != nil {
			t.Fatalf("real_e2e requires %s: %v", name, lookErr)
		}
		if filepath.Dir(path) == bin {
			t.Fatalf("real_e2e resolved %s to its test helper %s", name, path)
		}
	}

	expectedWords := realE2EWords(os.Getenv("RISPER_E2E_EXPECTED_WORDS"))
	if len(expectedWords) == 0 {
		t.Fatal("RISPER_E2E_EXPECTED_WORDS must contain at least one word")
	}
	audioPath := os.Getenv("RISPER_E2E_AUDIO")
	if audioPath == "" {
		audioPath = findRealE2EAudio(originalHome, expectedWords)
	}
	if audioPath == "" {
		t.Fatalf("no known-good input found; set RISPER_E2E_AUDIO to a WAV and RISPER_E2E_EXPECTED_WORDS to words in its transcript")
	}
	if !filepath.IsAbs(audioPath) {
		audioPath, err = filepath.Abs(audioPath)
		if err != nil {
			t.Fatal(err)
		}
	}
	if info, statErr := os.Stat(audioPath); statErr != nil || info.Size() <= 44 {
		t.Fatalf("real_e2e input audio %s is unavailable or empty: %v", audioPath, statErr)
	}

	whisperCLI := os.Getenv("RISPER_E2E_WHISPER_CLI")
	if whisperCLI == "" {
		whisperCLI = filepath.Join(originalHome, ".local", "share", "risper", "engines", "whisper.cpp", "build", "bin", "whisper-cli")
	}
	modelPath := os.Getenv("RISPER_E2E_MODEL")
	if modelPath == "" {
		modelPath = filepath.Join(originalHome, ".local", "share", "risper", "engines", "whisper.cpp", "models", "ggml-small.en.bin")
	}
	assertExecutable(t, whisperCLI)
	assertRegularFile(t, modelPath)

	configPath := filepath.Join(configHome, config.AppName, "config.toml")
	modelsPath := filepath.Join(configHome, config.AppName, "models.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	configText := fmt.Sprintf("sessions_dir = %s\nselected_model = \"real-e2e\"\ntranscription_engine = \"whisper.cpp\"\nmodel = \"small.en\"\nlanguage = \"en\"\npaste_mode = \"clipboard_only\"\nplay_sounds = false\n", tomlString(sessionsDir))
	if err := os.WriteFile(configPath, []byte(configText), 0o644); err != nil {
		t.Fatal(err)
	}
	command := fmt.Sprintf("%s -m %s -f {audio} -l en -t 8 -nt -otxt -of {raw_no_txt}", shellQuote(whisperCLI), shellQuote(modelPath))
	modelsText := fmt.Sprintf("[models.real-e2e]\nengine = \"whisper.cpp\"\nmodel = \"small.en\"\nlanguage = \"en\"\ncommand = %s\n", tomlString(command))
	if err := os.WriteFile(modelsPath, []byte(modelsText), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	assertRealE2EIsolation(t, root, originalHome, cfg)

	fmt.Fprintln(os.Stderr, "WARNING: real_e2e will play audio through the default sink and make noise through the speakers.")
	active := false
	defer func() {
		if active {
			_ = Main(nil)
		}
	}()

	if code := Main([]string{"--system"}); code != 0 {
		t.Fatalf("real system-audio recording start returned %d", code)
	}
	active = true

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	play := exec.CommandContext(ctx, "pw-play", audioPath)
	play.Stdout = os.Stdout
	play.Stderr = os.Stderr
	if err := play.Run(); err != nil {
		cancel()
		t.Fatalf("playing known-good input through the default sink: %v", err)
	}
	cancel()
	time.Sleep(300 * time.Millisecond)

	code := Main(nil)
	active = false
	if code != 0 {
		t.Fatalf("real system-audio recording stop/transcription returned %d", code)
	}

	metadata, err := session.Last(cfg)
	if err != nil || metadata == nil {
		t.Fatalf("isolated last session = %#v, %v", metadata, err)
	}
	if metadata.Status != "complete" {
		t.Fatalf("real session status = %q, errors=%v", metadata.Status, metadata.Errors)
	}
	if metadata.AudioSources == nil || len(metadata.AudioSources) != 2 || metadata.AudioSources[0] != "mic" || metadata.AudioSources[1] != "system" {
		t.Fatalf("real session audio sources = %#v", metadata.AudioSources)
	}
	for _, path := range []string{metadata.AudioPath, metadata.AudioSourcePaths["mic"], metadata.AudioSourcePaths["system"], metadata.TranscriptRawPath, metadata.TranscriptCleanPath} {
		assertRegularFile(t, path)
	}

	transcript := readRealE2EText(t, metadata.TranscriptCleanPath)
	for _, word := range expectedWords {
		if !containsRealE2EWord(transcript, word) {
			t.Fatalf("real transcript %q does not contain known word %q", transcript, word)
		}
	}
	if got := readRealE2EText(t, clipboardPath); got != transcript {
		t.Fatalf("isolated clipboard = %q, want transcript %q", got, transcript)
	}

	records, err := events.Read(session.SessionDir(metadata), 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, record := range records {
		if event, ok := record["event"].(string); ok {
			seen[event] = true
		}
	}
	for _, required := range []string{"recorder.mixed", "transcription.completed", "clipboard.copy", "paste.skipped"} {
		if !seen[required] {
			t.Fatalf("real session missing event %s: %#v", required, records)
		}
	}
}

func findRealE2EAudio(home string, expectedWords []string) string {
	entries, err := os.ReadDir(filepath.Join(home, ".local", "share", "risper", "sessions"))
	if err != nil {
		return ""
	}
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		if !entry.IsDir() {
			continue
		}
		metadata, loadErr := session.LoadSession(filepath.Join(home, ".local", "share", "risper", "sessions", entry.Name()))
		if loadErr != nil || metadata == nil || metadata.Status != "complete" {
			continue
		}
		transcript, readErr := os.ReadFile(metadata.TranscriptCleanPath)
		if readErr != nil {
			continue
		}
		matches := true
		for _, word := range expectedWords {
			if !containsRealE2EWord(string(transcript), word) {
				matches = false
				break
			}
		}
		if matches {
			if info, statErr := os.Stat(metadata.AudioPath); statErr == nil && info.Size() > 44 {
				return metadata.AudioPath
			}
		}
	}
	return ""
}

func realE2EWords(value string) []string {
	if value == "" {
		value = "looking,list"
	}
	var words []string
	for _, raw := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		word := strings.ToLower(strings.TrimSpace(raw))
		if word != "" {
			words = append(words, word)
		}
	}
	return words
}

func containsRealE2EWord(text, wanted string) bool {
	wanted = strings.ToLower(strings.TrimSpace(wanted))
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if word == wanted {
			return true
		}
	}
	return false
}

func assertRealE2EIsolation(t *testing.T, root, originalHome string, cfg config.Config) {
	t.Helper()
	for name, path := range map[string]string{
		"config":   cfg.ConfigPath,
		"models":   cfg.ModelsPath,
		"sessions": cfg.SessionsDir,
		"state":    cfg.StateDir,
	} {
		if !pathWithin(root, path) {
			t.Fatalf("isolated %s path escaped test root: %s", name, path)
		}
	}
	liveConfig := filepath.Join(originalHome, ".config", config.AppName)
	liveSessions := filepath.Join(originalHome, ".local", "share", config.AppName, "sessions")
	if pathWithin(liveConfig, cfg.ConfigPath) || pathWithin(liveConfig, cfg.ModelsPath) || pathWithin(liveSessions, cfg.SessionsDir) {
		t.Fatalf("real_e2e resolved a write path into live Risper data: config=%s models=%s sessions=%s", cfg.ConfigPath, cfg.ModelsPath, cfg.SessionsDir)
	}
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func writeRealE2EExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertExecutable(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("real_e2e executable %s is unavailable: %v", path, err)
	}
}

func assertRegularFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		t.Fatalf("real_e2e file %s is unavailable or empty: %v", path, err)
	}
}

func readRealE2EText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func tomlString(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
