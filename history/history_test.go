package history

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

func TestMainListsRecentSessionsWithLimit(t *testing.T) {
	cfg := historyTestConfig(t)
	older := makeHistorySession(t, cfg, time.Date(2026, 8, 19, 11, 0, 0, 0, time.UTC), "older transcript")
	newer := makeHistorySession(t, cfg, time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC), "newer transcript")
	older.Status = "failed"
	older.Errors = []string{"engine failed"}
	duration := 1.25
	older.DurationSeconds = &duration
	if err := session.SaveMetadata(older); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := captureOutput(t, func() int { return Main([]string{"--limit", "1"}) })
	if code != 0 || stderr != "" {
		t.Fatalf("history list = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "session") || !strings.Contains(stdout, newer.SessionID) || strings.Contains(stdout, older.SessionID) {
		t.Fatalf("limited history = %q", stdout)
	}
	if !strings.Contains(stdout, "newer transcript") {
		t.Fatalf("history preview missing: %q", stdout)
	}

	code, stdout, stderr = captureOutput(t, func() int { return Main([]string{"--limit", "0"}) })
	if code != 0 || stderr != "" || stdout != "No Risper sessions yet.\n" {
		t.Fatalf("zero limit = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	code, _, stderr = captureOutput(t, func() int { return Main([]string{"--limit", "-1"}) })
	if code != 2 || !strings.Contains(stderr, "--limit must not be negative") {
		t.Fatalf("negative limit = code %d, stderr %q", code, stderr)
	}
}

func TestMainRunsSessionActionsAndReportsMissingSessions(t *testing.T) {
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
	cfg := historyTestConfig(t)
	metadata := makeHistorySession(t, cfg, time.Date(2026, 8, 19, 13, 0, 0, 0, time.UTC), "history action transcript")

	code, _, stderr := captureOutput(t, func() int { return Main([]string{"--open", metadata.SessionID}) })
	if code != 0 || stderr != "" {
		t.Fatalf("open action = code %d, stderr %q", code, stderr)
	}
	code, _, stderr = captureOutput(t, func() int { return Main([]string{"--play", metadata.SessionID}) })
	if code != 0 || stderr != "" {
		t.Fatalf("play action = code %d, stderr %q", code, stderr)
	}
	code, stdout, stderr := captureOutput(t, func() int { return Main([]string{"--copy", metadata.SessionID}) })
	if code != 0 || stderr != "" || !strings.Contains(stdout, "copied with wl-copy") {
		t.Fatalf("copy action = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if got := readFile(t, clipboard); got != "history action transcript" {
		t.Fatalf("clipboard = %q", got)
	}
	waitForFileContains(t, gioLog, metadata.AudioPath)
	gioOutput := readFile(t, gioLog)
	if !strings.Contains(gioOutput, "open\n"+session.SessionDir(metadata)) || !strings.Contains(gioOutput, "open\n"+metadata.AudioPath) {
		t.Fatalf("gio calls = %q", gioOutput)
	}

	code, _, stderr = captureOutput(t, func() int { return Main([]string{"--open", "missing"}) })
	if code != 1 || !strings.Contains(stderr, "No such session: missing") {
		t.Fatalf("missing action = code %d, stderr %q", code, stderr)
	}
	code, _, stderr = captureOutput(t, func() int { return Main([]string{"--retranscribe", "missing"}) })
	if code != 1 || !strings.Contains(stderr, "No such session: missing") {
		t.Fatalf("missing retranscription = code %d, stderr %q", code, stderr)
	}
	code, _, stderr = captureOutput(t, func() int { return Main([]string{"--prune-audio"}) })
	if code != 1 || !strings.Contains(stderr, "audio_retention is 'never'") {
		t.Fatalf("never prune = code %d, stderr %q", code, stderr)
	}
}

func TestDeleteSessionRequiresExplicitConfirmation(t *testing.T) {
	t.Setenv("PATH", "")
	cfg := historyTestConfig(t)
	cancelled := makeHistorySession(t, cfg, time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC), "cancelled")
	deleted := makeHistorySession(t, cfg, time.Date(2026, 8, 19, 15, 0, 0, 0, time.UTC), "deleted")

	oldStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = reader
	_, _ = writer.WriteString("no\n")
	writer.Close()
	code, stdout, stderr := captureOutput(t, func() int {
		return deleteSession([]*session.Metadata{cancelled}, cancelled.SessionID)
	})
	reader.Close()
	os.Stdin = oldStdin
	if code != 1 || stderr != "" || !strings.Contains(stdout, "Cancelled.") {
		t.Fatalf("cancel delete = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if _, err := os.Stat(session.SessionDir(cancelled)); err != nil {
		t.Fatalf("cancelled session was removed: %v", err)
	}

	reader, writer, err = os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = reader
	_, _ = writer.WriteString("DELETE\n")
	writer.Close()
	code, stdout, stderr = captureOutput(t, func() int {
		return deleteSession([]*session.Metadata{deleted}, deleted.SessionID)
	})
	reader.Close()
	os.Stdin = oldStdin
	if code != 0 || stderr != "" || !strings.Contains(stdout, "Deleted "+deleted.SessionID) {
		t.Fatalf("confirmed delete = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if _, err := os.Stat(session.SessionDir(deleted)); !os.IsNotExist(err) {
		t.Fatalf("deleted session still exists, stat error %v", err)
	}
}

func historyTestConfig(t *testing.T) config.Config {
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

func makeHistorySession(t *testing.T, cfg config.Config, started time.Time, transcript string) *session.Metadata {
	t.Helper()
	metadata, err := session.CreateAt(cfg, started)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadata.AudioPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadata.TranscriptCleanPath, []byte(transcript), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata.Status = "complete"
	if err := session.SaveMetadata(metadata); err != nil {
		t.Fatal(err)
	}
	return metadata
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
