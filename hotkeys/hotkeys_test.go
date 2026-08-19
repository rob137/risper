package hotkeys

import (
	"strings"
	"testing"
	"time"
)

func tap(t *testing.T, detector *Detector, device string, alt uint16, at time.Duration) bool {
	t.Helper()
	detector.HandleKey(device, alt, true, at)
	triggered, _ := detector.HandleKey(device, alt, false, at+10*time.Millisecond)
	return triggered
}

func TestDoubleAltUsesKernelTimestampsAndDeviceLocalState(t *testing.T) {
	detector := NewDetector(350 * time.Millisecond)
	if tap(t, detector, "keyboard", LeftAlt, 0) {
		t.Fatal("first tap triggered")
	}

	// A second tap delivered much later still belongs to the 100 ms kernel-time
	// window. A key held on another device must not pollute this device.
	detector.HandleKey("ydotoold", 30, true, 50*time.Millisecond)
	detector.HandleKey("ydotoold", 30, false, 60*time.Millisecond)
	if !tap(t, detector, "keyboard", LeftAlt, 100*time.Millisecond) {
		t.Fatal("second tap did not trigger from kernel timestamps")
	}
}

func TestDoubleAltResetsLostReleaseAfterDeviceReset(t *testing.T) {
	detector := NewDetector(350 * time.Millisecond)
	detector.HandleKey("keyboard", LeftAlt, true, 0)
	detector.ResetDevice("keyboard")

	if tap(t, detector, "keyboard", LeftAlt, time.Second) {
		t.Fatal("first tap after device reset triggered")
	}
	if !tap(t, detector, "keyboard", LeftAlt, 1100*time.Millisecond) {
		t.Fatal("listener did not recover after a lost release")
	}
}

func TestDoubleAltLogsDiscardedTapReasons(t *testing.T) {
	detector := NewDetector(350 * time.Millisecond)
	detector.HandleKey("keyboard", LeftAlt, true, 0)
	_, diagnostic := detector.HandleKey("keyboard", LeftAlt, false, 10*time.Millisecond)
	if !strings.Contains(diagnostic, "first tap") {
		t.Fatalf("first tap diagnostic = %q", diagnostic)
	}
	_, diagnostic = detector.HandleKey("keyboard", LeftAlt, true, 1000*time.Millisecond)
	if diagnostic != "" {
		t.Fatalf("press diagnostic = %q", diagnostic)
	}
	_, diagnostic = detector.HandleKey("keyboard", LeftAlt, false, 1010*time.Millisecond)
	if !strings.Contains(diagnostic, "outside window") {
		t.Fatalf("outside-window diagnostic = %q", diagnostic)
	}
}
