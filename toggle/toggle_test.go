package toggle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/events"
	"github.com/rob137/risper/session"
)

// stubEnvironment installs fake desktop and engine commands on PATH and
// isolates every XDG root, so a toggle run exercises the real code path
// without touching Rob's sessions.
func stubEnvironment(t *testing.T) (string, config.Config) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"pw-record": `#!/bin/sh
source=mic
for arg in "$@"; do
  case "$arg" in *stream.capture.sink=true*) source=system;; esac
done
output=
for arg in "$@"; do output="$arg"; done
printf 'RIFF%s' "$source" > "$output"
printf 'audio-padding-01234567890123456789012345678901234567890123456789' >> "$output"
printf '%s %s\n' "$source" "$*" >> "$PW_RECORD_LOG"
trap 'exit 0' INT TERM
while :; do sleep 0.05; done
`,
		"ffmpeg": `#!/bin/sh
output=
for arg in "$@"; do output="$arg"; done
printf 'RIFFmixed-audio-padding-01234567890123456789012345678901234567890123456789' > "$output"
printf '%s\n' "$*" >> "$FFMPEG_LOG"
`,
		"whisper-cli": `#!/bin/sh
output=
while [ "$#" -gt 0 ]; do
  printf '<%s>\n' "$1" >> "$WHISPER_LOG"
  if [ "$1" = "-of" ]; then
    shift
    output="$1"
    printf '<%s>\n' "$1" >> "$WHISPER_LOG"
  fi
  shift
done
printf '%s\n' "${WHISPER_TEXT:-phase two transcript}" > "${output}.txt"
`,
		"wl-copy": `#!/bin/sh
cat > "$CLIPBOARD_PATH"
printf 'clipboard\n' >> "$ORDER_LOG"
`,
		"notify-send": `#!/bin/sh
printf '%s\n' "$*" >> "$NOTIFY_LOG"
printf '42\n'
`,
		"canberra-gtk-play": `#!/bin/sh
args="$*"
printf '%s\n' "$args" >> "$SOUND_LOG"
case "$args" in
  *service-login*)
    printf 'transcription-started\n' >> "$ORDER_LOG"
    if [ -n "${TRANSCRIPTION_SOUND_DELAY:-}" ]; then sleep "$TRANSCRIPTION_SOUND_DELAY"; fi
    printf 'transcription-finished\n' >> "$ORDER_LOG"
    ;;
esac
`,
		"ydotool": `#!/bin/sh
printf '%s\n' "$*" >> "$YDOTOOL_LOG"
exit "${YDOTOOL_EXIT:-0}"
`,
	} {
		writeExecutable(t, filepath.Join(bin, name), body)
	}

	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	stateHome := filepath.Join(root, "state")
	sessions := filepath.Join(dataHome, "risper", "sessions")
	clipboard := filepath.Join(root, "clipboard.txt")
	for key, value := range map[string]string{
		"XDG_CONFIG_HOME":  configHome,
		"XDG_DATA_HOME":    dataHome,
		"XDG_STATE_HOME":   stateHome,
		"XDG_SESSION_TYPE": "wayland",
		"PW_RECORD_LOG":    filepath.Join(root, "pw-record.log"),
		"FFMPEG_LOG":       filepath.Join(root, "ffmpeg.log"),
		"WHISPER_LOG":      filepath.Join(root, "whisper.log"),
		"CLIPBOARD_PATH":   clipboard,
		"NOTIFY_LOG":       filepath.Join(root, "notify.log"),
		"SOUND_LOG":        filepath.Join(root, "sound.log"),
		"YDOTOOL_LOG":      filepath.Join(root, "ydotool.log"),
		"ORDER_LOG":        filepath.Join(root, "order.log"),
	} {
		t.Setenv(key, value)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.MkdirAll(filepath.Join(configHome, "risper"), 0o755); err != nil {
		t.Fatal(err)
	}
	configText := `sessions_dir = "` + sessions + `"
selected_model = "stub"
transcription_engine = "external"
model = "stub"
language = "en"
paste_mode = "clipboard_only"
play_sounds = true
audio_retention = "7d"
`
	if err := os.WriteFile(filepath.Join(configHome, "risper", "config.toml"), []byte(configText), 0o644); err != nil {
		t.Fatal(err)
	}
	models := `[models.stub]
engine = "whisper.cpp"
model = "stub"
language = "en"
command = "whisper-cli -f {audio} --prompt \"{prompt}\" -nt -otxt -of {raw_no_txt}"
prompt = "Names and terms: Abdullah, coEngen, Singular Machines, Culham, Adrian, James, Will, Flic, Claude, Claude Code, Codex, ChatGPT, Emacs, Temporal, divertor. Plainer, shorter."
`
	if err := os.WriteFile(filepath.Join(configHome, "risper", "models.toml"), []byte(models), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return root, cfg
}

func TestToggleRunsRealRecordMixTranscribeClipboardCycle(t *testing.T) {
	root, cfg := stubEnvironment(t)
	clipboard := filepath.Join(root, "clipboard.txt")

	if code := Main(nil); code != 0 {
		t.Fatalf("start returned %d", code)
	}
	stale, err := session.Last(cfg)
	if err != nil || stale == nil {
		t.Fatalf("current session = %#v, %v", stale, err)
	}
	stale.Errors = []string{"stale error from an earlier attempt"}
	if err := session.SaveMetadata(stale); err != nil {
		t.Fatal(err)
	}
	if code := Main(nil); code != 0 {
		t.Fatalf("default stop returned %d", code)
	}
	first, err := session.Last(cfg)
	if err != nil || first == nil {
		t.Fatalf("first session = %#v, %v", first, err)
	}
	if first.Status != "complete" {
		t.Fatalf("first status = %q, errors=%v", first.Status, first.Errors)
	}
	if len(first.Errors) != 0 {
		t.Fatalf("successful transcription retained stale errors: %#v", first.Errors)
	}
	whisperLog := readFile(t, filepath.Join(root, "whisper.log"))
	if !strings.Contains(whisperLog, first.AudioPath) || strings.Contains(whisperLog, first.AudioSourcePaths["mic"]) {
		t.Fatalf("default transcription did not use mixed track: %q", whisperLog)
	}
	if !strings.Contains(whisperLog, "<Names and terms: Abdullah, coEngen, Singular Machines, Culham, Adrian, James, Will, Flic, Claude, Claude Code, Codex, ChatGPT, Emacs, Temporal, divertor. Plainer, shorter.>") {
		t.Fatalf("prompt was not preserved as one argument: %q", whisperLog)
	}
	if got := strings.TrimSpace(readFile(t, first.TranscriptRawPath)); got != "phase two transcript" {
		t.Fatalf("raw transcript = %q", got)
	}
	if got := strings.TrimSpace(readFile(t, first.TranscriptCleanPath)); got != "phase two transcript" {
		t.Fatalf("clean transcript = %q", got)
	}
	assertAudioFiles(t, first)

	if code := Main(nil); code != 0 {
		t.Fatalf("second start returned %d", code)
	}
	if code := Main(nil); code != 0 {
		t.Fatalf("second stop returned %d", code)
	}
	allSessions, err := session.All(cfg)
	if err != nil || len(allSessions) != 2 {
		t.Fatalf("sessions = %#v, %v", allSessions, err)
	}
	var second *session.Metadata
	for _, candidate := range allSessions {
		if candidate.SessionID != first.SessionID {
			second = candidate
			break
		}
	}
	if second == nil {
		t.Fatalf("could not identify second session among %#v", allSessions)
	}
	if second.Status != "complete" {
		t.Fatalf("second status = %q, errors=%v", second.Status, second.Errors)
	}
	whisperLog = readFile(t, filepath.Join(root, "whisper.log"))
	if strings.Count(whisperLog, first.AudioPath) != 1 || strings.Count(whisperLog, second.AudioPath) != 1 || strings.Contains(whisperLog, second.AudioSourcePaths["mic"]) {
		t.Fatalf("default transcription paths = %q", whisperLog)
	}
	if got := strings.TrimSpace(readFile(t, clipboard)); got != "phase two transcript" {
		t.Fatalf("clipboard = %q", got)
	}
	if got := strings.TrimSpace(readFile(t, second.TranscriptRawPath)); got != "phase two transcript" {
		t.Fatalf("mixed raw transcript = %q", got)
	}
	if got := strings.TrimSpace(readFile(t, second.TranscriptCleanPath)); got != "phase two transcript" {
		t.Fatalf("mixed clean transcript = %q", got)
	}
	if got := readFile(t, filepath.Join(root, "ffmpeg.log")); !strings.Contains(got, "normalize=1") {
		t.Fatalf("ffmpeg was not asked for a normalised mix: %q", got)
	}
	if got := readFile(t, filepath.Join(root, "pw-record.log")); !strings.Contains(got, "stream.capture.sink=true") {
		t.Fatalf("system recorder did not request sink capture: %q", got)
	}

	for _, metadata := range []*session.Metadata{first, second} {
		records, readErr := events.Read(session.SessionDir(metadata), 0)
		if readErr != nil {
			t.Fatal(readErr)
		}
		seen := map[string]bool{}
		for _, record := range records {
			if event, ok := record["event"].(string); ok {
				seen[event] = true
			}
		}
		for _, required := range []string{"recorder.mixed", "transcription.completed", "clipboard.copy", "paste.skipped"} {
			if !seen[required] {
				t.Fatalf("session %s missing event %s: %#v", metadata.SessionID, required, records)
			}
		}
	}
}

func TestTogglePastesAndSubmitsWhenAsked(t *testing.T) {
	root, cfg := stubEnvironment(t)
	if code := Main(nil); code != 0 {
		t.Fatalf("start returned %d", code)
	}
	if code := Main([]string{"--paste", "--enter"}); code != 0 {
		t.Fatalf("paste stop returned %d", code)
	}
	if got := readFile(t, filepath.Join(root, "ydotool.log")); got != "key --delay 150 ctrl+v\nkey --delay 300 enter\n" {
		t.Fatalf("ydotool log = %q", got)
	}
	last, err := session.Last(cfg)
	if err != nil || last == nil {
		t.Fatalf("session = %#v, %v", last, err)
	}
	if last.Status != "complete" {
		t.Fatalf("status = %q errors=%v", last.Status, last.Errors)
	}
	if last.PasteAttempted == nil || !*last.PasteAttempted {
		t.Fatalf("paste_attempted = %#v", last.PasteAttempted)
	}
	if last.PasteHelperSucceeded == nil || !*last.PasteHelperSucceeded {
		t.Fatalf("paste_helper_succeeded = %#v", last.PasteHelperSucceeded)
	}
	// Risper cannot see the target window accept the keys, so the stronger
	// claim stays false however well the helper ran.
	if last.PasteSucceeded == nil || *last.PasteSucceeded {
		t.Fatalf("paste_succeeded = %#v", last.PasteSucceeded)
	}
	if last.PasteConfirmation != "helper_ran_target_unverified" {
		t.Fatalf("paste_confirmation = %q", last.PasteConfirmation)
	}
}

func TestToggleDoesNotPutTranscriptionSoundOnClipboardPath(t *testing.T) {
	root, _ := stubEnvironment(t)
	t.Setenv("TRANSCRIPTION_SOUND_DELAY", "0.5")
	if code := Main(nil); code != 0 {
		t.Fatalf("start returned %d", code)
	}
	if code := Main(nil); code != 0 {
		t.Fatalf("stop returned %d", code)
	}

	lines := strings.Split(strings.TrimSpace(readFile(t, filepath.Join(root, "order.log"))), "\n")
	started := indexOf(lines, "transcription-started")
	clipboard := indexOf(lines, "clipboard")
	finished := indexOf(lines, "transcription-finished")
	if started < 0 || clipboard < 0 || finished < 0 {
		t.Fatalf("sound/clipboard order = %#v", lines)
	}
	if finished < clipboard {
		t.Fatalf("transcription sound finished before clipboard: %#v", lines)
	}
}

func TestToggleRemovesVoiceStopWordBeforePaste(t *testing.T) {
	root, cfg := stubEnvironment(t)
	t.Setenv("WHISPER_TEXT", "phase two transcript marzipan.")
	if code := Main(nil); code != 0 {
		t.Fatalf("start returned %d", code)
	}
	if code := Main([]string{"--paste", "--voice-stop"}); code != 0 {
		t.Fatalf("voice stop returned %d", code)
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(root, "clipboard.txt"))); got != "phase two transcript" {
		t.Fatalf("clipboard = %q", got)
	}
	last, err := session.Last(cfg)
	if err != nil || last == nil {
		t.Fatalf("session = %#v, %v", last, err)
	}
	if got := strings.TrimSpace(readFile(t, last.TranscriptCleanPath)); got != "phase two transcript" {
		t.Fatalf("clean transcript = %q", got)
	}
	records, err := events.Read(session.SessionDir(last), 0)
	if err != nil {
		t.Fatal(err)
	}
	var completed map[string]any
	for _, record := range records {
		if record["event"] == "transcription.completed" {
			completed = record
		}
	}
	if completed == nil || completed["voice_trigger"] != "marzipan" {
		t.Fatalf("completed event = %#v", completed)
	}
}

func TestToggleKeepsTranscriptWhenPasteHelperFails(t *testing.T) {
	root, cfg := stubEnvironment(t)
	t.Setenv("YDOTOOL_EXIT", "1")
	if code := Main(nil); code != 0 {
		t.Fatalf("start returned %d", code)
	}
	if code := Main([]string{"--paste", "--enter"}); code != 0 {
		t.Fatalf("failed paste turned a saved transcript into exit %d", code)
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(root, "clipboard.txt"))); got != "phase two transcript" {
		t.Fatalf("clipboard = %q", got)
	}
	if got := readFile(t, filepath.Join(root, "ydotool.log")); strings.Contains(got, "enter") {
		t.Fatalf("Return was sent after the paste failed: %q", got)
	}
	last, err := session.Last(cfg)
	if err != nil || last == nil {
		t.Fatalf("session = %#v, %v", last, err)
	}
	if last.Status != "complete" || last.PasteConfirmation != "not_pasted_clipboard_retained" {
		t.Fatalf("status = %q confirmation = %q", last.Status, last.PasteConfirmation)
	}
}

func TestToggleRejectsEnterWithoutPaste(t *testing.T) {
	if code := Main([]string{"--enter"}); code != 2 {
		t.Fatalf("--enter alone returned %d, want usage error 2", code)
	}
}

func TestToggleRejectsRemovedAudioSourceFlag(t *testing.T) {
	if code := Main([]string{"--system"}); code != 2 {
		t.Fatalf("--system returned %d, want usage error 2", code)
	}
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func indexOf(lines []string, want string) int {
	for index, line := range lines {
		if line == want {
			return index
		}
	}
	return -1
}

func assertAudioFiles(t *testing.T, metadata *session.Metadata) {
	t.Helper()
	if len(metadata.AudioSourcePaths) != 2 {
		t.Fatalf("audio source paths = %#v", metadata.AudioSourcePaths)
	}
	for source, path := range metadata.AudioSourcePaths {
		info, err := os.Stat(path)
		if err != nil || info.Size() <= 44 {
			t.Fatalf("%s source path %s: %v", source, path, err)
		}
	}
	if info, err := os.Stat(metadata.AudioPath); err != nil || info.Size() <= 44 {
		t.Fatalf("mixed audio path %s: %v", metadata.AudioPath, err)
	}
}
