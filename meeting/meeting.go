// Package meeting implements long-running, checkpointed transcription on top
// of Risper's durable two-source recorder.
package meeting

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/desktop"
	"github.com/rob137/risper/events"
	"github.com/rob137/risper/internal/files"
	"github.com/rob137/risper/models"
	"github.com/rob137/risper/recording"
	"github.com/rob137/risper/session"
	"github.com/rob137/risper/transcription"
)

const (
	stateFile       = "meeting.json"
	lockFile        = "meeting.lock"
	slicesDir       = "meeting-slices"
	workerLogFile   = "meeting-worker.log"
	pcmBytesSecond  = int64(16000 * 2)
	minimumTail     = pcmBytesSecond / 2
	workerPoll      = 2 * time.Second
	promptTailRunes = 500
)

type State struct {
	Version             int     `json:"version"`
	Status              string  `json:"status"`
	SessionDir          string  `json:"session_dir"`
	ProfileID           string  `json:"profile_id"`
	SliceSeconds        int     `json:"slice_seconds"`
	SilenceDB           float64 `json:"silence_db"`
	StartedAt           string  `json:"started_at"`
	EndedAt             string  `json:"ended_at,omitempty"`
	NextPCMByte         int64   `json:"next_pcm_byte"`
	LastCheckpointSlice int     `json:"last_checkpoint_slice"`
	WorkerPID           int     `json:"worker_pid,omitempty"`
	Slices              []Slice `json:"slices"`
}

type Slice struct {
	Index          int     `json:"index"`
	StartPCMByte   int64   `json:"start_pcm_byte"`
	EndPCMByte     int64   `json:"end_pcm_byte"`
	StartSeconds   float64 `json:"start_seconds"`
	EndSeconds     float64 `json:"end_seconds"`
	Status         string  `json:"status"`
	TranscriptPath string  `json:"transcript_path,omitempty"`
	ProfileID      string  `json:"profile_id,omitempty"`
	Engine         string  `json:"engine,omitempty"`
	Error          string  `json:"error,omitempty"`
}

func Main(args []string) int {
	parser := flag.NewFlagSet("risper meeting", flag.ContinueOnError)
	parser.SetOutput(os.Stderr)
	if err := parser.Parse(args); err != nil {
		return 2
	}
	if parser.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "risper meeting: want start, checkpoint, stop, or status")
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	switch parser.Arg(0) {
	case "start":
		err = Start(cfg)
	case "checkpoint":
		err = Checkpoint(cfg)
	case "stop":
		err = Stop(cfg)
	case "status":
		err = Status(cfg)
	case "worker":
		fmt.Fprintln(os.Stderr, "risper meeting: worker is an internal command")
		return 2
	default:
		fmt.Fprintf(os.Stderr, "risper meeting: unknown action %q\n", parser.Arg(0))
		return 2
	}
	if err != nil {
		return fail(err)
	}
	return 0
}

// WorkerMain is separate from Main so the private worker can take a session
// path without exposing it as part of the user-facing command grammar.
func WorkerMain(sessionDir string) int {
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	if err := Worker(cfg, filepath.Clean(sessionDir)); err != nil {
		return fail(err)
	}
	return 0
}

func fail(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, "risper meeting:", err)
	}
	return 1
}

func Start(cfg config.Config) error {
	if active, err := transcription.Current(cfg); err != nil {
		return err
	} else if active != nil {
		return errors.New("a dictation transcription is still running")
	}
	if active, err := recording.Current(cfg); err != nil {
		return err
	} else if active != nil {
		return errors.New("a recording is already active")
	}
	profile, err := models.Active(cfg)
	if err != nil {
		return err
	}
	profiles, err := models.Load(cfg)
	if err != nil {
		return err
	}
	if _, err := models.SelectFallback(profile, profiles, cfg.VoiceTriggerProfile); err != nil {
		return err
	}
	recorder, err := recording.StartMode(cfg, "meeting")
	if err != nil {
		return err
	}
	state := &State{
		Version:      1,
		Status:       "recording",
		SessionDir:   recorder.SessionDir,
		ProfileID:    profile.ID,
		SliceSeconds: cfg.MeetingSliceSeconds,
		SilenceDB:    cfg.MeetingSilenceDB,
		StartedAt:    recorder.StartedAt,
		Slices:       []Slice{},
	}
	if err := saveState(state); err != nil {
		_, _ = recording.Stop(cfg, recorder)
		return err
	}
	pid, err := spawnWorker(state.SessionDir)
	if err != nil {
		_, _ = recording.Stop(cfg, recorder)
		return fmt.Errorf("start background slicer: %w", err)
	}
	state.WorkerPID = pid
	if err := saveState(state); err != nil {
		_ = syscall.Kill(pid, syscall.SIGTERM)
		_, _ = recording.Stop(cfg, recorder)
		return err
	}
	_, _ = events.Append(state.SessionDir, "meeting.started", map[string]any{
		"profile": profile.ID, "slice_seconds": state.SliceSeconds,
		"silence_db": state.SilenceDB, "worker_pid": pid,
	})
	desktop.Notify(cfg, "🗒 Risper meeting started", "Double Alt checkpoints; ordinary dictation is unavailable. Stop with: risper meeting stop")
	fmt.Printf("Meeting recording started: %s\n", state.SessionDir)
	fmt.Println("Double Alt checkpoints; ordinary dictation is unavailable; risper meeting stop ends the meeting.")
	return nil
}

func spawnWorker(sessionDir string) (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, err
	}
	logPath := filepath.Join(sessionDir, workerLogFile)
	log, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	cmd := exec.Command(executable, "meeting-worker", sessionDir)
	cmd.Stdin = nil
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		log.Close()
		return 0, err
	}
	_ = log.Close()
	return cmd.Process.Pid, nil
}

func Worker(cfg config.Config, sessionDir string) error {
	for {
		recorder, err := recording.Current(cfg)
		if err != nil {
			return err
		}
		if recorder == nil || recorder.Mode != "meeting" || filepath.Clean(recorder.SessionDir) != sessionDir {
			return nil
		}
		err = withLock(sessionDir, func() error {
			state, err := loadState(sessionDir)
			if err != nil {
				return err
			}
			if state.Status != "recording" {
				return nil
			}
			return processAvailable(cfg, recorder, state, false)
		})
		if err != nil {
			_, _ = events.Append(sessionDir, "meeting.worker_error", map[string]any{"error": err.Error()})
		}
		time.Sleep(workerPoll)
	}
}

func Checkpoint(cfg config.Config) error {
	recorder, err := activeMeeting(cfg)
	if err != nil {
		return err
	}
	return withLock(recorder.SessionDir, func() error {
		state, err := loadState(recorder.SessionDir)
		if err != nil {
			return err
		}
		if err := processAvailable(cfg, recorder, state, true); err != nil {
			return err
		}
		from := checkpointStart(state)
		if !checkpointHasContent(state.Slices[from:]) {
			state.LastCheckpointSlice = len(state.Slices)
			if err := saveState(state); err != nil {
				return err
			}
			_, _ = events.Append(state.SessionDir, "meeting.checkpoint", map[string]any{
				"from_slice": from + 1, "through_slice": len(state.Slices),
				"copied": false, "message": "no new speech", "chars": 0,
			})
			desktop.Notify(cfg, "🗒 Meeting checkpoint", "No new speech to copy. Recording continues.")
			fmt.Println("No new meeting speech to copy; recording continues.")
			return nil
		}
		text := checkpointText(state)
		copied, message := desktop.CopyText(text)
		_, _ = events.Append(state.SessionDir, "meeting.checkpoint", map[string]any{
			"from_slice": state.LastCheckpointSlice + 1, "through_slice": len(state.Slices),
			"copied": copied, "message": message, "chars": len([]rune(text)),
		})
		if !copied {
			return errors.New(message)
		}
		state.LastCheckpointSlice = len(state.Slices)
		if err := saveState(state); err != nil {
			return err
		}
		desktop.Notify(cfg, "📋 Meeting checkpoint copied", checkpointSummary(state))
		fmt.Println("Meeting checkpoint copied to the clipboard.")
		return nil
	})
}

func Stop(cfg config.Config) error {
	recorder, err := activeMeeting(cfg)
	if err != nil {
		return err
	}
	return withLock(recorder.SessionDir, func() error {
		metadata, err := recording.Stop(cfg, recorder)
		if err != nil {
			return err
		}
		if metadata.Status != "recorded" {
			return fmt.Errorf("meeting audio did not stop cleanly; session retained at %s", recorder.SessionDir)
		}
		state, err := loadState(recorder.SessionDir)
		if err != nil {
			return err
		}
		// The finished mixed file uses ffmpeg's longest-source duration. Reading
		// it for the final flush keeps a small recorder-start/stop skew from
		// dropping the last words merely because one part file is shorter.
		finalRecorder := *recorder
		finalRecorder.PartPaths = map[string]string{"mixed": metadata.AudioPath}
		if err := processAvailable(cfg, &finalRecorder, state, true); err != nil {
			return err
		}
		from := checkpointStart(state)
		hasFinalDelta := checkpointHasContent(state.Slices[from:])
		copied := false
		clipboardMessage := "no new final speech; clipboard unchanged"
		if hasFinalDelta {
			copied, clipboardMessage = desktop.CopyText(checkpointText(state))
		}
		if copied || !hasFinalDelta {
			state.LastCheckpointSlice = len(state.Slices)
		}
		state.Status = "complete"
		state.EndedAt = time.Now().Format(time.RFC3339)
		if err := saveState(state); err != nil {
			return err
		}
		profile, profileErr := pinnedProfile(cfg, state.ProfileID)
		if profileErr == nil {
			metadata.TranscriptionEngine = profile.Engine
			metadata.Model = profile.Model
			metadata.Language = profile.Language
			recordMeetingCost(metadata, profile, state.Slices)
		}
		metadata.Status = "complete"
		metadata.Errors = append(metadata.Errors, failedSliceErrors(state.Slices)...)
		if err := session.SaveMetadata(metadata); err != nil {
			return err
		}
		_, _ = events.Append(state.SessionDir, "meeting.completed", map[string]any{
			"slices": len(state.Slices), "failed_slices": len(metadata.Errors),
			"transcript_path": metadata.TranscriptCleanPath, "final_delta_available": hasFinalDelta,
			"final_delta_copied": copied,
			"clipboard_message":  clipboardMessage,
		})
		body := "Transcript: " + metadata.TranscriptCleanPath
		if hasFinalDelta && !copied {
			body += ". Final checkpoint was not copied: " + clipboardMessage
		} else if !hasFinalDelta {
			body += ". No new final speech; clipboard unchanged."
		}
		desktop.Notify(cfg, "✅ Risper meeting stopped", body)
		fmt.Println("Meeting stopped. Transcript:", metadata.TranscriptCleanPath)
		if hasFinalDelta && !copied {
			return errors.New(clipboardMessage)
		}
		return nil
	})
}

func Status(cfg config.Config) error {
	recorder, err := recording.Current(cfg)
	if err != nil {
		return err
	}
	if recorder == nil || recorder.Mode != "meeting" {
		fmt.Println("No meeting is active.")
		return nil
	}
	state, err := loadState(recorder.SessionDir)
	if err != nil {
		return err
	}
	fmt.Printf("Meeting active: %s\n", state.SessionDir)
	fmt.Printf("Profile: %s; slices: %d; checkpointed: %d\n", state.ProfileID, len(state.Slices), state.LastCheckpointSlice)
	fmt.Printf("Transcript: %s\n", filepath.Join(state.SessionDir, session.TranscriptCleanFile))
	return nil
}

func activeMeeting(cfg config.Config) (*recording.State, error) {
	recorder, err := recording.Current(cfg)
	if err != nil {
		return nil, err
	}
	if recorder == nil {
		return nil, errors.New("no meeting is active")
	}
	if recorder.Mode != "meeting" {
		return nil, errors.New("the active recording is ordinary dictation, not a meeting")
	}
	return recorder, nil
}

func statePath(sessionDir string) string { return filepath.Join(sessionDir, stateFile) }

func loadState(sessionDir string) (*State, error) {
	var state State
	if err := files.ReadJSON(statePath(sessionDir), &state); err != nil {
		return nil, err
	}
	if filepath.Clean(state.SessionDir) != filepath.Clean(sessionDir) {
		return nil, errors.New("meeting state belongs to a different session")
	}
	return &state, nil
}

func saveState(state *State) error {
	if state == nil || state.SessionDir == "" {
		return errors.New("meeting state has no session directory")
	}
	return files.AtomicWriteJSON(statePath(state.SessionDir), state)
}

func withLock(sessionDir string, fn func() error) error {
	handle, err := os.OpenFile(filepath.Join(sessionDir, lockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer handle.Close()
	if err := syscall.Flock(int(handle.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(handle.Fd()), syscall.LOCK_UN)
	return fn()
}

func processAvailable(cfg config.Config, recorder *recording.State, state *State, force bool) error {
	if state.SliceSeconds <= 0 {
		return fmt.Errorf("meeting slice duration must be positive, got %d", state.SliceSeconds)
	}
	available, err := availablePCM(recorder.PartPaths)
	if err != nil {
		return err
	}
	for available-state.NextPCMByte >= minimumTail {
		targetBytes := int64(state.SliceSeconds) * pcmBytesSecond
		remaining := available - state.NextPCMByte
		if !force && remaining < targetBytes {
			break
		}
		end := available
		// A forced flush makes the short tail eligible, but it must not turn a
		// worker backlog into one unbounded upload. Catch up in the same bounded,
		// silence-seeking slices used by the background worker.
		if remaining >= targetBytes {
			desired := state.NextPCMByte + targetBytes
			end, err = quietestBoundary(recorder.PartPaths, desired, available)
			if err != nil {
				return err
			}
			if end <= state.NextPCMByte {
				end = desired
			}
		}
		slice := transcribeSlice(cfg, recorder, state, state.NextPCMByte, end)
		state.Slices = append(state.Slices, slice)
		state.NextPCMByte = end
		if err := saveState(state); err != nil {
			return err
		}
		if err := writeFullTranscript(state); err != nil {
			return err
		}
		_, _ = events.Append(state.SessionDir, "meeting.slice", map[string]any{
			"index": slice.Index, "start_seconds": slice.StartSeconds, "end_seconds": slice.EndSeconds,
			"status": slice.Status, "profile": slice.ProfileID, "engine": slice.Engine, "error": slice.Error,
		})
		available, err = availablePCM(recorder.PartPaths)
		if err != nil {
			return err
		}
	}
	return nil
}

func transcribeSlice(cfg config.Config, recorder *recording.State, state *State, start, end int64) Slice {
	index := len(state.Slices) + 1
	result := Slice{
		Index: index, StartPCMByte: start, EndPCMByte: end,
		StartSeconds: float64(start) / float64(pcmBytesSecond),
		EndSeconds:   float64(end) / float64(pcmBytesSecond),
		Status:       "failed",
	}
	pcm, err := mixedPCM(recorder.PartPaths, start, end)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if dbFS(pcm) < state.SilenceDB {
		result.Status = "silent"
		return result
	}
	profile, err := pinnedProfile(cfg, state.ProfileID)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	profiles, err := models.Load(cfg)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	fallback, err := models.SelectFallback(profile, profiles, cfg.VoiceTriggerProfile)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	tail := transcriptTail(state)
	if profile.Engine == "openai" && tail != "" {
		profile.Prompt = strings.TrimSpace(strings.TrimSpace(profile.Prompt) + "\nPrevious transcript context: " + tail)
	}
	dir := filepath.Join(state.SessionDir, slicesDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		result.Error = err.Error()
		return result
	}
	base := filepath.Join(dir, fmt.Sprintf("%06d", index))
	wavPath := base + ".wav"
	mp3Path := base + ".mp3"
	rawPath := base + ".raw.txt"
	cleanPath := base + ".txt"
	if err := writeWAV(wavPath, pcm); err != nil {
		result.Error = err.Error()
		return result
	}
	defer os.Remove(wavPath)
	audioPath := wavPath
	if profile.Engine == "openai" {
		if err := encodeMP3(wavPath, mp3Path); err != nil {
			result.Error = err.Error()
			return result
		}
		defer os.Remove(mp3Path)
		audioPath = mp3Path
	}
	transcribed := transcription.TranscriptionResult{Profile: profile}
	if fallback.ID != "" {
		transcribed, err = transcription.TranscribeWithFallbackAudio(
			profile, fallback, audioPath, wavPath, rawPath, cleanPath,
			time.Duration(profile.FallbackTimeoutSeconds)*time.Second, nil,
		)
	} else {
		transcribed.Transcript, err = transcription.Transcribe(profile, audioPath, rawPath, cleanPath, nil)
	}
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if promptEcho(transcribed.Transcript, tail) {
		_ = os.Remove(rawPath)
		_ = os.Remove(cleanPath)
		result.Status = "prompt_echo"
		return result
	}
	result.Status = "transcribed"
	result.TranscriptPath = cleanPath
	result.ProfileID = transcribed.Profile.ID
	result.Engine = transcribed.Profile.Engine
	return result
}

func pinnedProfile(cfg config.Config, id string) (models.Profile, error) {
	profiles, err := models.Load(cfg)
	if err != nil {
		return models.Profile{}, err
	}
	profile, ok := profiles[id]
	if !ok {
		return models.Profile{}, fmt.Errorf("meeting profile %q is no longer configured", id)
	}
	return profile, nil
}

func transcriptTail(state *State) string {
	for i := len(state.Slices) - 1; i >= 0; i-- {
		path := state.Slices[i].TranscriptPath
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		runes := []rune(strings.TrimSpace(string(data)))
		if len(runes) > promptTailRunes {
			runes = runes[len(runes)-promptTailRunes:]
		}
		return string(runes)
	}
	return ""
}

func promptEcho(text, tail string) bool {
	normalize := func(value string) string {
		return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
	}
	return tail != "" && normalize(text) == normalize(tail)
}

func writeFullTranscript(state *State) error {
	text := transcriptForSlices(state.Slices)
	raw := filepath.Join(state.SessionDir, session.TranscriptRawFile)
	clean := filepath.Join(state.SessionDir, session.TranscriptCleanFile)
	return transcription.WriteTranscript(raw, clean, text)
}

func transcriptForSlices(slices []Slice) string {
	parts := make([]string, 0, len(slices))
	for _, slice := range slices {
		switch slice.Status {
		case "transcribed":
			data, err := os.ReadFile(slice.TranscriptPath)
			if err == nil && strings.TrimSpace(string(data)) != "" {
				parts = append(parts, strings.TrimSpace(string(data)))
			}
		case "failed":
			parts = append(parts, fmt.Sprintf("[Transcription gap %.1fs–%.1fs: %s]", slice.StartSeconds, slice.EndSeconds, slice.Error))
		case "prompt_echo":
			parts = append(parts, fmt.Sprintf("[Possible prompt echo omitted %.1fs–%.1fs]", slice.StartSeconds, slice.EndSeconds))
		}
	}
	if len(parts) == 0 {
		return "[No speech transcribed]"
	}
	return strings.Join(parts, "\n\n")
}

func checkpointText(state *State) string {
	from := checkpointStart(state)
	body := transcriptForSlices(state.Slices[from:])
	startSeconds := float64(0)
	endSeconds := float64(state.NextPCMByte) / float64(pcmBytesSecond)
	if from > 0 {
		startSeconds = state.Slices[from-1].EndSeconds
	}
	return fmt.Sprintf("Risper meeting checkpoint | %s–%s\n\n%s", wallClock(state.StartedAt, startSeconds), wallClock(state.StartedAt, endSeconds), body)
}

func checkpointStart(state *State) int {
	from := state.LastCheckpointSlice
	if from < 0 || from > len(state.Slices) {
		return 0
	}
	return from
}

func checkpointHasContent(slices []Slice) bool {
	for _, slice := range slices {
		switch slice.Status {
		case "transcribed", "failed", "prompt_echo":
			return true
		}
	}
	return false
}

func wallClock(startedAt string, offset float64) string {
	started, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return fmt.Sprintf("+%.0fs", offset)
	}
	return started.Add(time.Duration(offset * float64(time.Second))).In(time.Local).Format("15:04:05")
}

func checkpointSummary(state *State) string {
	failed := 0
	for _, slice := range state.Slices {
		if slice.Status == "failed" {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Sprintf("Copied through slice %d; transcript has %d marked gap(s).", len(state.Slices), failed)
	}
	return fmt.Sprintf("Copied through slice %d. Recording continues.", len(state.Slices))
}

func failedSliceErrors(slices []Slice) []string {
	var result []string
	for _, slice := range slices {
		switch slice.Status {
		case "failed":
			result = append(result, fmt.Sprintf("meeting slice %d failed: %s", slice.Index, slice.Error))
		case "prompt_echo":
			result = append(result, fmt.Sprintf("meeting slice %d omitted as a possible prompt echo", slice.Index))
		}
	}
	return result
}

func recordMeetingCost(metadata *session.Metadata, profile models.Profile, slices []Slice) {
	metadata.TranscriptionCost = nil
	metadata.TranscriptionCurrency = ""
	metadata.TranscriptionRatePerMinute = nil
	if profile.Engine != "openai" || profile.BillingPricePerMinute <= 0 || profile.BillingCurrency == "" {
		return
	}
	seconds := float64(0)
	for _, slice := range slices {
		if slice.Status == "transcribed" && slice.Engine == "openai" {
			seconds += slice.EndSeconds - slice.StartSeconds
		}
	}
	cost := profile.BillingPricePerMinute * seconds / 60
	metadata.TranscriptionCost = &cost
	metadata.TranscriptionCurrency = profile.BillingCurrency
	metadata.TranscriptionRatePerMinute = &profile.BillingPricePerMinute
}

func encodeMP3(input, output string) error {
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", input, "-ar", "16000", "-ac", "1", "-b:a", "32k", output)
	cmd.Stdout = io.Discard
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("encode meeting slice: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
