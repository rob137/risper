package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob137/risper/config"
)

func TestSendSuccessUsesGeneratedActiveThemeSound(t *testing.T) {
	root, cfg, soundLog, ffmpegLog := setupGeneratedSoundTest(t, true)
	Play(cfg, "success_send")

	log := readDesktopTestFile(t, soundLog)
	if !strings.Contains(log, "-f "+filepath.Join(cfg.DataDir, "success-send-")) || strings.Contains(log, "-i complete") {
		t.Fatalf("send sound arguments = %q", log)
	}
	ffmpeg := readDesktopTestFile(t, ffmpegLog)
	source := filepath.Join(root, "data", "sounds", "Yaru", "stereo", "complete.oga")
	if strings.Count(ffmpeg, source) != 2 || !strings.Contains(ffmpeg, sendSoundFilter) {
		t.Fatalf("send generation arguments = %q", ffmpeg)
	}
	generated, err := filepath.Glob(filepath.Join(cfg.DataDir, "success-send-*.wav"))
	if err != nil || len(generated) != 1 {
		t.Fatalf("generated send sounds = %#v, err = %v", generated, err)
	}

	Play(cfg, "success_send")
	if got := strings.Count(readDesktopTestFile(t, ffmpegLog), sendSoundFilter); got != 1 {
		t.Fatalf("cached send generation count = %d, want 1", got)
	}
}

func TestPlainSuccessKeepsThemeEventPlayback(t *testing.T) {
	_, cfg, soundLog, ffmpegLog := setupGeneratedSoundTest(t, true)
	Play(cfg, "success")

	if got := readDesktopTestFile(t, soundLog); !strings.Contains(got, "-i complete") || strings.Contains(got, "-f ") {
		t.Fatalf("plain success arguments = %q", got)
	}
	if got := readDesktopTestFile(t, ffmpegLog); got != "" {
		t.Fatalf("plain success unexpectedly generated audio: %q", got)
	}
}

func TestSendSuccessFallsBackWithoutRubberband(t *testing.T) {
	_, cfg, soundLog, ffmpegLog := setupGeneratedSoundTest(t, false)
	Play(cfg, "success_send")

	if got := readDesktopTestFile(t, soundLog); !strings.Contains(got, "-i complete") || strings.Contains(got, "-f ") {
		t.Fatalf("fallback success arguments = %q", got)
	}
	if got := readDesktopTestFile(t, ffmpegLog); got != "" {
		t.Fatalf("fallback unexpectedly ran generation: %q", got)
	}
	generated, err := filepath.Glob(filepath.Join(cfg.DataDir, "success-send-*.wav"))
	if err != nil || len(generated) != 0 {
		t.Fatalf("fallback generated send sounds = %#v, err = %v", generated, err)
	}
}

func TestSendSuccessRegeneratesWhenThemeChanges(t *testing.T) {
	root, cfg, _, ffmpegLog := setupGeneratedSoundTest(t, true)
	otherTheme := filepath.Join(root, "data", "sounds", "Breeze", "stereo")
	if err := os.MkdirAll(otherTheme, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherTheme, "complete.oga"), []byte("breeze source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data", "sounds", "Breeze", "index.theme"), []byte("[Sound Theme]\nDirectories=stereo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	Play(cfg, "success_send")
	t.Setenv("SOUND_THEME", "Breeze")
	Play(cfg, "success_send")

	generated, err := filepath.Glob(filepath.Join(cfg.DataDir, "success-send-*.wav"))
	if err != nil || len(generated) != 2 {
		t.Fatalf("theme-specific generated sounds = %#v, err = %v", generated, err)
	}
	ffmpeg := readDesktopTestFile(t, ffmpegLog)
	yaruSource := filepath.Join(root, "data", "sounds", "Yaru", "stereo", "complete.oga")
	breezeSource := filepath.Join(root, "data", "sounds", "Breeze", "stereo", "complete.oga")
	if strings.Count(ffmpeg, yaruSource) != 2 || strings.Count(ffmpeg, breezeSource) != 2 {
		t.Fatalf("theme-specific generation arguments = %q", ffmpeg)
	}
}

func setupGeneratedSoundTest(t *testing.T, rubberband bool) (string, config.Config, string, string) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	dataHome := filepath.Join(root, "data")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	soundDir := filepath.Join(dataHome, "sounds", "Yaru", "stereo")
	if err := os.MkdirAll(soundDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataHome, "sounds", "Yaru", "index.theme"), []byte("[Sound Theme]\nDirectories=stereo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(soundDir, "complete.oga"), []byte("yaru source"), 0o644); err != nil {
		t.Fatal(err)
	}
	soundLog := filepath.Join(root, "sound.log")
	ffmpegLog := filepath.Join(root, "ffmpeg.log")
	for _, path := range []string{soundLog, ffmpegLog} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeDesktopTestExecutable(t, filepath.Join(bin, "gsettings"), `#!/bin/sh
printf "'%s'\n" "${SOUND_THEME:-Yaru}"
`)
	writeDesktopTestExecutable(t, filepath.Join(bin, "canberra-gtk-play"), `#!/bin/sh
printf '%s\n' "$*" >> "$SOUND_LOG"
`)
	filterOutput := ""
	if rubberband {
		filterOutput = "printf ' T.C rubberband A->A Apply time-stretching\\n'\n"
	}
	writeDesktopTestExecutable(t, filepath.Join(bin, "ffmpeg"), `#!/bin/sh
if [ "$1" = "-hide_banner" ] && [ "$2" = "-filters" ]; then
`+filterOutput+`  exit 0
fi
printf '%s\n' "$*" >> "$FFMPEG_LOG"
output=
for arg in "$@"; do output="$arg"; done
printf 'RIFFgenerated-send-audio-padding-012345678901234567890123456789' > "$output"
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_DATA_DIRS", dataHome)
	t.Setenv("SOUND_THEME", "Yaru")
	t.Setenv("SOUND_LOG", soundLog)
	t.Setenv("FFMPEG_LOG", ffmpegLog)
	return root, config.Config{PlaySounds: true, DataDir: filepath.Join(dataHome, "risper")}, soundLog, ffmpegLog
}

func writeDesktopTestExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readDesktopTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
