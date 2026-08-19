// Package recording owns the PipeWire capture and the durable recording state.
package recording

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/events"
	"github.com/rob137/risper/internal/files"
	"github.com/rob137/risper/session"
)

type Source string

const (
	Mic    Source = "mic"
	System Source = "system"
	Mixed  Source = "mixed"
)

const wavHeaderBytes = 44

const recorderStartupGrace = 100 * time.Millisecond

// State is the cross-process marker for the two recorders belonging to one
// session. TranscriptionSource is a request, not a capture choice: both
// sources are always recorded.
type State struct {
	SessionDir          string            `json:"session_dir"`
	MetadataPath        string            `json:"metadata_path"`
	AudioPath           string            `json:"audio_path"`
	Sources             []Source          `json:"sources"`
	RecorderPIDs        map[string]int    `json:"recorder_pids"`
	PartPaths           map[string]string `json:"part_paths"`
	RecorderBackend     string            `json:"recorder_backend"`
	StartedAt           string            `json:"started_at"`
	TranscriptionSource Source            `json:"transcription_source"`
}

func SourcePath(audioPath string, source Source) string {
	ext := filepath.Ext(audioPath)
	return audioPath[:len(audioPath)-len(ext)] + "." + string(source) + ext
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func Current(cfg config.Config) (*State, error) {
	if _, err := os.Stat(cfg.CurrentStatePath); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var state State
	if err := files.ReadJSON(cfg.CurrentStatePath, &state); err != nil {
		if removeErr := os.Remove(cfg.CurrentStatePath); removeErr != nil && !os.IsNotExist(removeErr) {
			return nil, removeErr
		}
		return nil, nil
	}
	for _, pid := range state.RecorderPIDs {
		if pidAlive(pid) {
			return &state, nil
		}
	}
	if err := os.Remove(cfg.CurrentStatePath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return nil, nil
}

func Start(cfg config.Config, requestMixed bool) (*State, error) {
	if current, err := Current(cfg); err != nil {
		return nil, err
	} else if current != nil {
		return nil, errors.New("recording is already active")
	}
	if !config.CommandExists("pw-record") {
		return nil, errors.New("pw-record is not installed; cannot record audio")
	}
	if !config.CommandExists("ffmpeg") {
		return nil, errors.New("ffmpeg is not installed; cannot combine audio sources")
	}

	metadata, err := session.Create(cfg)
	if err != nil {
		return nil, err
	}
	partPaths := map[string]string{
		string(Mic):    SourcePath(metadata.AudioPath, Mic),
		string(System): SourcePath(metadata.AudioPath, System),
	}
	metadata.AudioSources = []string{string(Mic), string(System)}
	metadata.AudioSourcePaths = partPaths
	if err := session.SaveMetadata(metadata); err != nil {
		return nil, err
	}

	sessionDir := session.SessionDir(metadata)
	statusLog := filepath.Join(sessionDir, session.StatusLogFile)
	appendLog(statusLog, "starting recorder backend=pw-record sources=mic,system")
	if _, err := events.Append(sessionDir, "recorder.starting", map[string]any{
		"backend":    "pw-record",
		"sources":    []string{string(Mic), string(System)},
		"audio_path": metadata.AudioPath,
		"part_paths": partPaths,
	}); err != nil {
		return nil, err
	}

	pids := make(map[string]int, 2)
	waits := make(map[string]<-chan error, 2)
	for _, source := range []Source{Mic, System} {
		cmd := recorderCommand(source, partPaths[string(source)])
		logPath := filepath.Join(sessionDir, recorderLogName(source))
		logFile, openErr := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if openErr != nil {
			stopPIDs(pids)
			return nil, openErr
		}
		cmd.Stdout = io.Discard
		cmd.Stderr = logFile
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		startErr := cmd.Start()
		closeErr := logFile.Close()
		if startErr != nil {
			stopPIDs(pids)
			markStartFailed(metadata, pids)
			return nil, startErr
		}
		if closeErr != nil {
			stopPIDs(pids)
			markStartFailed(metadata, pids)
			return nil, closeErr
		}
		pids[string(source)] = cmd.Process.Pid
		// The state file is the cross-process handle used by the next toggle,
		// but this process still owns the child while it remains alive. Reap it
		// asynchronously so a clean SIGINT does not look like a live zombie.
		waited := make(chan error, 1)
		waits[string(source)] = waited
		go func() { waited <- cmd.Wait() }()
	}
	for _, source := range []Source{Mic, System} {
		timer := time.NewTimer(recorderStartupGrace)
		select {
		case err := <-waits[string(source)]:
			if !timer.Stop() {
				<-timer.C
			}
			reason := fmt.Sprintf("pw-record %s exited during startup: %v", source, err)
			stopPIDs(pids)
			markStartFailedReason(metadata, pids, reason)
			return nil, errors.New(reason)
		case <-timer.C:
		}
	}

	transcriptionSource := Mic
	if requestMixed {
		transcriptionSource = Mixed
	}
	state := &State{
		SessionDir:          sessionDir,
		MetadataPath:        session.MetadataPath(sessionDir),
		AudioPath:           metadata.AudioPath,
		Sources:             []Source{Mic, System},
		RecorderPIDs:        pids,
		PartPaths:           partPaths,
		RecorderBackend:     "pw-record",
		StartedAt:           metadata.StartedAt,
		TranscriptionSource: transcriptionSource,
	}
	if err := files.AtomicWriteJSON(cfg.CurrentStatePath, state); err != nil {
		stopPIDs(pids)
		return nil, err
	}
	appendLog(statusLog, fmt.Sprintf("pw-record pids=%v transcription_source=%s", pids, transcriptionSource))
	if _, err := events.Append(sessionDir, "recorder.started", map[string]any{
		"backend":              "pw-record",
		"pids":                 pids,
		"transcription_source": transcriptionSource,
	}); err != nil {
		return nil, err
	}
	return state, nil
}

func recorderCommand(source Source, path string) *exec.Cmd {
	args := []string{"--rate", "16000", "--channels", "1", "--format", "s16"}
	if source == System {
		// With no --target, this follows the default sink's monitor. The
		// property is what distinguishes sink capture from another mic.
		args = append(args, "-P", "{ stream.capture.sink=true }")
	}
	return exec.Command("pw-record", append(args, path)...)
}

func recorderLogName(source Source) string {
	if source == Mic {
		return "pw-record.log"
	}
	return "pw-record.system.log"
}

func markStartFailed(metadata *session.Metadata, pids map[string]int) {
	markStartFailedReason(metadata, pids, "")
}

func markStartFailedReason(metadata *session.Metadata, pids map[string]int, reason string) {
	metadata.Status = "failed"
	metadata.Errors = append(metadata.Errors, "could not start all audio recorders")
	if reason != "" {
		metadata.Errors = append(metadata.Errors, reason)
	}
	_ = session.SaveMetadata(metadata)
	details := map[string]any{
		"sources": []string{string(Mic), string(System)},
		"started": pids,
	}
	if reason != "" {
		details["error"] = reason
	}
	_, _ = events.Append(session.SessionDir(metadata), "recorder.start_failed", details)
}

func appendLog(path, message string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	handle, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer handle.Close()
	_, err = fmt.Fprintf(handle, "%s %s\n", time.Now().Format(time.RFC3339), message)
	return err
}

func stopPIDs(pids map[string]int) {
	for _, pid := range pids {
		if pidAlive(pid) {
			_ = syscall.Kill(-pid, syscall.SIGINT)
		}
	}
	for _, pid := range pids {
		waitForExit(pid, 4*time.Second)
	}
	for _, pid := range pids {
		if pidAlive(pid) {
			_ = syscall.Kill(-pid, syscall.SIGTERM)
		}
	}
	for _, pid := range pids {
		waitForExit(pid, 2*time.Second)
	}
	for _, pid := range pids {
		if pidAlive(pid) {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	}
}

func waitForExit(pid int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func Stop(cfg config.Config, state *State) (*session.Metadata, error) {
	if state == nil {
		return nil, errors.New("cannot stop a nil recording state")
	}
	metadata, err := session.LoadSession(state.SessionDir)
	if err != nil {
		return nil, err
	}
	if metadata == nil {
		return nil, fmt.Errorf("recording session is missing metadata: %s", state.SessionDir)
	}
	statusLog := filepath.Join(state.SessionDir, session.StatusLogFile)
	appendLog(statusLog, fmt.Sprintf("stopping recorder backend=%s pids=%v", state.RecorderBackend, state.RecorderPIDs))
	_, _ = events.Append(state.SessionDir, "recorder.stopping", map[string]any{
		"backend": state.RecorderBackend,
		"pids":    state.RecorderPIDs,
	})
	stopPIDs(state.RecorderPIDs)

	errorsFound := append([]string{}, metadata.Errors...)
	if err := mixSources(state, metadata.AudioPath, statusLog); err != nil {
		message := "could not combine audio sources: " + err.Error()
		errorsFound = append(errorsFound, message)
		appendLog(statusLog, message)
		_, _ = events.Append(state.SessionDir, "recorder.mix_failed", map[string]any{
			"error": err.Error(), "parts": state.PartPaths,
		})
	} else {
		for _, source := range state.Sources {
			if !hasAudio(state.PartPaths[string(source)]) {
				errorsFound = append(errorsFound, "No audio captured from: "+string(source)+".")
			}
		}
	}
	endedAt := time.Now().Format(time.RFC3339)
	duration := durationSeconds(metadata.StartedAt, endedAt)
	status := "recorded"
	if info, statErr := os.Stat(metadata.AudioPath); statErr != nil || info.Size() <= wavHeaderBytes {
		errorsFound = append(errorsFound, "Recording stopped but mixed audio file was missing or empty.")
		status = "failed"
	}
	metadata.EndedAt = &endedAt
	metadata.DurationSeconds = &duration
	metadata.Status = status
	metadata.Errors = errorsFound
	if err := session.SaveMetadata(metadata); err != nil {
		return nil, err
	}
	appendLog(statusLog, "recording stopped status="+status)
	info, _ := os.Stat(metadata.AudioPath)
	audioBytes := int64(0)
	if info != nil {
		audioBytes = info.Size()
	}
	_, _ = events.Append(state.SessionDir, "recorder.stopped", map[string]any{
		"status": status, "audio_path": metadata.AudioPath,
		"audio_bytes": audioBytes, "duration_seconds": duration,
	})
	if err := os.Remove(cfg.CurrentStatePath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return metadata, nil
}

func durationSeconds(startedAt, endedAt string) float64 {
	start, startErr := time.Parse(time.RFC3339, startedAt)
	end, endErr := time.Parse(time.RFC3339, endedAt)
	if startErr != nil || endErr != nil {
		return 0
	}
	return float64(end.Sub(start)) / float64(time.Second)
}

func hasAudio(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > wavHeaderBytes
}

func mixSources(state *State, output, statusLog string) error {
	usable := make([]string, 0, len(state.Sources))
	for _, source := range state.Sources {
		path := state.PartPaths[string(source)]
		if hasAudio(path) {
			usable = append(usable, path)
		}
	}
	if len(usable) == 0 {
		return errors.New("no source captured any audio")
	}
	if len(usable) == 1 {
		if err := copyFile(usable[0], output); err != nil {
			return err
		}
	} else {
		inputs := make([]string, 0, len(usable)*2)
		for _, path := range usable {
			inputs = append(inputs, "-i", path)
		}
		args := []string{
			"-hide_banner", "-loglevel", "error", "-y",
		}
		args = append(args, inputs...)
		args = append(args,
			"-filter_complex", fmt.Sprintf("amix=inputs=%d:duration=longest:normalize=1", len(usable)),
			"-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", output,
		)
		var stderr bytes.Buffer
		cmd := exec.Command("ffmpeg", args...)
		cmd.Stdout = io.Discard
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			if text := stderr.String(); text != "" {
				appendLog(statusLog, "ffmpeg: "+text)
			}
			return err
		}
	}
	usedSources := make([]string, 0, len(usable))
	droppedSources := make([]string, 0)
	for _, source := range state.Sources {
		path := state.PartPaths[string(source)]
		found := false
		for _, used := range usable {
			if used == path {
				found = true
				break
			}
		}
		if found {
			usedSources = append(usedSources, string(source))
		} else {
			droppedSources = append(droppedSources, string(source))
		}
	}
	appendLog(statusLog, fmt.Sprintf("combined sources=%v dropped=%v", usedSources, droppedSources))
	_, _ = events.Append(state.SessionDir, "recorder.mixed", map[string]any{
		"used_sources": usedSources, "dropped_sources": droppedSources,
		"output": output,
	})
	return nil
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}
