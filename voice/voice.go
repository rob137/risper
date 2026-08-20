// Package voice contains the opt-in voice-trigger listener. It deliberately
// keeps detection audio out of session folders: PCM is read from pw-record,
// held in memory for one short utterance, and sent to whisper.cpp over stdin.
package voice

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/models"
	"github.com/rob137/risper/transcription"
)

const (
	sampleRate         = 16000
	frameSamples       = sampleRate / 10
	calibrationFrames  = 5
	maxCandidateFrames = 25
)

type Action int

const (
	ActionNone Action = iota
	ActionStart
	ActionStop
	ActionSend
)

// Listener is the lifecycle boundary used by the daemon. The implementation
// is Linux/PipeWire-specific, while the interface keeps daemon tests hardware
// independent.
type Listener interface {
	Start() (bool, string)
	Stop()
	SetLogger(func(string))
}

type linuxListener struct {
	cfg      config.Config
	onAction func(Action)

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	logger func(string)
}

func NewListener(cfg config.Config, onAction func(Action)) Listener {
	return &linuxListener{cfg: cfg, onAction: onAction}
}

func (listener *linuxListener) SetLogger(logger func(string)) {
	listener.mu.Lock()
	listener.logger = logger
	listener.mu.Unlock()
}

func (listener *linuxListener) log(message string) {
	listener.mu.Lock()
	logger := listener.logger
	listener.mu.Unlock()
	if logger != nil {
		logger(message)
	}
}

func (listener *linuxListener) Start() (bool, string) {
	if !config.CommandExists("pw-record") {
		return false, "voice trigger unavailable: pw-record is not installed"
	}
	profile, err := triggerProfile(listener.cfg)
	if err != nil {
		return false, "voice trigger unavailable: " + err.Error()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	listener.mu.Lock()
	if listener.cancel != nil && listener.done != nil {
		select {
		case <-listener.done:
			listener.cancel = nil
			listener.done = nil
		default:
		}
	}
	if listener.cancel != nil {
		listener.mu.Unlock()
		cancel()
		return false, "voice trigger listener is already running"
	}
	listener.cancel = cancel
	listener.done = done
	listener.mu.Unlock()
	go listener.run(ctx, done, profile)
	return true, fmt.Sprintf("voice trigger listener started profile=%s gate=%.1fdB", profile.ID, listener.cfg.VoiceNoiseGateDB)
}

func (listener *linuxListener) Stop() {
	listener.mu.Lock()
	cancel := listener.cancel
	done := listener.done
	listener.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	if done != nil {
		<-done
	}
	listener.mu.Lock()
	listener.cancel = nil
	listener.done = nil
	listener.mu.Unlock()
}

func (listener *linuxListener) run(ctx context.Context, done chan struct{}, profile models.Profile) {
	defer close(done)
	cmd := micCommand(ctx)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		listener.log("voice trigger listener could not open pw-record stdout: " + err.Error())
		return
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		listener.log("voice trigger listener could not start pw-record: " + err.Error())
		return
	}

	segments := make(chan []int16, 2)
	var worker sync.WaitGroup
	worker.Add(1)
	go func() {
		defer worker.Done()
		listener.recognize(ctx, profile, segments)
	}()

	stream, err := pcmStream(stdout)
	if err != nil {
		listener.log("voice trigger listener could not read pw-record stream: " + err.Error())
	} else {
		listener.readFrames(stream, segments)
	}
	close(segments)
	worker.Wait()
	waitErr := cmd.Wait()
	if ctx.Err() == nil && waitErr != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			listener.log("voice trigger pw-record stopped: " + message)
		} else {
			listener.log("voice trigger pw-record stopped: " + waitErr.Error())
		}
	}
}

func micCommand(ctx context.Context) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "pw-record", "--rate", "16000", "--channels", "1", "--format", "s16", "--latency", "20ms", "-")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

func (listener *linuxListener) readFrames(stream io.Reader, segments chan<- []int16) {
	segmenter := NewSegmenter(listener.cfg.VoiceNoiseGateDB, time.Duration(listener.cfg.VoiceSilenceMS)*time.Millisecond)
	readBuffer := make([]byte, frameSamples*2)
	pending := make([]byte, 0, len(readBuffer)*2)
	for {
		n, err := stream.Read(readBuffer)
		if n > 0 {
			pending = append(pending, readBuffer[:n]...)
			for len(pending) >= len(readBuffer) {
				frame := pcm16(pending[:len(readBuffer)])
				pending = pending[len(readBuffer):]
				for _, segment := range segmenter.Add(frame) {
					select {
					case segments <- segment:
					default:
						listener.log("voice trigger utterance dropped while recognizer was busy")
					}
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				listener.log("voice trigger audio read stopped: " + err.Error())
			}
			break
		}
	}
	if segment := segmenter.Flush(); len(segment) > 0 {
		select {
		case segments <- segment:
		default:
		}
	}
}

func (listener *linuxListener) recognize(ctx context.Context, profile models.Profile, segments <-chan []int16) {
	profile.Prompt = strings.TrimSpace(profile.Prompt + " Trigger words: " + listener.cfg.VoiceStartWord + ", " + listener.cfg.VoiceStopWord + ", " + listener.cfg.VoiceSendWord + ".")
	for segment := range segments {
		if ctx.Err() != nil {
			return
		}
		text, err := transcription.TranscribeStdinContext(ctx, profile, wavBytes(segment))
		if err != nil {
			if ctx.Err() == nil {
				listener.log("voice trigger recognition failed: " + err.Error())
			}
			continue
		}
		action := MatchTrigger(text, listener.cfg.VoiceStartWord, listener.cfg.VoiceStopWord, listener.cfg.VoiceSendWord)
		if action != ActionNone && listener.onAction != nil {
			listener.onAction(action)
		}
	}
}

func triggerProfile(cfg config.Config) (models.Profile, error) {
	profiles, err := models.Load(cfg)
	if err != nil {
		return models.Profile{}, err
	}
	profile, ok := profiles[cfg.VoiceTriggerProfile]
	if !ok {
		return models.Profile{}, fmt.Errorf("profile %q is not configured in %s", cfg.VoiceTriggerProfile, cfg.ModelsPath)
	}
	if profile.Engine != "whisper.cpp" {
		return models.Profile{}, fmt.Errorf("profile %q uses engine %q; voice triggers require whisper.cpp", profile.ID, profile.Engine)
	}
	return profile, nil
}

// MatchTrigger only accepts an utterance that decoded to one word. This keeps
// a trigger word in ordinary dictation from firing merely because it appeared
// somewhere in a longer sentence.
func MatchTrigger(text, start, stop, send string) Action {
	words := normalizedWords(text)
	if len(words) != 1 {
		return ActionNone
	}
	switch words[0] {
	case strings.ToLower(start):
		return ActionStart
	case strings.ToLower(stop):
		return ActionStop
	case strings.ToLower(send):
		return ActionSend
	default:
		return ActionNone
	}
}

func normalizedWords(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// StripTrailingTrigger removes only a trigger word at the end of a completed
// transcript. It leaves ordinary uses elsewhere untouched.
func StripTrailingTrigger(text, word string) string {
	trimmed := strings.TrimSpace(text)
	end := len(trimmed)
	for end > 0 {
		r, size := utf8LastRune(trimmed[:end])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			break
		}
		end -= size
	}
	start := end
	for start > 0 {
		r, size := utf8LastRune(trimmed[:start])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			break
		}
		start -= size
	}
	if start == end || !strings.EqualFold(trimmed[start:end], word) {
		return text
	}
	return strings.TrimSpace(trimmed[:start])
}

func utf8LastRune(text string) (rune, int) {
	r, size := utf8.DecodeLastRuneInString(text)
	return r, size
}

// Segmenter turns 100 ms PCM frames into short speech bursts. Its gate is
// relative to the recent quiet level, never an absolute dBFS cutoff.
type Segmenter struct {
	gate          *LoudnessGate
	silenceFrames int
	maxFrames     int
	inSpeech      bool
	candidate     bool
	silence       int
	frames        int
	samples       []int16
}

func NewSegmenter(gateDB float64, silence time.Duration) *Segmenter {
	frames := int(math.Round(silence.Seconds() * 10))
	if frames < 2 {
		frames = 2
	}
	return &Segmenter{gate: NewLoudnessGate(gateDB), silenceFrames: frames, maxFrames: maxCandidateFrames}
}

func (segmenter *Segmenter) Add(frame []int16) [][]int16 {
	speech := segmenter.gate.Speech(frame)
	if speech {
		if !segmenter.inSpeech {
			segmenter.inSpeech = true
			segmenter.candidate = true
			segmenter.silence = 0
			segmenter.frames = 0
			segmenter.samples = nil
		}
		segmenter.silence = 0
		if segmenter.candidate {
			segmenter.samples = append(segmenter.samples, frame...)
			segmenter.frames++
			if segmenter.frames > segmenter.maxFrames {
				// A one-word trigger should fit comfortably inside this
				// window. Keep listening for the quiet boundary, but do not
				// spend a recognition pass on a burst that cannot match.
				segmenter.candidate = false
				segmenter.samples = nil
			}
		}
		return nil
	}
	if !segmenter.inSpeech {
		return nil
	}
	segmenter.silence++
	if segmenter.silence >= segmenter.silenceFrames {
		if !segmenter.candidate {
			segmenter.takeOne()
			return nil
		}
		return segmenter.take()
	}
	return nil
}

func (segmenter *Segmenter) Flush() []int16 {
	if !segmenter.inSpeech || !segmenter.candidate {
		if segmenter.inSpeech {
			segmenter.takeOne()
		}
		return nil
	}
	return segmenter.takeOne()
}

func (segmenter *Segmenter) take() [][]int16 {
	return [][]int16{segmenter.takeOne()}
}

func (segmenter *Segmenter) takeOne() []int16 {
	result := append([]int16(nil), segmenter.samples...)
	segmenter.inSpeech = false
	segmenter.candidate = false
	segmenter.silence = 0
	segmenter.frames = 0
	segmenter.samples = nil
	return result
}

type LoudnessGate struct {
	thresholdDB float64
	calibration []float64
	floorDB     float64
	ready       bool
}

func NewLoudnessGate(thresholdDB float64) *LoudnessGate {
	if thresholdDB < 0 {
		thresholdDB = 0
	}
	return &LoudnessGate{thresholdDB: thresholdDB}
}

func (gate *LoudnessGate) Speech(frame []int16) bool {
	db := rmsDB(frame)
	if !gate.ready {
		gate.calibration = append(gate.calibration, db)
		if len(gate.calibration) < calibrationFrames {
			return false
		}
		gate.floorDB = median(gate.calibration)
		gate.ready = true
		return false
	}
	speech := db >= gate.floorDB+gate.thresholdDB
	if !speech {
		// Slow tracking keeps the threshold relative while avoiding a speech
		// burst dragging the floor upward during the burst.
		gate.floorDB += (db - gate.floorDB) * 0.05
	}
	return speech
}

func rmsDB(samples []int16) float64 {
	if len(samples) == 0 {
		return -100
	}
	var sum float64
	for _, sample := range samples {
		value := float64(sample) / 32768
		sum += value * value
	}
	rms := math.Sqrt(sum / float64(len(samples)))
	if rms <= 0 {
		return -100
	}
	return 20 * math.Log10(rms)
}

func median(values []float64) float64 {
	copyValues := append([]float64(nil), values...)
	for i := 1; i < len(copyValues); i++ {
		for j := i; j > 0 && copyValues[j] < copyValues[j-1]; j-- {
			copyValues[j], copyValues[j-1] = copyValues[j-1], copyValues[j]
		}
	}
	return copyValues[len(copyValues)/2]
}

func pcm16(data []byte) []int16 {
	samples := make([]int16, len(data)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return samples
}

func wavBytes(samples []int16) []byte {
	pcm := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(sample))
	}
	dataSize := uint32(len(pcm))
	result := make([]byte, 44+len(pcm))
	copy(result[0:4], "RIFF")
	binary.LittleEndian.PutUint32(result[4:8], 36+dataSize)
	copy(result[8:12], "WAVE")
	copy(result[12:16], "fmt ")
	binary.LittleEndian.PutUint32(result[16:20], 16)
	binary.LittleEndian.PutUint16(result[20:22], 1)
	binary.LittleEndian.PutUint16(result[22:24], 1)
	binary.LittleEndian.PutUint32(result[24:28], sampleRate)
	binary.LittleEndian.PutUint32(result[28:32], sampleRate*2)
	binary.LittleEndian.PutUint16(result[32:34], 2)
	binary.LittleEndian.PutUint16(result[34:36], 16)
	copy(result[36:40], "data")
	binary.LittleEndian.PutUint32(result[40:44], dataSize)
	copy(result[44:], pcm)
	return result
}

func pcmStream(input io.Reader) (io.Reader, error) {
	header := make([]byte, 12)
	n, err := io.ReadFull(input, header)
	if err != nil {
		if n > 0 {
			return io.MultiReader(bytes.NewReader(header[:n]), input), nil
		}
		return nil, err
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return io.MultiReader(bytes.NewReader(header), input), nil
	}
	for {
		chunk := make([]byte, 8)
		if _, err := io.ReadFull(input, chunk); err != nil {
			return nil, err
		}
		size := binary.LittleEndian.Uint32(chunk[4:8])
		if string(chunk[0:4]) == "data" {
			if size == 0 || size == math.MaxUint32 {
				return input, nil
			}
			return io.LimitReader(input, int64(size)), nil
		}
		skip := int64(size)
		if skip%2 != 0 {
			skip++
		}
		if _, err := io.CopyN(io.Discard, input, skip); err != nil {
			return nil, err
		}
	}
}
