// Package transcription owns the durable marker for an in-flight
// transcription and the cancellation boundary around its processes.
package transcription

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/events"
	"github.com/rob137/risper/internal/files"
	"github.com/rob137/risper/session"
)

type State struct {
	SessionDir    string `json:"session_dir"`
	MetadataPath  string `json:"metadata_path"`
	ControllerPID int    `json:"controller_pid"`
	WorkerPID     *int   `json:"worker_pid"`
	Profile       string `json:"profile"`
	StartedAt     string `json:"started_at"`
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func Current(cfg config.Config) (*State, error) {
	if _, err := os.Stat(cfg.CurrentTranscriptionPath); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var state State
	if err := files.ReadJSON(cfg.CurrentTranscriptionPath, &state); err != nil {
		if removeErr := os.Remove(cfg.CurrentTranscriptionPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return nil, removeErr
		}
		return nil, nil
	}
	if pidAlive(state.ControllerPID) || (state.WorkerPID != nil && pidAlive(*state.WorkerPID)) {
		return &state, nil
	}
	if err := os.Remove(cfg.CurrentTranscriptionPath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return nil, nil
}

func Start(cfg config.Config, metadata *session.Metadata, profileID string) error {
	if metadata == nil {
		return errors.New("cannot start transcription state without session metadata")
	}
	state := State{
		SessionDir:    session.SessionDir(metadata),
		MetadataPath:  session.MetadataPath(session.SessionDir(metadata)),
		ControllerPID: os.Getpid(),
		Profile:       profileID,
		StartedAt:     time.Now().Format(time.RFC3339),
	}
	return files.AtomicWriteJSON(cfg.CurrentTranscriptionPath, state)
}

func SetWorkerPID(cfg config.Config, workerPID int) error {
	state, err := Current(cfg)
	if err != nil || state == nil {
		return err
	}
	state.WorkerPID = &workerPID
	return files.AtomicWriteJSON(cfg.CurrentTranscriptionPath, state)
}

func Finish(cfg config.Config) error {
	err := os.Remove(cfg.CurrentTranscriptionPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func appendLog(path, message string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	handle, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(handle, "%s %s\n", time.Now().Format(time.RFC3339), message); err != nil {
		_ = handle.Close()
		return err
	}
	return handle.Close()
}

func terminatePID(pid int, timeout time.Duration) error {
	if !pidAlive(pid) {
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func terminateProcessGroup(pid int) error {
	if pid <= 0 || pid == os.Getpid() || !pidAlive(pid) {
		return nil
	}
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.ESRCH) {
			return terminatePID(pid, time.Second)
		}
		return err
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return terminatePID(pid, 200*time.Millisecond)
	}
	return nil
}

func stateMetadataPath(state *State) string {
	if state.MetadataPath != "" {
		return state.MetadataPath
	}
	return session.MetadataPath(state.SessionDir)
}

// Cancel records cancellation before terminating worker processes. This keeps
// the session useful even if a child has already disappeared.
func Cancel(cfg config.Config, state *State) error {
	if state == nil {
		return errors.New("cannot cancel a nil transcription state")
	}
	const message = "transcription cancelled by user"
	metadata, err := session.LoadSession(filepath.Dir(stateMetadataPath(state)))
	if err != nil {
		return err
	}
	transcriptionDir := state.SessionDir
	if metadata != nil {
		transcriptionDir = session.SessionDir(metadata)
		if err := appendLog(filepath.Join(transcriptionDir, session.StatusLogFile), message); err != nil {
			return err
		}
		if _, err := events.Append(transcriptionDir, "transcription.cancel_requested", map[string]any{
			"controller_pid": state.ControllerPID,
			"worker_pid":     state.WorkerPID,
		}); err != nil {
			return err
		}
		metadata.Status = "cancelled"
		metadata.Errors = append(metadata.Errors, message)
		if err := session.SaveMetadata(metadata); err != nil {
			return err
		}
	}
	if state.ControllerPID != 0 && state.ControllerPID != os.Getpid() {
		if err := syscall.Kill(state.ControllerPID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
	}
	if state.WorkerPID != nil {
		if err := terminateProcessGroup(*state.WorkerPID); err != nil {
			return err
		}
	}
	if state.ControllerPID != 0 && state.ControllerPID != os.Getpid() {
		if err := terminatePID(state.ControllerPID, 200*time.Millisecond); err != nil {
			return err
		}
	}
	if err := Finish(cfg); err != nil {
		return err
	}
	return appendLog(cfg.LogPath, message)
}
