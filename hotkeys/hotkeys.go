// Package hotkeys contains input-event detectors that are independent of the
// Linux device reader. Keeping device state here makes it possible to test the
// gesture without opening /dev/input.
package hotkeys

import "time"

const (
	LeftAlt    uint16 = 56
	RightAlt   uint16 = 100
	LeftShift  uint16 = 42
	RightShift uint16 = 54
)

// Gesture is the outcome of one key event. Shift is the only modifier allowed
// to join the two Alt taps, and it selects the paste variant rather than
// discarding the gesture as a combination.
type Gesture int

const (
	GestureNone Gesture = iota
	GestureToggle
	GestureTogglePaste
)

type deviceState struct {
	keysDown       map[uint16]bool
	candidate      uint16
	candidateShift bool
	polluted       bool
	lastEvent      time.Duration
	lastTap        time.Duration
	lastTapShift   bool
	hasLastTap     bool
}

// Detector recognizes two clean Alt taps on the same input device. The
// timestamp must be the event timestamp supplied by the kernel, not the time
// at which a reader happened to schedule the event.
type Detector struct {
	Window     time.Duration
	StaleAfter time.Duration
	devices    map[string]*deviceState
}

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

func isAlt(key uint16) bool   { return key == LeftAlt || key == RightAlt }
func isShift(key uint16) bool { return key == LeftShift || key == RightShift }

// onlyShiftHeld reports that nothing except Shift is down, which is the
// condition for both starting and completing a clean tap.
func onlyShiftHeld(state *deviceState) bool {
	for key := range state.keysDown {
		if !isShift(key) {
			return false
		}
	}
	return true
}

func shiftHeld(state *deviceState) bool {
	for key := range state.keysDown {
		if isShift(key) {
			return true
		}
	}
	return false
}

func reset(state *deviceState) {
	state.keysDown = make(map[uint16]bool)
	state.candidate = 0
	state.candidateShift = false
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
func (detector *Detector) HandleKey(device string, key uint16, pressed bool, timestamp time.Duration) (Gesture, string) {
	if !isAlt(key) && !pressed {
		return detector.handleOtherRelease(device, key, timestamp)
	}
	state := detector.state(device)
	if state.lastEvent != 0 && timestamp >= state.lastEvent && timestamp-state.lastEvent > detector.StaleAfter {
		reset(state)
		state.hasLastTap = false
		state.lastEvent = timestamp
		if !pressed || !isAlt(key) {
			return GestureNone, "double-alt state reset after stale input"
		}
		// Treat the current Alt press as the beginning of a new tap.
	}
	state.lastEvent = timestamp

	if pressed {
		if state.keysDown[key] {
			return GestureNone, ""
		}
		switch {
		case isAlt(key) && onlyShiftHeld(state):
			state.candidate = key
			state.candidateShift = shiftHeld(state)
			state.polluted = false
		case isShift(key) && state.candidate == 0:
			// Shift pressed before the taps is part of the paste variant.
		default:
			state.polluted = true
		}
		state.keysDown[key] = true
		return GestureNone, ""
	}

	delete(state.keysDown, key)
	if !isAlt(key) || state.candidate != key {
		if onlyShiftHeld(state) {
			state.candidate = 0
			state.candidateShift = false
			state.polluted = false
		}
		return GestureNone, ""
	}

	pureTap := !state.polluted && onlyShiftHeld(state)
	tapShift := state.candidateShift
	state.candidate = 0
	state.candidateShift = false
	state.polluted = false
	if !pureTap {
		state.hasLastTap = false
		return GestureNone, "double-alt tap discarded: key combination was held"
	}

	shiftChanged := state.hasLastTap && tapShift != state.lastTapShift
	if !state.hasLastTap || shiftChanged || timestamp < state.lastTap || timestamp-state.lastTap > detector.Window {
		firstTap := !state.hasLastTap
		state.lastTap = timestamp
		state.lastTapShift = tapShift
		state.hasLastTap = true
		switch {
		case firstTap:
			return GestureNone, "double-alt first tap"
		case shiftChanged:
			return GestureNone, "double-alt tap discarded: shift changed between taps"
		default:
			return GestureNone, "double-alt tap outside window"
		}
	}
	state.hasLastTap = false
	if tapShift {
		return GestureTogglePaste, "shift double-alt triggered"
	}
	return GestureToggle, "double-alt triggered"
}

func (detector *Detector) handleOtherRelease(device string, key uint16, timestamp time.Duration) (Gesture, string) {
	state := detector.state(device)
	if state.lastEvent != 0 && timestamp >= state.lastEvent && timestamp-state.lastEvent > detector.StaleAfter {
		reset(state)
		state.hasLastTap = false
	}
	state.lastEvent = timestamp
	delete(state.keysDown, key)
	if onlyShiftHeld(state) && state.candidate == 0 {
		state.polluted = false
	}
	return GestureNone, ""
}
