package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainDispatchesServiceActionsToSystemctl(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "systemctl.log")
	writeExecutable(t, filepath.Join(bin, "systemctl"), `#!/bin/sh
printf '%s\n' "$@" > "$SYSTEMCTL_LOG"
exit "${SYSTEMCTL_EXIT:-0}"
`)
	t.Setenv("PATH", bin)
	t.Setenv("SYSTEMCTL_LOG", logPath)

	tests := []struct {
		name       string
		args       []string
		wantArgs   string
		wantOutput string
	}{
		{name: "default start", args: nil, wantArgs: "--user\nenable\n--now\nrisper.service\n", wantOutput: "Risper daemon enabled and running."},
		{name: "stop", args: []string{"stop"}, wantArgs: "--user\nstop\nrisper.service\n", wantOutput: "Risper daemon stopped. Autostart remains enabled."},
		{name: "kill alias", args: []string{"kill"}, wantArgs: "--user\nstop\nrisper.service\n", wantOutput: "Risper daemon stopped. Autostart remains enabled."},
		{name: "restart", args: []string{"restart"}, wantArgs: "--user\nrestart\nrisper.service\n", wantOutput: "Risper daemon restarted."},
		{name: "status", args: []string{"status"}, wantArgs: "--user\nstatus\nrisper.service\n--no-pager\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := captureOutput(t, func() int { return Main(test.args) })
			if code != 0 || stderr != "" {
				t.Fatalf("Main() = code %d, stdout %q, stderr %q", code, stdout, stderr)
			}
			if got := readFile(t, logPath); got != test.wantArgs {
				t.Fatalf("systemctl args = %q, want %q", got, test.wantArgs)
			}
			if test.wantOutput != "" && !strings.Contains(stdout, test.wantOutput) {
				t.Fatalf("stdout = %q, want %q", stdout, test.wantOutput)
			}
		})
	}
}

func TestMainReturnsServiceErrorsAndUsageErrors(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(bin, "systemctl"), "#!/bin/sh\nexit 7\n")
	t.Setenv("PATH", bin)
	code, stdout, stderr := captureOutput(t, func() int { return Main([]string{"restart"}) })
	if code != 7 || stdout != "" || stderr != "" {
		t.Fatalf("failed service = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	code, _, stderr = captureOutput(t, func() int { return Main([]string{"explode"}) })
	if code != 2 || !strings.Contains(stderr, `invalid action "explode"`) {
		t.Fatalf("invalid action = code %d, stderr %q", code, stderr)
	}
	code, _, stderr = captureOutput(t, func() int { return Main([]string{"start", "extra"}) })
	if code != 2 || !strings.Contains(stderr, "unexpected positional arguments") {
		t.Fatalf("extra argument = code %d, stderr %q", code, stderr)
	}
	code, _, _ = captureOutput(t, func() int { return Main([]string{"--help"}) })
	if code != 0 {
		t.Fatalf("help returned %d", code)
	}
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
