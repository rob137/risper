package platforms

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/rob137/risper/hotkeys"
)

const testAlt uint16 = 56

func makeFIFO(t *testing.T) (string, *os.File) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "event0")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	writer, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	return path, writer
}

func writeTap(t *testing.T, writer *os.File, at time.Duration) {
	t.Helper()
	for _, value := range []int32{KeyPress, KeyRelease} {
		if _, err := writer.Write(EncodeInputEvent(at, EVKey, testAlt, value)); err != nil {
			t.Fatal(err)
		}
		at += 10 * time.Millisecond
	}
}

func TestLinuxDoubleAltListenerStopsQuietDevice(t *testing.T) {
	path, writer := makeFIFO(t)
	defer writer.Close()
	listener := NewLinuxDoubleAltListenerForPaths(350, func(hotkeys.Gesture) {}, []string{path})
	if ok, message := listener.Start(); !ok {
		t.Fatal(message)
	}
	listener.Stop()
}

func TestLinuxDoubleAltListenerTriggersFromKernelEvents(t *testing.T) {
	path, writer := makeFIFO(t)
	defer writer.Close()
	triggered := make(chan hotkeys.Gesture, 1)
	listener := NewLinuxDoubleAltListenerForPaths(350, func(gesture hotkeys.Gesture) { triggered <- gesture }, []string{path})
	if ok, message := listener.Start(); !ok {
		t.Fatal(message)
	}
	defer listener.Stop()

	writeTap(t, writer, time.Second)
	writeTap(t, writer, 1100*time.Millisecond)
	select {
	case <-triggered:
	case <-time.After(2 * time.Second):
		t.Fatal("double Alt did not trigger")
	}
}

func TestLinuxDoubleAltListenerReopensDeviceAfterReadFailure(t *testing.T) {
	path, writer := makeFIFO(t)
	triggered := make(chan hotkeys.Gesture, 1)
	listener := NewLinuxDoubleAltListenerForPaths(350, func(gesture hotkeys.Gesture) { triggered <- gesture }, []string{path})
	if ok, message := listener.Start(); !ok {
		writer.Close()
		t.Fatal(message)
	}
	// Leave an Alt press held from the first device lifetime. The failure must
	// clear that device's detector state before the same path is reopened.
	if _, err := writer.Write(EncodeInputEvent(time.Second, EVKey, testAlt, KeyPress)); err != nil {
		listener.Stop()
		writer.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		listener.Stop()
		t.Fatal(err)
	}

	// Give the manager a chance to observe EOF and clear the old device state.
	// The listener itself must remain alive while it reopens the same path.
	time.Sleep(400 * time.Millisecond)
	defer listener.Stop()
	var err error
	writer, err = os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	writeTap(t, writer, 2*time.Second)
	writeTap(t, writer, 2100*time.Millisecond)
	select {
	case <-triggered:
	case <-time.After(2 * time.Second):
		t.Fatal("restarted listener did not trigger")
	}
}
