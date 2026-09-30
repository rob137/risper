package meeting

import (
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/internal/files"
	"github.com/rob137/risper/recording"
	"github.com/rob137/risper/session"
)

func TestQuietestBoundarySeeksNearbySilence(t *testing.T) {
	root := t.TempDir()
	pcm := constantPCM(40*time.Second, 4000)
	for sample := 24 * 16000; sample < 25*16000; sample++ {
		binary.LittleEndian.PutUint16(pcm[sample*2:sample*2+2], 0)
	}
	path := filepath.Join(root, "audio.mic.wav")
	if err := writeWAV(path, pcm); err != nil {
		t.Fatal(err)
	}
	boundary, err := quietestBoundary(map[string]string{"mic": path}, 20*pcmBytesSecond, 40*pcmBytesSecond)
	if err != nil {
		t.Fatal(err)
	}
	seconds := float64(boundary) / float64(pcmBytesSecond)
	if seconds < 24 || seconds > 25 {
		t.Fatalf("quiet boundary = %.3fs, want the 24s-25s silence", seconds)
	}
}

func TestProcessAvailableTranscribesRollingSliceAndStitchesTranscript(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "session")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	part := filepath.Join(sessionDir, "audio.mic.wav")
	if err := writeWAV(part, constantPCM(31*time.Second, 6000)); err != nil {
		t.Fatal(err)
	}
	modelsPath := filepath.Join(root, "models.toml")
	if err := os.WriteFile(modelsPath, []byte(`[models.pinned]
engine = "external"
model = "stub"
language = "en"
command = "printf 'rolling meeting words'"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{ModelsPath: modelsPath, Language: "en"}
	recorder := &recording.State{SessionDir: sessionDir, PartPaths: map[string]string{"mic": part}}
	state := &State{
		Version: 1, Status: "recording", SessionDir: sessionDir, ProfileID: "pinned",
		SliceSeconds: 30, SilenceDB: -80, StartedAt: time.Now().Format(time.RFC3339), Slices: []Slice{},
	}
	if err := saveState(state); err != nil {
		t.Fatal(err)
	}
	if err := processAvailable(cfg, recorder, state, false); err != nil {
		t.Fatal(err)
	}
	if len(state.Slices) != 1 || state.Slices[0].Status != "transcribed" {
		t.Fatalf("slices = %#v", state.Slices)
	}
	data, err := os.ReadFile(filepath.Join(sessionDir, "transcript.clean.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != "rolling meeting words" {
		t.Fatalf("stitched transcript = %q", got)
	}
	if _, err := os.Stat(filepath.Join(sessionDir, slicesDir, "000001.wav")); !os.IsNotExist(err) {
		t.Fatalf("temporary PCM slice was retained: %v", err)
	}
}

func TestForcedProcessKeepsBacklogInBoundedSlices(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "session")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	part := filepath.Join(sessionDir, "audio.mic.wav")
	if err := writeWAV(part, constantPCM(75*time.Second, 6000)); err != nil {
		t.Fatal(err)
	}
	modelsPath := filepath.Join(root, "models.toml")
	if err := os.WriteFile(modelsPath, []byte(`[models.pinned]
engine = "external"
model = "stub"
language = "en"
command = "printf 'bounded slice'"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{ModelsPath: modelsPath, Language: "en"}
	recorder := &recording.State{SessionDir: sessionDir, PartPaths: map[string]string{"mic": part}}
	state := &State{
		Version: 1, Status: "recording", SessionDir: sessionDir, ProfileID: "pinned",
		SliceSeconds: 30, SilenceDB: -80, StartedAt: time.Now().Format(time.RFC3339), Slices: []Slice{},
	}
	if err := saveState(state); err != nil {
		t.Fatal(err)
	}
	if err := processAvailable(cfg, recorder, state, true); err != nil {
		t.Fatal(err)
	}
	if len(state.Slices) < 2 {
		t.Fatalf("forced backlog became %d slice(s), want multiple bounded slices", len(state.Slices))
	}
	for _, slice := range state.Slices {
		if duration := slice.EndSeconds - slice.StartSeconds; duration > 40.01 {
			t.Fatalf("slice %d duration = %.2fs, want at most target plus silence-search radius", slice.Index, duration)
		}
	}
	if state.NextPCMByte != 75*pcmBytesSecond {
		t.Fatalf("processed bytes = %d, want %d", state.NextPCMByte, 75*pcmBytesSecond)
	}
}

func TestSilentSliceSkipsTranscription(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "session")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	part := filepath.Join(sessionDir, "audio.mic.wav")
	if err := writeWAV(part, make([]byte, int(2*pcmBytesSecond))); err != nil {
		t.Fatal(err)
	}
	state := &State{SessionDir: sessionDir, ProfileID: "missing", SliceSeconds: 30, SilenceDB: -50, StartedAt: time.Now().Format(time.RFC3339)}
	recorder := &recording.State{SessionDir: sessionDir, PartPaths: map[string]string{"mic": part}}
	slice := transcribeSlice(config.Config{}, recorder, state, 0, 2*pcmBytesSecond)
	if slice.Status != "silent" || slice.Error != "" {
		t.Fatalf("silent slice = %#v", slice)
	}
}

func TestCheckpointContainsOnlyUncopiedDeltaAndMarksGaps(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	if err := os.WriteFile(first, []byte("old words\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("new words\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := &State{
		StartedAt: "2026-09-30T10:00:00+01:00", NextPCMByte: 90 * pcmBytesSecond, LastCheckpointSlice: 1,
		Slices: []Slice{
			{Index: 1, Status: "transcribed", EndSeconds: 30, TranscriptPath: first},
			{Index: 2, Status: "transcribed", StartSeconds: 30, EndSeconds: 60, TranscriptPath: second},
			{Index: 3, Status: "failed", StartSeconds: 60, EndSeconds: 90, Error: "network down"},
		},
	}
	text := checkpointText(state)
	if strings.Contains(text, "old words") || !strings.Contains(text, "new words") || !strings.Contains(text, "Transcription gap 60.0s–90.0s") {
		t.Fatalf("checkpoint = %q", text)
	}
}

func TestCheckpointCopiesDeltaWithoutStoppingRecorder(t *testing.T) {
	root, cfg, clipboard := meetingTestEnvironment(t)
	sessionDir := filepath.Join(root, "session")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	part := filepath.Join(sessionDir, "audio.mic.wav")
	if err := writeWAV(part, constantPCM(2*time.Second, 6000)); err != nil {
		t.Fatal(err)
	}
	recorder := &recording.State{
		SessionDir: sessionDir, RecorderPIDs: map[string]int{"mic": os.Getpid()},
		PartPaths: map[string]string{"mic": part}, StartedAt: time.Now().Format(time.RFC3339), Mode: "meeting",
	}
	if err := files.AtomicWriteJSON(cfg.CurrentStatePath, recorder); err != nil {
		t.Fatal(err)
	}
	state := &State{
		Version: 1, Status: "recording", SessionDir: sessionDir, ProfileID: "pinned",
		SliceSeconds: 30, SilenceDB: -80, StartedAt: recorder.StartedAt, Slices: []Slice{},
	}
	if err := saveState(state); err != nil {
		t.Fatal(err)
	}

	if err := Checkpoint(cfg); err != nil {
		t.Fatal(err)
	}
	current, err := recording.Current(cfg)
	if err != nil || current == nil || current.Mode != "meeting" {
		t.Fatalf("checkpoint stopped recorder: current=%#v err=%v", current, err)
	}
	data, err := os.ReadFile(clipboard)
	if err != nil {
		t.Fatal(err)
	}
	if text := string(data); !strings.Contains(text, "Risper meeting checkpoint") || !strings.Contains(text, "meeting words") {
		t.Fatalf("clipboard = %q", text)
	}
	if err := Checkpoint(cfg); err != nil {
		t.Fatal(err)
	}
	unchanged, err := os.ReadFile(clipboard)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != string(data) {
		t.Fatalf("empty checkpoint replaced clipboard: before=%q after=%q", data, unchanged)
	}
	saved, err := loadState(sessionDir)
	if err != nil || saved.LastCheckpointSlice != 1 {
		t.Fatalf("saved meeting state = %#v, %v", saved, err)
	}
}

func TestPromptEchoIsVisibleAsTranscriptWarning(t *testing.T) {
	slices := []Slice{{Index: 1, Status: "prompt_echo", StartSeconds: 10, EndSeconds: 20}}
	if got := transcriptForSlices(slices); !strings.Contains(got, "Possible prompt echo omitted 10.0s–20.0s") {
		t.Fatalf("prompt echo transcript = %q", got)
	}
	errors := failedSliceErrors(slices)
	if len(errors) != 1 || !strings.Contains(errors[0], "possible prompt echo") {
		t.Fatalf("prompt echo errors = %#v", errors)
	}
}

func TestStopFinalizesMeetingAndCopiesLastDelta(t *testing.T) {
	_, cfg, clipboard := meetingTestEnvironment(t)
	metadata, err := session.CreateAt(cfg, time.Now().Add(-2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	metadata.RecordingMode = "meeting"
	metadata.Errors = []string{"pre-existing audio warning"}
	if err := session.SaveMetadata(metadata); err != nil {
		t.Fatal(err)
	}
	sessionDir := session.SessionDir(metadata)
	part := recording.SourcePath(metadata.AudioPath, recording.Mic)
	if err := writeWAV(part, constantPCM(2*time.Second, 6000)); err != nil {
		t.Fatal(err)
	}
	recorderProcess := exec.Command("sleep", "60")
	recorderProcess.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := recorderProcess.Start(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- recorderProcess.Wait() }()
	t.Cleanup(func() {
		_ = syscall.Kill(-recorderProcess.Process.Pid, syscall.SIGKILL)
		select {
		case <-waited:
		case <-time.After(time.Second):
		}
	})
	recorder := &recording.State{
		SessionDir: sessionDir, MetadataPath: session.MetadataPath(sessionDir), AudioPath: metadata.AudioPath,
		Sources: []recording.Source{recording.Mic}, RecorderPIDs: map[string]int{"mic": recorderProcess.Process.Pid},
		PartPaths: map[string]string{"mic": part}, RecorderBackend: "test", StartedAt: metadata.StartedAt, Mode: "meeting",
	}
	if err := files.AtomicWriteJSON(cfg.CurrentStatePath, recorder); err != nil {
		t.Fatal(err)
	}
	state := &State{
		Version: 1, Status: "recording", SessionDir: sessionDir, ProfileID: "pinned",
		SliceSeconds: 30, SilenceDB: -80, StartedAt: metadata.StartedAt, Slices: []Slice{},
	}
	if err := saveState(state); err != nil {
		t.Fatal(err)
	}

	if err := Stop(cfg); err != nil {
		t.Fatal(err)
	}
	completed, err := session.LoadSession(sessionDir)
	if err != nil || completed == nil || completed.Status != "complete" || completed.RecordingMode != "meeting" {
		t.Fatalf("completed metadata = %#v, %v", completed, err)
	}
	if len(completed.Errors) != 1 || completed.Errors[0] != "pre-existing audio warning" {
		t.Fatalf("completed errors = %#v", completed.Errors)
	}
	if data, err := os.ReadFile(completed.TranscriptCleanPath); err != nil || strings.TrimSpace(string(data)) != "meeting words" {
		t.Fatalf("transcript = %q, %v", data, err)
	}
	if data, err := os.ReadFile(clipboard); err != nil || !strings.Contains(string(data), "meeting words") {
		t.Fatalf("clipboard = %q, %v", data, err)
	}
	saved, err := loadState(sessionDir)
	if err != nil || saved.Status != "complete" || saved.EndedAt == "" {
		t.Fatalf("saved meeting state = %#v, %v", saved, err)
	}
	if current, err := recording.Current(cfg); err != nil || current != nil {
		t.Fatalf("recording remained active: %#v, %v", current, err)
	}
}

func TestDBFS(t *testing.T) {
	if got := dbFS(constantPCM(time.Second, 3276)); math.Abs(got-(-20)) > 0.1 {
		t.Fatalf("dbFS = %.3f, want about -20", got)
	}
	if !math.IsInf(dbFS(make([]byte, 320)), -1) {
		t.Fatal("digital silence should be -Inf dBFS")
	}
}

func constantPCM(duration time.Duration, value int16) []byte {
	samples := int(duration.Seconds() * 16000)
	pcm := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		binary.LittleEndian.PutUint16(pcm[i*2:i*2+2], uint16(value))
	}
	return pcm
}

func meetingTestEnvironment(t *testing.T) (string, config.Config, string) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	clipboard := filepath.Join(root, "clipboard.txt")
	if err := os.WriteFile(filepath.Join(bin, "wl-copy"), []byte("#!/bin/sh\ncat > \"$CLIPBOARD_PATH\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "notify-send"), []byte("#!/bin/sh\nprintf '1\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	t.Setenv("CLIPBOARD_PATH", clipboard)
	modelsPath := filepath.Join(root, "models.toml")
	if err := os.WriteFile(modelsPath, []byte(`[models.pinned]
engine = "external"
model = "stub"
language = "en"
command = "printf 'meeting words'"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(root, "state")
	return root, config.Config{
		SessionsDir: filepath.Join(root, "sessions"), StateDir: stateDir,
		CurrentStatePath: filepath.Join(stateDir, "current-recording.json"),
		ModelsPath:       modelsPath, Language: "en", Model: "stub", TranscriptionEngine: "external",
	}, clipboard
}
