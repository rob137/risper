package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rob137/risper/config"
)

func TestPlayWaitsForSoundHelper(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "finished")
	script := "#!/bin/sh\nsleep 0.05\nprintf done > " + marker + "\n"
	if err := os.WriteFile(filepath.Join(bin, "canberra-gtk-play"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	Play(config.Config{PlaySounds: true}, "success")

	if got, err := os.ReadFile(marker); err != nil || string(got) != "done" {
		t.Fatalf("sound helper marker = %q, err = %v", got, err)
	}
}
