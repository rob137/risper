package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestPlayAsyncCanBeWaitedAtLifecycleBoundary(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	started := filepath.Join(root, "started")
	finished := filepath.Join(root, "finished")
	script := "#!/bin/sh\nprintf started > " + started + "\nsleep 0.2\nprintf finished > " + finished + "\n"
	if err := os.WriteFile(filepath.Join(bin, "canberra-gtk-play"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	playback := PlayAsync(config.Config{PlaySounds: true}, "success")
	if err := waitForTestFile(started); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(finished); !os.IsNotExist(err) {
		t.Fatalf("sound finished before caller waited: err = %v", err)
	}
	playback.Wait()
	if got, err := os.ReadFile(finished); err != nil || string(got) != "finished" {
		t.Fatalf("sound helper completion = %q, err = %v", got, err)
	}
}

func waitForTestFile(path string) error {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", path)
}
