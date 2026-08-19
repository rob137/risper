package pastecheck

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainCopiesClipboardMarker(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	clipboard := filepath.Join(root, "clipboard.txt")
	if err := os.WriteFile(filepath.Join(bin, "wl-copy"), []byte("#!/bin/sh\n/usr/bin/cat > \"$CLIPBOARD_PATH\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	t.Setenv("CLIPBOARD_PATH", clipboard)

	stdout := captureStdout(t, func() int { return Main(nil) })
	if !strings.Contains(stdout, "Copied marker RISPER_PASTE_TEST_") || !strings.Contains(stdout, "Paste it") {
		t.Fatalf("output = %q", stdout)
	}
	if got, err := os.ReadFile(clipboard); err != nil || !strings.HasPrefix(string(got), "RISPER_PASTE_TEST_") {
		t.Fatalf("clipboard = %q, err=%v", got, err)
	}
}

func captureStdout(t *testing.T, run func() int) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = writer
	code := run()
	writer.Close()
	os.Stdout = old
	if code != 0 {
		t.Fatalf("Main() = %d", code)
	}
	data, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
