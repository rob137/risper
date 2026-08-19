package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainRejectsUnknownCommandsAndPrintsUsage(t *testing.T) {
	code, stdout, stderr := captureOutput(t, func() int { return Main([]string{"mystery"}) })
	if code != 2 || stdout != "" {
		t.Fatalf("unknown command = code %d, stdout %q", code, stdout)
	}
	if !strings.Contains(stderr, `unknown command "mystery"`) || !strings.Contains(stderr, "Usage: risper") {
		t.Fatalf("unknown command stderr = %q", stderr)
	}

	for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}} {
		code, stdout, stderr = captureOutput(t, func() int { return Main(args) })
		if code != 0 || stderr != "" || !strings.Contains(stdout, "Usage: risper") {
			t.Fatalf("help %v = code %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
}

func TestMainDispatchesTopLevelServiceActionAndBareStart(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "systemctl.log")
	if err := os.WriteFile(filepath.Join(bin, "systemctl"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$SYSTEMCTL_LOG\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("SYSTEMCTL_LOG", logPath)

	code, stdout, stderr := captureOutput(t, func() int { return Main([]string{"status"}) })
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("status = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if got := readFile(t, logPath); got != "--user\nstatus\nrisper.service\n--no-pager\n" {
		t.Fatalf("status dispatch args = %q", got)
	}

	code, stdout, stderr = captureOutput(t, func() int { return Main(nil) })
	if code != 0 || stderr != "" || !strings.Contains(stdout, "Risper daemon enabled and running.") {
		t.Fatalf("bare risper = code %d, stdout %q, stderr %q", code, stdout, stderr)
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

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
