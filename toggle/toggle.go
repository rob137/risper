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
	"github.com/rob137/risper/voice"
)

const version = "0.1.0"

const (
	// pasteSettleMS lets the window that gained focus during transcription
	// finish drawing before the paste shortcut arrives.
	pasteSettleMS = 150
	// returnSettleMS gives the target time to accept the pasted text, because
	// Return arriving first submits an empty field.
	returnSettleMS = 300
)

// finish says what should happen once the transcript reaches the clipboard. It
// only applies to the run that stops a recording; the run that starts one has
// no transcript to place.
type finish struct {
	paste       bool
	enter       bool
	triggerWord string
}

func Main(args []string) int {
	parser := flag.NewFlagSet("risper-toggle", flag.ContinueOnError)
	parser.SetOutput(os.Stderr)
	paste := parser.Bool("paste", false, "replay a paste into the focused window once the transcript is copied")
	enter := parser.Bool("enter", false, "press Return after pasting")
	voiceStop := parser.Bool("voice-stop", false, "remove the configured voice stop word from the completed transcript")
	voiceSend := parser.Bool("voice-send", false, "remove the configured voice send word from the completed transcript")
	if err := parser.Parse(args); err != nil {
		return 2
	}
	if parser.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "risper-toggle: unexpected positional argument")
		return 2
	}
	if *enter && !*paste {
		fmt.Fprintln(os.Stderr, "risper-toggle: --enter needs --paste")
		return 2
	}
	if *voiceStop && *voiceSend {
		fmt.Fprintln(os.Stderr, "risper-toggle: --voice-stop and --voice-send are mutually exclusive")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		return fail(cfg, err)
	}
	request := finish{paste: *paste, enter: *enter}
	if *voiceStop {
		request.triggerWord = cfg.VoiceStopWord
	} else if *voiceSend {
		request.triggerWord = cfg.VoiceSendWord
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
		return finishSession(cfg, metadata, request)
	}

	state, err := recording.Start(cfg)
	if err != nil {
		return fail(cfg, err)
	}
	desktop.Notify(cfg, "🎙 Risper listening to mic and computer", "Run risper-toggle again to stop.")
	// A voice-started toggle must return to the listener as soon as recording
	// is established; the sound helper can finish independently.
	desktop.PlayAsync(cfg, "recording_start")
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

func finishSession(cfg config.Config, metadata *session.Metadata, request finish) int {
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
	// The start sound is useful feedback, but waiting for it here puts a
	// multi-second theme sample between stopping the recording and the
	// transcript reaching the clipboard. Start it now and wait only at the
	// lifecycle boundary, after the pipeline has completed.
	transcriptionSound := desktop.PlayAsync(cfg, "transcription_start")
	defer transcriptionSound.Wait()
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
	if request.triggerWord != "" {
		transcript = voice.StripTrailingTrigger(transcript, request.triggerWord)
		if err := transcription.WriteTranscript(metadata.TranscriptRawPath, metadata.TranscriptCleanPath, transcript); err != nil {
			return transcriptionFailure(cfg, metadata, err)
		}
	}
	_, _ = events.Append(session.SessionDir(metadata), "transcription.completed", map[string]any{
		"raw_path": metadata.TranscriptRawPath, "clean_path": metadata.TranscriptCleanPath,
		"transcript_chars": len([]rune(transcript)), "audio_source": audioSource,
		"voice_trigger": request.triggerWord,
	})

	copied, clipboardMessage := desktop.CopyText(transcript)
	appendLog(filepath.Join(session.SessionDir(metadata), session.StatusLogFile), clipboardMessage)
	_, _ = events.Append(session.SessionDir(metadata), "clipboard.copy", map[string]any{
		"ok": copied, "message": clipboardMessage, "transcript_chars": len([]rune(transcript)),
	})
	if !copied {
		return transcriptionFailure(cfg, metadata, errors.New(clipboardMessage))
	}
	pasted, submitted, pasteMessage, confirmation := placeTranscript(cfg, request)
	appendLog(filepath.Join(session.SessionDir(metadata), session.StatusLogFile), pasteMessage)
	if request.paste {
		_, _ = events.Append(session.SessionDir(metadata), "paste.result", map[string]any{
			"ok": pasted, "enter": request.enter, "keys": cfg.PasteKeys,
			"message": pasteMessage, "confirmation": confirmation,
			"session_type": metadata.SessionType,
		})
	} else {
		_, _ = events.Append(session.SessionDir(metadata), "paste.skipped", map[string]any{
			"reason": "automatic_paste_disabled", "session_type": metadata.SessionType,
		})
	}
	attempted := request.paste
	falseValue := false
	metadata.Status = "complete"
	metadata.Errors = []string{}
	metadata.PasteAttempted = &attempted
	metadata.PasteHelperSucceeded = &pasted
	// The clipboard is the only outcome Risper can observe. Whether the target
	// window took the keys is never confirmed, so paste_succeeded stays false.
	metadata.PasteSucceeded = &falseValue
	metadata.PasteConfirmation = confirmation
	if err := session.SaveMetadata(metadata); err != nil {
		return fail(cfg, err)
	}
	if request.paste && pasted {
		desktop.Notify(cfg, "✅ Risper pasted", "Sent to the focused window; transcript is on the clipboard.")
	} else if request.paste {
		desktop.Notify(cfg, "✅ Risper copied", "Paste unavailable; transcript is on the clipboard.")
	} else {
		desktop.Notify(cfg, "✅ Risper copied", "Transcript is on the clipboard.")
	}
	if submitted {
		// The send sound is intentionally fire-and-forget here. The daemon uses
		// toggle process exit as its in-flight boundary, so waiting for the
		// 1.29s rising pair would create a deadband before the next trigger.
		desktop.PlayAsync(cfg, "success_send")
	} else {
		desktop.PlayAsync(cfg, "success")
	}
	return 0
}

// placeTranscript replays the paste shortcut, and then Return, into whatever
// window holds focus. Each ydotool call waits before pressing, which also
// gives the target time to accept the paste before Return arrives.
func placeTranscript(cfg config.Config, request finish) (bool, bool, string, string) {
	if !request.paste {
		return false, false, "automatic paste skipped; transcript left on clipboard", "not_attempted_automatic_paste_disabled"
	}
	pasted, message := desktop.SendKeys(cfg.PasteKeys, pasteSettleMS)
	if !pasted {
		return false, false, message, "not_pasted_clipboard_retained"
	}
	if !request.enter {
		return true, false, message, "helper_ran_target_unverified"
	}
	sent, enterMessage := desktop.SendKeys("enter", returnSettleMS)
	if !sent {
		return true, false, message + "; " + enterMessage, "pasted_but_return_not_sent"
	}
	return true, true, message + "; " + enterMessage, "helper_ran_target_unverified"
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
