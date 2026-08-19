// Package retranscribe re-runs a saved session through the active local
// model. The Go workflow is clipboard-only: there is no paste helper path.
package retranscribe

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/desktop"
	"github.com/rob137/risper/events"
	"github.com/rob137/risper/internal/args"
	"github.com/rob137/risper/models"
	"github.com/rob137/risper/session"
	"github.com/rob137/risper/transcription"
)

func Main(argv []string) int {
	parser := flag.NewFlagSet("risper retranscribe", flag.ContinueOnError)
	parser.SetOutput(os.Stderr)
	mixed := parser.Bool("mixed", false, "transcribe the mixed mic-and-system audio")
	system := parser.Bool("system", false, "alias for --mixed")
	// --copy remains accepted for scripts written against the Python command;
	// Go retranscription copies to the clipboard regardless.
	_ = parser.Bool("copy", false, "copy the transcript (always enabled)")
	if err := parser.Parse(args.Reorder(argv, nil)); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if parser.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "risper retranscribe: unexpected positional arguments")
		return 2
	}
	sessionID := "last"
	if parser.NArg() == 1 {
		sessionID = parser.Arg(0)
	}
	cfg, err := config.Load()
	if err != nil {
		return reportError(err)
	}
	metadata, err := session.Find(cfg, sessionID)
	if err != nil {
		return reportError(err)
	}
	if metadata == nil {
		fmt.Fprintln(os.Stderr, "No such session: "+sessionID)
		return 1
	}
	useMixed := *mixed || *system
	audioPath := selectedAudioPath(metadata, useMixed)
	if _, err := os.Stat(audioPath); err != nil {
		fmt.Fprintln(os.Stderr, session.MissingAudioMessage(metadata))
		return 1
	}
	profile, err := models.Active(cfg)
	if err != nil {
		return failSession(cfg, metadata, err)
	}
	return run(cfg, metadata, profile, audioPath, useMixed)
}

func selectedAudioPath(metadata *session.Metadata, mixed bool) string {
	if metadata == nil {
		return ""
	}
	if mixed {
		return metadata.AudioPath
	}
	if path := metadata.AudioSourcePaths["mic"]; path != "" {
		return path
	}
	// Sessions created before Go recorded per-source tracks only have
	// audio.wav. That remains the compatible default for old metadata.
	return metadata.AudioPath
}

func run(cfg config.Config, metadata *session.Metadata, profile models.Profile, audioPath string, mixed bool) int {
	dir := session.SessionDir(metadata)
	metadata.Status = "transcribing"
	metadata.TranscriptionEngine = profile.Engine
	metadata.Model = profile.Model
	metadata.Language = profile.Language
	if err := session.SaveMetadata(metadata); err != nil {
		return reportError(err)
	}
	appendLog(filepath.Join(dir, session.StatusLogFile), "starting retranscription audio_source="+audioSourceName(mixed))
	_, _ = events.Append(dir, "retranscription.starting", map[string]any{
		"profile": profile.ID, "engine": profile.Engine, "model": profile.Model,
		"language": profile.Language, "audio_path": audioPath, "audio_source": audioSourceName(mixed),
	})
	desktop.Notify(cfg, "📝 Retranscribing speech", "Using "+profile.ID+".")
	desktop.Play(cfg, "transcription_start")
	if err := transcription.Start(cfg, metadata, profile.ID); err != nil {
		return failSession(cfg, metadata, err)
	}
	defer transcription.Finish(cfg)
	transcript, err := transcription.Transcribe(
		profile,
		audioPath,
		metadata.TranscriptRawPath,
		metadata.TranscriptCleanPath,
		func(pid int) error { return transcription.SetWorkerPID(cfg, pid) },
	)
	if err != nil {
		return failSession(cfg, metadata, fmt.Errorf("retranscription failed: %w", err))
	}
	_, _ = events.Append(dir, "retranscription.completed", map[string]any{
		"raw_path": metadata.TranscriptRawPath, "clean_path": metadata.TranscriptCleanPath,
		"transcript_chars": len([]rune(transcript)), "audio_source": audioSourceName(mixed),
	})

	copied, clipboardMessage := desktop.CopyText(transcript)
	appendLog(filepath.Join(dir, session.StatusLogFile), clipboardMessage)
	_, _ = events.Append(dir, "clipboard.copy", map[string]any{
		"ok": copied, "message": clipboardMessage, "transcript_chars": len([]rune(transcript)),
	})
	if !copied {
		return failSession(cfg, metadata, errors.New(clipboardMessage))
	}
	appendLog(filepath.Join(dir, session.StatusLogFile), "automatic paste skipped; transcript left on clipboard")
	_, _ = events.Append(dir, "paste.skipped", map[string]any{
		"reason": "automatic_paste_disabled", "session_type": metadata.SessionType,
	})
	falseValue := false
	metadata.Status = "complete"
	metadata.PasteAttempted = &falseValue
	metadata.PasteHelperSucceeded = &falseValue
	metadata.PasteSucceeded = &falseValue
	metadata.PasteConfirmation = "not_attempted_automatic_paste_disabled"
	if err := session.SaveMetadata(metadata); err != nil {
		return reportError(err)
	}
	desktop.Notify(cfg, "✅ Risper copied", "Transcript is on the clipboard.")
	desktop.Play(cfg, "success")
	fmt.Println(transcript)
	return 0
}

func failSession(cfg config.Config, metadata *session.Metadata, err error) int {
	message := err.Error()
	if !strings.HasPrefix(message, "retranscription failed:") {
		message = "retranscription failed: " + message
	}
	dir := session.SessionDir(metadata)
	appendLog(filepath.Join(dir, session.ErrorLogFile), message)
	appendLog(filepath.Join(dir, session.StatusLogFile), message)
	_, _ = events.Append(dir, "retranscription.failed", map[string]any{
		"error": strings.TrimPrefix(message, "retranscription failed: "), "error_type": fmt.Sprintf("%T", err),
	})
	metadata.Status = "failed"
	metadata.Errors = append(metadata.Errors, message)
	if saveErr := session.SaveMetadata(metadata); saveErr != nil {
		fmt.Fprintln(os.Stderr, "risper retranscribe:", saveErr)
		return 1
	}
	desktop.Notify(cfg, "⚠ Risper retranscription failed", "Audio was kept; see session error log.")
	desktop.Play(cfg, "error")
	fmt.Fprintln(os.Stderr, message)
	return 1
}

func audioSourceName(mixed bool) string {
	if mixed {
		return "mixed"
	}
	return "mic"
}

func appendLog(path, message string) error {
	handle, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer handle.Close()
	_, err = fmt.Fprintf(handle, "%s %s\n", nowRFC3339(), message)
	return err
}

func nowRFC3339() string {
	return timeNow().Format("2006-01-02T15:04:05Z07:00")
}

var timeNow = func() time.Time { return time.Now() }

func reportError(err error) int {
	fmt.Fprintln(os.Stderr, "risper retranscribe:", err)
	return 1
}
