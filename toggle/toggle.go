// Package toggle drives one complete record, transcribe, and clipboard cycle.
package toggle

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/desktop"
	"github.com/rob137/risper/events"
	"github.com/rob137/risper/models"
	"github.com/rob137/risper/recording"
	"github.com/rob137/risper/session"
	"github.com/rob137/risper/transcription"
)

const version = "0.1.0"

func Main(args []string) int {
	parser := flag.NewFlagSet("risper-toggle", flag.ContinueOnError)
	parser.SetOutput(os.Stderr)
	system := parser.Bool("system", false, "transcribe the mixed mic and computer-output audio")
	if err := parser.Parse(args); err != nil {
		return 2
	}
	if parser.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "risper-toggle: unexpected positional argument")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		return fail(cfg, err)
	}
	if state, stateErr := transcription.Current(cfg); stateErr != nil {
		return fail(cfg, stateErr)
	} else if state != nil {
		if err := transcription.Cancel(cfg, state); err != nil {
			return fail(cfg, err)
		}
		desktop.Notify(cfg, "🛑 Risper cancelled", "Transcription stopped.")
		desktop.Play(cfg, "cancel")
		return 0
	}

	if state, stateErr := recording.Current(cfg); stateErr != nil {
		return fail(cfg, stateErr)
	} else if state != nil {
		metadata, stopErr := recording.Stop(cfg, state)
		if stopErr != nil {
			return fail(cfg, stopErr)
		}
		useMixed := *system || state.TranscriptionSource == recording.Mixed
		return finishSession(cfg, metadata, useMixed)
	}

	state, err := recording.Start(cfg, *system)
	if err != nil {
		return fail(cfg, err)
	}
	desktop.Notify(cfg, "🎙 Risper listening to mic and computer", "Run risper-toggle again to stop.")
	desktop.Play(cfg, "recording_start")
	fmt.Printf("Risper %s: recording %s sources=mic,system\n", version, state.SessionDir)
	return 0
}

func fail(cfg config.Config, err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, "risper-toggle:", err)
		if cfg.StateDir != "" {
			appendLog(cfg.LogPath, err.Error())
		}
		desktop.Notify(cfg, "⚠ Risper could not continue", err.Error())
		desktop.Play(cfg, "error")
	}
	return 1
}

func finishSession(cfg config.Config, metadata *session.Metadata, useMixed bool) int {
	if metadata == nil {
		return fail(cfg, errors.New("recording returned no session metadata"))
	}
	if metadata.Status != "recorded" {
		_, _ = events.Append(session.SessionDir(metadata), "workflow.finish_rejected", map[string]any{"status": metadata.Status})
		desktop.Notify(cfg, "⚠ Risper recording failed", "Audio was not captured cleanly; see session error log.")
		desktop.Play(cfg, "error")
		return 1
	}

	profile, err := models.Active(cfg)
	if err != nil {
		return transcriptionFailure(cfg, metadata, err)
	}
	audioPath := metadata.AudioPath
	audioSource := recording.Mixed
	if !useMixed {
		audioSource = recording.Mic
		if path := metadata.AudioSourcePaths[string(recording.Mic)]; path != "" {
			audioPath = path
		}
	}
	if info, err := os.Stat(audioPath); err != nil {
		return transcriptionFailure(cfg, metadata, fmt.Errorf("audio source %s is unavailable: %w", audioSource, err))
	} else if info.Size() <= 44 {
		return transcriptionFailure(cfg, metadata, fmt.Errorf("audio source %s is empty", audioSource))
	}

	metadata.Status = "transcribing"
	metadata.TranscriptionEngine = profile.Engine
	metadata.Model = profile.Model
	metadata.Language = profile.Language
	if err := session.SaveMetadata(metadata); err != nil {
		return fail(cfg, err)
	}
	_, _ = events.Append(session.SessionDir(metadata), "transcription.starting", map[string]any{
		"profile": profile.ID, "engine": profile.Engine, "model": profile.Model,
		"language": profile.Language, "audio_path": audioPath, "audio_source": audioSource,
	})
	appendLog(filepath.Join(session.SessionDir(metadata), session.StatusLogFile), "starting transcription audio_source="+string(audioSource))
	if err := transcription.Start(cfg, metadata, profile.ID); err != nil {
		return transcriptionFailure(cfg, metadata, err)
	}
	defer transcription.Finish(cfg)

	title := "📝 Transcribing speech"
	body := "Using " + profile.ID + "."
	desktop.Notify(cfg, title, body)
	desktop.Play(cfg, "transcription_start")
	stopHeartbeat := make(chan struct{})
	go heartbeat(cfg, title, body, stopHeartbeat)
	transcript, err := transcription.Transcribe(
		profile,
		audioPath,
		metadata.TranscriptRawPath,
		metadata.TranscriptCleanPath,
		func(pid int) error { return transcription.SetWorkerPID(cfg, pid) },
	)
	close(stopHeartbeat)
	if err != nil {
		return transcriptionFailure(cfg, metadata, err)
	}
	_, _ = events.Append(session.SessionDir(metadata), "transcription.completed", map[string]any{
		"raw_path": metadata.TranscriptRawPath, "clean_path": metadata.TranscriptCleanPath,
		"transcript_chars": len([]rune(transcript)), "audio_source": audioSource,
	})

	copied, clipboardMessage := desktop.CopyText(transcript)
	appendLog(filepath.Join(session.SessionDir(metadata), session.StatusLogFile), clipboardMessage)
	_, _ = events.Append(session.SessionDir(metadata), "clipboard.copy", map[string]any{
		"ok": copied, "message": clipboardMessage, "transcript_chars": len([]rune(transcript)),
	})
	if !copied {
		return transcriptionFailure(cfg, metadata, errors.New(clipboardMessage))
	}
	appendLog(filepath.Join(session.SessionDir(metadata), session.StatusLogFile), "automatic paste skipped; transcript left on clipboard")
	_, _ = events.Append(session.SessionDir(metadata), "paste.skipped", map[string]any{
		"reason": "automatic_paste_disabled", "session_type": metadata.SessionType,
	})
	falseValue := false
	metadata.Status = "complete"
	metadata.Errors = []string{}
	metadata.PasteAttempted = &falseValue
	metadata.PasteHelperSucceeded = &falseValue
	metadata.PasteSucceeded = &falseValue
	metadata.PasteConfirmation = "not_attempted_automatic_paste_disabled"
	if err := session.SaveMetadata(metadata); err != nil {
		return fail(cfg, err)
	}
	desktop.Notify(cfg, "✅ Risper copied", "Transcript is on the clipboard.")
	desktop.Play(cfg, "success")
	return 0
}

func heartbeat(cfg config.Config, title, body string, stop <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	started := time.Now()
	for {
		select {
		case <-ticker.C:
			elapsed := int(time.Since(started).Seconds())
			desktop.Notify(cfg, title, fmt.Sprintf("%s %ds elapsed.", body, elapsed))
			desktop.Play(cfg, "transcription_progress")
		case <-stop:
			return
		}
	}
}

func transcriptionFailure(cfg config.Config, metadata *session.Metadata, err error) int {
	message := "transcription failed: " + err.Error()
	dir := session.SessionDir(metadata)
	appendLog(filepath.Join(dir, session.ErrorLogFile), message)
	appendLog(filepath.Join(dir, session.StatusLogFile), message)
	_, _ = events.Append(dir, "transcription.failed", map[string]any{
		"error": err.Error(), "error_type": fmt.Sprintf("%T", err),
	})
	metadata.Status = "failed"
	metadata.Errors = append(metadata.Errors, message)
	if saveErr := session.SaveMetadata(metadata); saveErr != nil {
		return fail(cfg, saveErr)
	}
	desktop.Notify(cfg, "⚠ Risper transcription failed", "Audio was saved. Configure a local engine and retranscribe.")
	desktop.Play(cfg, "error")
	return 1
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
