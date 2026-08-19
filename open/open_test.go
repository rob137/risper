package open

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/session"
)

func TestMainOpensConfiguredTargetsAndCopiesLastTranscript(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	gioLog := filepath.Join(root, "gio.log")
	clipboard := filepath.Join(root, "clipboard.txt")
	writeExecutable(t, filepath.Join(bin, "gio"), `#!/bin/sh
printf '%s\n' "$@" >> "$GIO_LOG"
`)
	writeExecutable(t, filepath.Join(bin, "wl-copy"), `#!/bin/sh
cat > "$CLIPBOARD_PATH"
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GIO_LOG", gioLog)
	t.Setenv("CLIPBOARD_PATH", clipboard)
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	cfg := openTestConfig(t)

	for _, target := range []string{"recordings", "config"} {
		code, _, stderr := captureOutput(t, func() int { return Main([]string{target}) })
		if code != 0 || stderr != "" {
			t.Fatalf("open %s = code %d, stderr %q", target, code, stderr)
		}
	}
	code, _, stderr := captureOutput(t, func() int { return Main([]string{"last-session"}) })
	if code != 1 || !strings.Contains(stderr, "No Risper sessions yet.") {
		t.Fatalf("open without session = code %d, stderr %q", code, stderr)
	}

	metadata, err := session.CreateAt(cfg, time.Date(2026, 8, 19, 16, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadata.AudioPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadata.TranscriptCleanPath, []byte("last transcript"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{"last-session", "last-audio", "play-last", "last-transcript"} {
		code, _, stderr = captureOutput(t, func() int { return Main([]string{target}) })
		if code != 0 || stderr != "" {
			t.Fatalf("open %s = code %d, stderr %q", target, code, stderr)
		}
	}
	code, stdout, stderr := captureOutput(t, func() int { return Main([]string{"copy-last"}) })
	if code != 0 || stderr != "" || stdout != "copied with wl-copy\n" {
		t.Fatalf("copy-last = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if got := readFile(t, clipboard); got != "last transcript" {
		t.Fatalf("clipboard = %q", got)
	}
	waitForFileContains(t, gioLog, metadata.TranscriptCleanPath)
	gioOutput := readFile(t, gioLog)
	if !strings.Contains(gioOutput, "open\n"+cfg.SessionsDir) || !strings.Contains(gioOutput, "open\n"+cfg.ConfigPath) || !strings.Contains(gioOutput, "open\n"+session.SessionDir(metadata)) {
		t.Fatalf("gio calls = %q", gioOutput)
	}

	code, _, stderr = captureOutput(t, func() int { return Main([]string{"invalid"}) })
	if code != 2 || !strings.Contains(stderr, `invalid target "invalid"`) {
		t.Fatalf("invalid target = code %d, stderr %q", code, stderr)
	}
	code, _, stderr = captureOutput(t, func() int { return Main([]string{"recordings", "extra"}) })
	if code != 2 || !strings.Contains(stderr, "unexpected positional arguments") {
		t.Fatalf("extra target = code %d, stderr %q", code, stderr)
	}
}

func TestMainReportsMissingTranscriptAndOpenFailures(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("PATH", "")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := session.CreateAt(cfg, time.Date(2026, 8, 19, 17, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	code, _, stderr := captureOutput(t, func() int { return Main([]string{"last-transcript"}) })
	if code != 1 || !strings.Contains(stderr, "Last session has no transcript.") {
		t.Fatalf("missing transcript = code %d, stderr %q", code, stderr)
	}
	code, _, stderr = captureOutput(t, func() int { return openPath(session.SessionDir(metadata)) })
	if code != 1 || !strings.Contains(stderr, "gio is not installed") {
		t.Fatalf("missing gio = code %d, stderr %q", code, stderr)
	}
	code, _, stderr = captureOutput(t, func() int { return openPath(filepath.Join(root, "missing")) })
	if code != 1 || !strings.Contains(stderr, "not found:") {
		t.Fatalf("missing path = code %d, stderr %q", code, stderr)
	}
}

func openTestConfig(t *testing.T) config.Config {
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

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func waitForFileContains(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%q did not contain %q", path, want)
}
