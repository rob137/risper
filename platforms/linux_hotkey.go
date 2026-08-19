package platforms

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/rob137/risper/hotkeys"
)

const (
	EVKey              uint16 = 1
	KeyRelease         int32  = 0
	KeyPress           int32  = 1
	inputEventBytes           = 24 // struct input_event on the supported 64-bit Linux host
	devicePollInterval        = 250 * time.Millisecond
	readRetryInterval         = 20 * time.Millisecond
)

const (
	EV_KEY      = EVKey
	KEY_RELEASE = KeyRelease
	KEY_PRESS   = KeyPress
)

// InputEvent is the subset of Linux struct input_event needed by the
// detector. Timestamp comes from the kernel's monotonic event clock.
type InputEvent struct {
	Timestamp time.Duration
	Type      uint16
	Code      uint16
	Value     int32
}

// EncodeInputEvent is useful for tests and for callers that need to feed a
// real evdev-shaped event into a fixture device such as a FIFO.
func EncodeInputEvent(timestamp time.Duration, eventType, code uint16, value int32) []byte {
	data := make([]byte, inputEventBytes)
	seconds := timestamp / time.Second
	micros := (timestamp % time.Second) / time.Microsecond
	binary.LittleEndian.PutUint64(data[0:8], uint64(seconds))
	binary.LittleEndian.PutUint64(data[8:16], uint64(micros))
	binary.LittleEndian.PutUint16(data[16:18], eventType)
	binary.LittleEndian.PutUint16(data[18:20], code)
	binary.LittleEndian.PutUint32(data[20:24], uint32(value))
	return data
}

func decodeInputEvent(data []byte) InputEvent {
	seconds := int64(binary.LittleEndian.Uint64(data[0:8]))
	micros := int64(binary.LittleEndian.Uint64(data[8:16]))
	return InputEvent{
		Timestamp: time.Duration(seconds)*time.Second + time.Duration(micros)*time.Microsecond,
		Type:      binary.LittleEndian.Uint16(data[16:18]),
		Code:      binary.LittleEndian.Uint16(data[18:20]),
		Value:     int32(binary.LittleEndian.Uint32(data[20:24])),
	}
}

type deviceMessage struct {
	path  string
	event *InputEvent
	err   error
}

type LinuxDoubleAltListener struct {
	window       time.Duration
	onTrigger    func()
	onLog        func(string)
	devicePaths  []string
	stop         chan struct{}
	done         chan struct{}
	mu           sync.Mutex
	started      bool
	files        map[string]*os.File
	lastFailures map[string]string
}

// NewLinuxDoubleAltListener creates a listener over the current
// /dev/input/event* set. The listener refreshes that set while running, so an
// unplug/replug or suspend/resume does not require a daemon restart.
func NewLinuxDoubleAltListener(windowMS int, onTrigger func()) *LinuxDoubleAltListener {
	return newLinuxDoubleAltListener(windowMS, onTrigger, nil)
}

// NewLinuxDoubleAltListenerForPaths is the path-explicit form used by tests.
// It is also useful for a narrowly-scoped diagnostic fixture.
func NewLinuxDoubleAltListenerForPaths(windowMS int, onTrigger func(), paths []string) *LinuxDoubleAltListener {
	return newLinuxDoubleAltListener(windowMS, onTrigger, paths)
}

func newLinuxDoubleAltListener(windowMS int, onTrigger func(), paths []string) *LinuxDoubleAltListener {
	if windowMS < 100 {
		windowMS = 100
	}
	return &LinuxDoubleAltListener{
		window:       time.Duration(windowMS) * time.Millisecond,
		onTrigger:    onTrigger,
		onLog:        func(string) {},
		devicePaths:  append([]string(nil), paths...),
		files:        make(map[string]*os.File),
		lastFailures: make(map[string]string),
	}
}

// SetLogger adds diagnostics for both successful and discarded gesture
// attempts. The listener remains useful without one, which keeps the input
// package independent of the daemon's log format.
func (listener *LinuxDoubleAltListener) SetLogger(logger func(string)) {
	listener.mu.Lock()
	defer listener.mu.Unlock()
	if logger == nil {
		listener.onLog = func(string) {}
		return
	}
	listener.onLog = logger
}

func (listener *LinuxDoubleAltListener) Start() (bool, string) {
	listener.mu.Lock()
	defer listener.mu.Unlock()
	if listener.started {
		return true, "double-alt listener already running"
	}
	paths := listener.paths()
	var failures []string
	for _, path := range paths {
		if file, err := openInputDevice(path); err == nil {
			listener.files[path] = file
		} else {
			failures = append(failures, fmt.Sprintf("%s: %s", filepath.Base(path), err))
		}
	}
	if len(listener.files) == 0 {
		detail := "no /dev/input/event* devices found"
		if len(paths) > 0 {
			detail = "could not read any input device"
			if len(failures) > 0 {
				detail += ": " + failures[0]
			}
		}
		return false, "double-alt listener unavailable: " + detail
	}
	listener.stop = make(chan struct{})
	listener.done = make(chan struct{})
	listener.started = true
	go listener.run()
	return true, fmt.Sprintf("double-alt listener reading %d input device(s)", len(listener.files))
}

func (listener *LinuxDoubleAltListener) Stop() {
	listener.mu.Lock()
	if !listener.started {
		listener.mu.Unlock()
		return
	}
	stop := listener.stop
	done := listener.done
	files := make([]*os.File, 0, len(listener.files))
	for _, file := range listener.files {
		files = append(files, file)
	}
	listener.started = false
	listener.mu.Unlock()

	close(stop)
	for _, file := range files {
		_ = file.Close()
	}
	<-done

	listener.mu.Lock()
	listener.files = make(map[string]*os.File)
	listener.mu.Unlock()
}

func (listener *LinuxDoubleAltListener) paths() []string {
	if len(listener.devicePaths) > 0 {
		return append([]string(nil), listener.devicePaths...)
	}
	paths, _ := filepath.Glob("/dev/input/event*")
	return paths
}

func openInputDevice(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func (listener *LinuxDoubleAltListener) run() {
	defer close(listener.done)
	messages := make(chan deviceMessage, 32)
	var workers sync.WaitGroup
	for path, file := range listener.files {
		workers.Add(1)
		go listener.readDevice(path, file, messages, &workers)
	}
	defer func() {
		workers.Wait()
	}()

	detector := hotkeys.NewDetector(listener.window)
	ticker := time.NewTicker(devicePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-listener.stop:
			return
		case message := <-messages:
			if message.event != nil {
				triggered, diagnostic := detector.HandleKey(
					message.path,
					message.event.Code,
					message.event.Value == KeyPress,
					message.event.Timestamp,
				)
				if diagnostic != "" {
					listener.log(diagnostic)
				}
				if triggered && listener.onTrigger != nil {
					go listener.onTrigger()
				}
			}
			if message.err != nil {
				detector.ResetDevice(message.path)
				listener.removeDevice(message.path)
				listener.log(fmt.Sprintf("double-alt input device %s lost: %s; retrying", message.path, message.err))
			}
		case <-ticker.C:
			listener.reopenMissing(messages, &workers)
		}
	}
}

func (listener *LinuxDoubleAltListener) readDevice(path string, file *os.File, messages chan<- deviceMessage, workers *sync.WaitGroup) {
	defer workers.Done()
	defer file.Close()
	partial := make([]byte, 0, inputEventBytes*4)
	chunk := make([]byte, inputEventBytes*8)
	for {
		select {
		case <-listener.stop:
			return
		default:
		}
		n, err := file.Read(chunk)
		if n > 0 {
			partial = append(partial, chunk[:n]...)
			for len(partial) >= inputEventBytes {
				event := decodeInputEvent(partial[:inputEventBytes])
				partial = partial[inputEventBytes:]
				if event.Type != EVKey || (event.Value != KeyPress && event.Value != KeyRelease) {
					continue
				}
				select {
				case messages <- deviceMessage{path: path, event: &event}:
				case <-listener.stop:
					return
				}
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
			timer := time.NewTimer(readRetryInterval)
			select {
			case <-timer.C:
			case <-listener.stop:
				if !timer.Stop() {
					<-timer.C
				}
				return
			}
			continue
		}
		if errors.Is(err, io.EOF) {
			err = errors.New("end of input device")
		}
		select {
		case messages <- deviceMessage{path: path, err: err}:
		case <-listener.stop:
		}
		return
	}
}

func (listener *LinuxDoubleAltListener) removeDevice(path string) {
	listener.mu.Lock()
	defer listener.mu.Unlock()
	if file := listener.files[path]; file != nil {
		_ = file.Close()
		delete(listener.files, path)
	}
}

func (listener *LinuxDoubleAltListener) reopenMissing(messages chan<- deviceMessage, workers *sync.WaitGroup) {
	var logs []string
	listener.mu.Lock()
	if !listener.started {
		listener.mu.Unlock()
		return
	}
	paths := listener.paths()
	for _, path := range paths {
		if listener.files[path] != nil {
			continue
		}
		file, err := openInputDevice(path)
		if err != nil {
			message := err.Error()
			if listener.lastFailures[path] != message {
				listener.lastFailures[path] = message
				logs = append(logs, fmt.Sprintf("double-alt input device %s unavailable: %s", path, message))
			}
			continue
		}
		listener.files[path] = file
		listener.lastFailures[path] = ""
		workers.Add(1)
		go listener.readDevice(path, file, messages, workers)
	}
	listener.mu.Unlock()
	for _, message := range logs {
		listener.log(message)
	}
}

func (listener *LinuxDoubleAltListener) log(message string) {
	listener.mu.Lock()
	logger := listener.onLog
	listener.mu.Unlock()
	if logger != nil {
		logger(message)
	}
}
