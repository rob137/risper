package voice

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestMatchTriggerRequiresOneConfiguredWord(t *testing.T) {
	const start, stop, send = "quasar", "marzipan", "tangerine"
	for _, test := range []struct {
		text   string
		action Action
	}{
		{"Quasar.", ActionStart},
		{"MARZIPAN", ActionStop},
		{"tangerine!", ActionSend},
		{"please quasar", ActionNone},
		{"ordinary dictation", ActionNone},
	} {
		if got := MatchTrigger(test.text, start, stop, send); got != test.action {
			t.Errorf("MatchTrigger(%q) = %v, want %v", test.text, got, test.action)
		}
	}
}

func TestStripTrailingTriggerOnlyRemovesTheFinalWord(t *testing.T) {
	if got := StripTrailingTrigger("Make a note about marzipan.", "marzipan"); got != "Make a note about" {
		t.Fatalf("stripped transcript = %q", got)
	}
	if got := StripTrailingTrigger("marzipan", "marzipan"); got != "" {
		t.Fatalf("single trigger transcript = %q", got)
	}
	unchanged := "marzipan belongs in the note"
	if got := StripTrailingTrigger(unchanged, "marzipan"); got != unchanged {
		t.Fatalf("ordinary use changed to %q", got)
	}
}

func TestSegmenterUsesRelativeLoudnessGate(t *testing.T) {
	segmenter := NewSegmenter(10, 300*time.Millisecond)
	var segments [][]int16
	for i := 0; i < calibrationFrames; i++ {
		segments = append(segments, segmenter.Add(constantFrame(100))...)
	}
	for i := 0; i < 3; i++ {
		segments = append(segments, segmenter.Add(constantFrame(1000))...)
	}
	for i := 0; i < 3; i++ {
		segments = append(segments, segmenter.Add(constantFrame(100))...)
	}
	if len(segments) != 1 || len(segments[0]) != 3*frameSamples {
		t.Fatalf("segments = %d, samples = %d; want one three-frame burst", len(segments), len(segments[0]))
	}

	for _, pair := range [][2]int16{{100, 1000}, {300, 3000}} {
		gate := NewLoudnessGate(10)
		for i := 0; i < calibrationFrames; i++ {
			gate.Speech(constantFrame(pair[0]))
		}
		if !gate.Speech(constantFrame(pair[1])) {
			t.Fatalf("relative gate rejected %d -> %d level change", pair[0], pair[1])
		}
	}
}

func TestSegmenterSuppressesOverlongSpeechUntilQuiet(t *testing.T) {
	segmenter := NewSegmenter(10, 300*time.Millisecond)
	for i := 0; i < calibrationFrames; i++ {
		segmenter.Add(constantFrame(100))
	}

	for i := 0; i <= maxCandidateFrames; i++ {
		if segments := segmenter.Add(constantFrame(1000)); len(segments) != 0 {
			t.Fatalf("overlong speech produced %d segment(s)", len(segments))
		}
	}
	for i := 0; i < 3; i++ {
		if segments := segmenter.Add(constantFrame(100)); len(segments) != 0 {
			t.Fatalf("overlong speech closed into %d segment(s)", len(segments))
		}
	}

	var segments [][]int16
	for i := 0; i < 3; i++ {
		segments = append(segments, segmenter.Add(constantFrame(1000))...)
	}
	for i := 0; i < 3; i++ {
		segments = append(segments, segmenter.Add(constantFrame(100))...)
	}
	if len(segments) != 1 || len(segments[0]) != 3*frameSamples {
		t.Fatalf("new speech after quiet = %d segments, %d samples; want one three-frame burst", len(segments), len(segments[0]))
	}
}

func TestPCMStreamRemovesWavHeaderAndPreservesRawStreams(t *testing.T) {
	samples := []int16{1, -2, 300}
	wav := wavBytes(samples)
	stream, err := pcmStream(bytes.NewReader(wav))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, wav[44:]) {
		t.Fatalf("PCM data = %v, want %v", data, wav[44:])
	}

	raw := []byte("raw pcm")
	stream, err = pcmStream(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	data, err = io.ReadAll(stream)
	if err != nil || string(data) != string(raw) {
		t.Fatalf("raw stream = %q, err = %v", data, err)
	}
}

func TestWAVTriggerInputIsSmallAndInMemory(t *testing.T) {
	data := wavBytes(constantFrame(1000))
	if len(data) != 44+frameSamples*2 {
		t.Fatalf("WAV size = %d", len(data))
	}
	if !strings.HasPrefix(string(data), "RIFF") {
		t.Fatal("WAV does not have a RIFF header")
	}
}

func TestMicCommandDoesNotCaptureTheSystemMonitor(t *testing.T) {
	args := micCommand(context.Background()).Args
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "stream.capture.sink=true") {
		t.Fatalf("voice command unexpectedly captures the system monitor: %q", joined)
	}
	if args[len(args)-1] != "-" {
		t.Fatalf("voice command output = %q, want stdout", args[len(args)-1])
	}
}

func constantFrame(value int16) []int16 {
	frame := make([]int16, frameSamples)
	for i := range frame {
		frame[i] = value
	}
	return frame
}
