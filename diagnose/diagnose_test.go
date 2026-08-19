package diagnose

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/events"
	"github.com/rob137/risper/models"
	"github.com/rob137/risper/session"
	"github.com/rob137/risper/transcription"
)

func TestMainPrintsEnvironmentDiagnosisWithoutReadingLiveData(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(bin, "lsb_release"), "#!/bin/sh\nprintf 'Distributor ID: Test Linux\\n'\n")
	writeExecutable(t, filepath.Join(bin, "gnome-shell"), "#!/bin/sh\nprintf 'GNOME Shell 99\\n'\n")
	t.Setenv("PATH", bin)
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	t.Setenv("XDG_CURRENT_DESKTOP", "TestDesktop")
	t.Setenv("DESKTOP_SESSION", "test-session")
	cfg := diagnoseTestConfig(t)
	profileBinary := filepath.Join(bin, "whisper-cli")
	writeExecutable(t, profileBinary, "#!/bin/sh\nexit 0\n")
	if err := models.Write(cfg, models.Profile{
		ID: "stub", Engine: "stub-engine", Model: "tiny", Language: "cy", Command: profileBinary + " -f {audio}",
	}, true); err != nil {
		t.Fatal(err)
	}
	state := transcription.State{SessionDir: filepath.Join(cfg.SessionsDir, "active"), ControllerPID: os.Getpid()}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.CurrentTranscriptionPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := captureOutput(t, func() int { return Main(nil) })
	if code != 0 || stderr != "" {
		t.Fatalf("environment diagnosis = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	for _, want := range []string{
		"Risper diagnosis",
		"Distributor ID: Test Linux",
		"GNOME Shell 99",
		"XDG_CURRENT_DESKTOP=TestDesktop",
		"selected model      stub",
		"selected engine     stub-engine",
		"selected model name tiny",
		"command binary      yes (" + profileBinary + ")",
		"active transcription " + state.SessionDir + " worker=<nil>",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("environment diagnosis missing %q in %q", want, stdout)
		}
	}
}

func TestMainPrintsSessionDiagnosisAndHandlesBadArguments(t *testing.T) {
	cfg := diagnoseTestConfig(t)
	metadata, err := session.CreateAt(cfg, time.Date(2026, 8, 19, 18, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	ended := "2026-08-19T18:01:02Z"
	duration := 62.5
	metadata.Status = "failed"
	metadata.EndedAt = &ended
	metadata.DurationSeconds = &duration
	metadata.AudioSources = []string{"mic", "system"}
	metadata.AudioSourcePaths = map[string]string{"mic": filepath.Join(session.SessionDir(metadata), "audio.mic.wav")}
	metadata.Errors = []string{"engine unavailable"}
	if err := os.WriteFile(metadata.AudioPath, []byte("mixed audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadata.AudioSourcePaths["mic"], []byte("mic audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadata.TranscriptRawPath, []byte("raw transcript"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadata.TranscriptCleanPath, []byte("clean transcript"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(session.SessionDir(metadata), session.StatusLogFile), []byte("status one\nstatus two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(session.SessionDir(metadata), session.ErrorLogFile), []byte("error one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.LogPath, []byte("daemon one\ndaemon two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := events.AppendAt(session.SessionDir(metadata), "test.event", time.Date(2026, 8, 19, 18, 1, 0, 0, time.UTC), map[string]any{"detail": "kept"}); err != nil {
		t.Fatal(err)
	}
	if err := session.SaveMetadata(metadata); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := captureOutput(t, func() int { return Main([]string{"last"}) })
	if code != 0 || stderr != "" {
		t.Fatalf("session diagnosis = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	for _, want := range []string{
		"Risper session diagnosis: " + metadata.SessionID,
		"status               failed",
		"duration_seconds     62.5",
		"audio_sources        mic,system",
		"errors               1",
		"  - engine unavailable",
		"  audio            yes",
		"  unmixed mic      yes",
		"  unmixed system   no",
		"test.event {\"detail\":\"kept\"}",
		"status two",
		"error one",
		"daemon two",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("session diagnosis missing %q in %q", want, stdout)
		}
	}

	code, stdout, stderr = captureOutput(t, func() int { return Main([]string{"missing"}) })
	if code != 1 || stdout != "No such session: missing\n" || stderr != "" {
		t.Fatalf("missing session = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	code, _, stderr = captureOutput(t, func() int { return Main([]string{"one", "two"}) })
	if code != 2 || !strings.Contains(stderr, "unexpected positional arguments") {
		t.Fatalf("extra arguments = code %d, stderr %q", code, stderr)
	}
	code, _, stderr = captureOutput(t, func() int { return Main([]string{"--help"}) })
	if code != 0 || stderr == "" || !strings.Contains(stderr, "Usage of risper diagnose") {
		t.Fatalf("help = code %d, stderr %q", code, stderr)
	}
}

func TestDiagnosisHelpersHandleOptionalValuesAndCommandFailures(t *testing.T) {
	text := "text"
	number := 1.5
	truth := true
	for _, test := range []struct {
		name string
		got  any
		want any
	}{
		{name: "nil", got: valueOrBlank(nil), want: ""},
		{name: "string", got: valueOrBlank(&text), want: "text"},
		{name: "nil string", got: valueOrBlank((*string)(nil)), want: ""},
		{name: "number", got: valueOrBlank(&number), want: 1.5},
		{name: "nil number", got: valueOrBlank((*float64)(nil)), want: ""},
		{name: "bool", got: valueOrBlank(&truth), want: true},
		{name: "nil bool", got: valueOrBlank((*bool)(nil)), want: ""},
		{name: "other", got: valueOrBlank(7), want: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("valueOrBlank() = %#v, want %#v", test.got, test.want)
			}
		})
	}
	metadata := &session.Metadata{AudioPath: filepath.Join(t.TempDir(), "audio.wav"), AudioSourcePaths: map[string]string{"mic": "/custom/mic.wav"}}
	if got := sourcePath(metadata, "mic"); got != "/custom/mic.wav" {
		t.Fatalf("custom source path = %q", got)
	}
	if got := sourcePath(metadata, "system"); got != filepath.Join(session.SessionDir(metadata), "audio.system.wav") {
		t.Fatalf("fallback source path = %q", got)
	}
	if present, size := fileInfo(filepath.Join(t.TempDir(), "missing")); present || size != 0 {
		t.Fatalf("missing file info = %v, %d", present, size)
	}
	if got := run([]string{"/bin/sh", "-c", "printf success"}); got != "success" {
		t.Fatalf("successful run = %q", got)
	}
	if got := run([]string{"/bin/sh", "-c", "exit 3"}); !strings.HasPrefix(got, "unavailable: ") {
		t.Fatalf("failed run = %q", got)
	}
	if got := run([]string{"/definitely/missing/risper-command"}); !strings.HasPrefix(got, "unavailable: ") {
		t.Fatalf("missing run = %q", got)
	}
}

func diagnoseTestConfig(t *testing.T) config.Config {
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

func captureOutput(t *testing.T, run func() int) (int, string, string) {
	t.Helper()
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout, oldStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter
	code := run()
	stdoutWriter.Close()
	stderrWriter.Close()
	os.Stdout, os.Stderr = oldStdout, oldStderr
	stdout, _ := io.ReadAll(stdoutReader)
	stderr, _ := io.ReadAll(stderrReader)
	stdoutReader.Close()
	stderrReader.Close()
	return code, string(stdout), string(stderr)
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
