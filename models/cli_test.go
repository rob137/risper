package models

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob137/risper/config"
)

func TestMainManagesProfiles(t *testing.T) {
	cliTestConfig(t)

	code, stdout, stderr := captureOutput(t, func() int { return Main(nil) })
	if code != 0 || stderr != "" || !strings.Contains(stdout, "No profiles configured.") {
		t.Fatalf("empty list = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	code, stdout, stderr = captureOutput(t, func() int {
		return Main([]string{"add-external", "slow", "--engine", "whisper.cpp", "--model", "base.en", "--command", "/bin/echo {audio}", "--language", "cy", "--select"})
	})
	if code != 0 || stderr != "" || stdout != "Added slow\n" {
		t.Fatalf("add profile = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	code, stdout, stderr = captureOutput(t, func() int {
		return Main([]string{"add-external", "fast", "--engine", "stub", "--model", "tiny", "--command", "/bin/echo fast"})
	})
	if code != 0 || stderr != "" || stdout != "Added fast\n" {
		t.Fatalf("add second profile = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	code, stdout, stderr = captureOutput(t, func() int { return Main([]string{"list"}) })
	if code != 0 || stderr != "" || !strings.Contains(stdout, "* slow") || !strings.Contains(stdout, "  fast") {
		t.Fatalf("list = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if strings.Index(stdout, "fast") > strings.Index(stdout, "slow") {
		t.Fatalf("list is not sorted: %q", stdout)
	}

	code, stdout, stderr = captureOutput(t, func() int { return Main([]string{"current"}) })
	if code != 0 || stderr != "" || stdout != "slow: whisper.cpp base.en (cy)\n" {
		t.Fatalf("current = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	code, stdout, stderr = captureOutput(t, func() int { return Main([]string{"select", "fast"}) })
	if code != 0 || stderr != "" || stdout != "Selected fast\n" {
		t.Fatalf("select = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.SelectedModel != "fast" {
		t.Fatalf("selected model = %q, want fast", reloaded.SelectedModel)
	}
}

func TestMainRejectsMalformedModelCommands(t *testing.T) {
	cliTestConfig(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "invalid action", args: []string{"unknown"}, want: "invalid action unknown"},
		{name: "missing select id", args: []string{"select"}, want: "Usage: risper models select PROFILE_ID"},
		{name: "missing add fields", args: []string{"add-external", "id"}, want: "Usage: risper models add-external"},
		{name: "unknown option", args: []string{"--bad"}, want: "action is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, _, stderr := captureOutput(t, func() int { return Main(test.args) })
			if code != 2 || !strings.Contains(stderr, test.want) {
				t.Fatalf("Main(%v) = code %d, stderr %q", test.args, code, stderr)
			}
		})
	}
	code, stdout, stderr := captureOutput(t, func() int { return Main([]string{"--help"}) })
	if code != 0 || stderr != "Usage: risper models [list|current|select|add-external]\n" || stdout != "" {
		t.Fatalf("help = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func cliTestConfig(t *testing.T) config.Config {
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
