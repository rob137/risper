// Package hotkeys contains input-event detectors that are independent of the
// Linux device reader. Keeping device state here makes it possible to test the
// gesture without opening /dev/input.
package hotkeys

import "time"

const (
	LeftAlt  uint16 = 56
	RightAlt uint16 = 100
)

type deviceState struct {
	keysDown   map[uint16]bool
	candidate  uint16
	polluted   bool
	lastEvent  time.Duration
	lastTap    time.Duration
	hasLastTap bool
}

// Detector recognizes two clean Alt taps on the same input device. The
// timestamp must be the event timestamp supplied by the kernel, not the time
// at which a reader happened to schedule the event.
type Detector struct {
	Window     time.Duration
	StaleAfter time.Duration
	devices    map[string]*deviceState
}

type DoubleAltDetector = Detector

func NewDoubleAltDetector(window time.Duration) *Detector { return NewDetector(window) }

func NewDetector(window time.Duration) *Detector {
	if window <= 0 {
		window = 350 * time.Millisecond
	}
	staleAfter := 2 * window
	if staleAfter < time.Second {
		staleAfter = time.Second
	}
	return &Detector{
		Window:     window,
		StaleAfter: staleAfter,
		devices:    make(map[string]*deviceState),
	}
}

func (detector *Detector) state(device string) *deviceState {
	if detector.devices == nil {
		detector.devices = make(map[string]*deviceState)
	}
	state := detector.devices[device]
	if state == nil {
		state = &deviceState{keysDown: make(map[uint16]bool)}
		detector.devices[device] = state
	}
	return state
}

func isAlt(key uint16) bool { return key == LeftAlt || key == RightAlt }

func reset(state *deviceState) {
	state.keysDown = make(map[uint16]bool)
	state.candidate = 0
	state.polluted = false
}

// ResetDevice forgets keys held by a device that disappeared or returned an
// input read error. Without this boundary, one lost release can poison the
// detector until the whole daemon is restarted.
func (detector *Detector) ResetDevice(device string) {
	delete(detector.devices, device)
}

// HandleKey consumes one EV_KEY press or release. It returns a non-empty
// diagnostic for gesture-level events so the daemon can log both successful
// and discarded taps. Ordinary non-Alt keys return an empty diagnostic.
func (detector *Detector) HandleKey(device string, key uint16, pressed bool, timestamp time.Duration) (bool, string) {
	if !isAlt(key) && !pressed {
		return detector.handleOtherRelease(device, key, timestamp)
	}
	state := detector.state(device)
	if state.lastEvent != 0 && timestamp >= state.lastEvent && timestamp-state.lastEvent > detector.StaleAfter {
		reset(state)
		state.hasLastTap = false
		state.lastEvent = timestamp
		if !pressed || !isAlt(key) {
			return false, "double-alt state reset after stale input"
		}
		// Treat the current Alt press as the beginning of a new tap.
	}
	state.lastEvent = timestamp

	if pressed {
		if state.keysDown[key] {
			return false, ""
		}
		if isAlt(key) && len(state.keysDown) == 0 {
			state.candidate = key
			state.polluted = false
		} else {
			state.polluted = true
		}
		state.keysDown[key] = true
		return false, ""
	}

	delete(state.keysDown, key)
	if !isAlt(key) || state.candidate != key {
		if len(state.keysDown) == 0 {
			state.candidate = 0
			state.polluted = false
		}
		return false, ""
	}

	pureTap := !state.polluted && len(state.keysDown) == 0
	state.candidate = 0
	state.polluted = false
	if !pureTap {
		state.hasLastTap = false
		return false, "double-alt tap discarded: key combination was held"
	}

	if !state.hasLastTap || timestamp < state.lastTap || timestamp-state.lastTap > detector.Window {
		firstTap := !state.hasLastTap
		state.lastTap = timestamp
		state.hasLastTap = true
		if firstTap {
			return false, "double-alt first tap"
		}
		return false, "double-alt tap outside window"
	}
	state.hasLastTap = false
	return true, "double-alt triggered"
}

func (detector *Detector) handleOtherRelease(device string, key uint16, timestamp time.Duration) (bool, string) {
	state := detector.state(device)
	if state.lastEvent != 0 && timestamp >= state.lastEvent && timestamp-state.lastEvent > detector.StaleAfter {
		reset(state)
		state.hasLastTap = false
	}
	state.lastEvent = timestamp
	delete(state.keysDown, key)
	if len(state.keysDown) == 0 {
		state.candidate = 0
		state.polluted = false
	}
	return false, ""
}
