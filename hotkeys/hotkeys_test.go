package hotkeys

import (
	"strings"
	"testing"
	"time"
)

func tap(t *testing.T, detector *Detector, device string, alt uint16, at time.Duration) Gesture {
	t.Helper()
	detector.HandleKey(device, alt, true, at)
	gesture, _ := detector.HandleKey(device, alt, false, at+10*time.Millisecond)
	return gesture
}

func TestDoubleAltUsesKernelTimestampsAndDeviceLocalState(t *testing.T) {
	detector := NewDetector(350 * time.Millisecond)
	if tap(t, detector, "keyboard", LeftAlt, 0) != GestureNone {
		t.Fatal("first tap triggered")
	}

	// A second tap delivered much later still belongs to the 100 ms kernel-time
	// window. A key held on another device must not pollute this device.
	detector.HandleKey("ydotoold", 30, true, 50*time.Millisecond)
	detector.HandleKey("ydotoold", 30, false, 60*time.Millisecond)
	if tap(t, detector, "keyboard", LeftAlt, 100*time.Millisecond) != GestureToggle {
		t.Fatal("second tap did not trigger from kernel timestamps")
	}
}

func TestDoubleAltResetsLostReleaseAfterDeviceReset(t *testing.T) {
	detector := NewDetector(350 * time.Millisecond)
	detector.HandleKey("keyboard", LeftAlt, true, 0)
	detector.ResetDevice("keyboard")

	if tap(t, detector, "keyboard", LeftAlt, time.Second) != GestureNone {
		t.Fatal("first tap after device reset triggered")
	}
	if tap(t, detector, "keyboard", LeftAlt, 1100*time.Millisecond) != GestureToggle {
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

// shiftTap performs one Alt tap with Shift already held, which is how the
// paste variant is produced in practice.
func shiftTap(t *testing.T, detector *Detector, device string, alt uint16, at time.Duration) Gesture {
	t.Helper()
	detector.HandleKey(device, LeftShift, true, at)
	detector.HandleKey(device, alt, true, at+5*time.Millisecond)
	gesture, _ := detector.HandleKey(device, alt, false, at+10*time.Millisecond)
	detector.HandleKey(device, LeftShift, false, at+15*time.Millisecond)
	return gesture
}

func TestShiftDoubleAltRequestsPaste(t *testing.T) {
	detector := NewDetector(350 * time.Millisecond)
	if shiftTap(t, detector, "keyboard", LeftAlt, 0) != GestureNone {
		t.Fatal("first shift tap triggered")
	}
	if shiftTap(t, detector, "keyboard", LeftAlt, 100*time.Millisecond) != GestureTogglePaste {
		t.Fatal("second shift tap did not request paste")
	}
}

func TestShiftHeldAcrossBothTapsWithoutRelease(t *testing.T) {
	detector := NewDetector(350 * time.Millisecond)
	detector.HandleKey("keyboard", LeftShift, true, 0)
	detector.HandleKey("keyboard", LeftAlt, true, 10*time.Millisecond)
	if gesture, _ := detector.HandleKey("keyboard", LeftAlt, false, 20*time.Millisecond); gesture != GestureNone {
		t.Fatal("first tap triggered")
	}
	detector.HandleKey("keyboard", LeftAlt, true, 100*time.Millisecond)
	gesture, diagnostic := detector.HandleKey("keyboard", LeftAlt, false, 110*time.Millisecond)
	if gesture != GestureTogglePaste {
		t.Fatalf("gesture = %v diagnostic = %q", gesture, diagnostic)
	}
}

func TestShiftChangingBetweenTapsDoesNotTrigger(t *testing.T) {
	detector := NewDetector(350 * time.Millisecond)
	if tap(t, detector, "keyboard", LeftAlt, 0) != GestureNone {
		t.Fatal("first tap triggered")
	}
	if shiftTap(t, detector, "keyboard", LeftAlt, 100*time.Millisecond) != GestureNone {
		t.Fatal("mixed taps triggered")
	}
	// The mismatched tap becomes the new first tap, so the gesture is still
	// reachable without waiting out the window.
	if shiftTap(t, detector, "keyboard", LeftAlt, 200*time.Millisecond) != GestureTogglePaste {
		t.Fatal("shift taps did not recover after a mismatch")
	}
}

func TestOtherModifiersStillDiscardTheGesture(t *testing.T) {
	detector := NewDetector(350 * time.Millisecond)
	control := uint16(29)
	detector.HandleKey("keyboard", control, true, 0)
	detector.HandleKey("keyboard", LeftAlt, true, 10*time.Millisecond)
	if gesture, _ := detector.HandleKey("keyboard", LeftAlt, false, 20*time.Millisecond); gesture != GestureNone {
		t.Fatal("ctrl+alt tap counted as a tap")
	}
	detector.HandleKey("keyboard", LeftAlt, true, 100*time.Millisecond)
	if gesture, _ := detector.HandleKey("keyboard", LeftAlt, false, 110*time.Millisecond); gesture != GestureNone {
		t.Fatal("ctrl held triggered the gesture")
	}
}

func TestShiftDoubleAltSurvivesAQuietKeyboard(t *testing.T) {
	detector := NewDetector(350 * time.Millisecond)
	// Any earlier keystroke leaves lastEvent set, so the Shift press that
	// starts the gesture arrives after the stale-input window has passed.
	detector.HandleKey("keyboard", 30, true, 0)
	detector.HandleKey("keyboard", 30, false, 10*time.Millisecond)

	quiet := 10 * time.Second
	if shiftTap(t, detector, "keyboard", LeftAlt, quiet) != GestureNone {
		t.Fatal("first shift tap triggered")
	}
	gesture, diagnostic := func() (Gesture, string) {
		detector.HandleKey("keyboard", LeftShift, true, quiet+100*time.Millisecond)
		detector.HandleKey("keyboard", LeftAlt, true, quiet+105*time.Millisecond)
		return detector.HandleKey("keyboard", LeftAlt, false, quiet+110*time.Millisecond)
	}()
	if gesture != GestureTogglePaste {
		t.Fatalf("gesture after a quiet keyboard = %v (%s), want paste", gesture, diagnostic)
	}
}
