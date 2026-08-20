// Package daemon owns the small amount of work that must survive independently
// of a recording toggle: recovery, audio retention, and the global hotkey.
package daemon

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/desktop"
	"github.com/rob137/risper/hotkeys"
	"github.com/rob137/risper/platforms"
	"github.com/rob137/risper/recording"
	"github.com/rob137/risper/session"
)

const (
	PruneInterval = time.Hour
	retryInterval = time.Second
)

type Options struct {
	ListenerFactory func(windowMS int, onTrigger func(hotkeys.Gesture)) platforms.DoubleAltListener
	Notify          func(config.Config, string, string)
	ToggleCommand   string
	PruneInterval   time.Duration
	RetryInterval   time.Duration
}

func (options Options) withDefaults() Options {
	if options.ListenerFactory == nil {
		options.ListenerFactory = func(windowMS int, onTrigger func(hotkeys.Gesture)) platforms.DoubleAltListener {
			return platforms.NewLinuxDoubleAltListener(windowMS, onTrigger)
		}
	}
	if options.Notify == nil {
		options.Notify = desktop.Notify
	}
	if options.ToggleCommand == "" {
		options.ToggleCommand = "risper-toggle"
	}
	if options.PruneInterval <= 0 {
		options.PruneInterval = PruneInterval
	}
	if options.RetryInterval <= 0 {
		options.RetryInterval = retryInterval
	}
	return options
}

// Main is the service entry point used by cmd/risper-daemon. Signal handling
// lives here rather than in the worker so RunWithOptions remains deterministic
// and easy to exercise in tests.
func Main() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "risper-daemon:", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := Run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "risper-daemon:", err)
		return 1
	}
	return 0
}

func Run(ctx context.Context, cfg config.Config) error {
	return RunWithOptions(ctx, cfg, Options{})
}

// RunWithOptions performs startup work before entering the service loop. It
// never changes config or session audio except for the explicitly configured
// retention policy; all session mutations are the same durable operations
// used by the command surface.
func RunWithOptions(ctx context.Context, cfg config.Config, rawOptions Options) error {
	options := rawOptions.withDefaults()
	if _, err := recording.Current(cfg); err != nil {
		return fmt.Errorf("clear stale recording state: %w", err)
	}
	recovered, err := session.MarkIncompleteRecordingsRecovered(cfg)
	if err != nil {
		return fmt.Errorf("recover incomplete recordings: %w", err)
	}
	appendLog(cfg.LogPath, fmt.Sprintf("daemon started; recovered=%d; audio_retention=%s", recovered, retentionText(cfg)))
	if recovered > 0 {
		options.Notify(cfg, "♻ Risper recovered sessions", fmt.Sprintf("%d incomplete session(s) marked recovered.", recovered))
	}
	pruneAudio(cfg)

	var listener platforms.DoubleAltListener
	notifiedUnavailable := false
	startListener := func() {
		if !cfg.DoubleAltEnabled || !platforms.IsLinux() || listener != nil {
			return
		}
		candidate := options.ListenerFactory(cfg.DoubleAltWindowMS, func(gesture hotkeys.Gesture) {
			startToggle(cfg, options, gesture)
		})
		if candidate == nil {
			appendLog(cfg.LogPath, "double-alt listener unavailable: factory returned no listener")
			return
		}
		candidate.SetLogger(func(message string) { appendLog(cfg.LogPath, message) })
		ok, message := candidate.Start()
		appendLog(cfg.LogPath, message)
		if ok {
			listener = candidate
			notifiedUnavailable = false
			return
		}
		candidate.Stop()
		if !notifiedUnavailable {
			options.Notify(cfg, "⚠ Risper double Alt unavailable", message)
			notifiedUnavailable = true
		}
	}
	if cfg.DoubleAltEnabled && !platforms.IsLinux() {
		appendLog(cfg.LogPath, "double-alt disabled; input listener is Linux-only")
	} else {
		startListener()
	}

	pruneTicker := time.NewTicker(options.PruneInterval)
	defer pruneTicker.Stop()
	retryTicker := time.NewTicker(options.RetryInterval)
	defer retryTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			if listener != nil {
				listener.Stop()
			}
			appendLog(cfg.LogPath, "daemon stopped")
			return nil
		case <-pruneTicker.C:
			pruneAudio(cfg)
		case <-retryTicker.C:
			startListener()
		}
	}
}

func retentionText(cfg config.Config) string {
	if cfg.AudioRetentionSeconds == nil {
		return "never"
	}
	return fmt.Sprintf("%.0fs", *cfg.AudioRetentionSeconds)
}

func pruneAudio(cfg config.Config) {
	count, err := session.PruneExpiredAudio(cfg)
	if err != nil {
		appendLog(cfg.LogPath, "audio prune failed: "+err.Error())
		return
	}
	if count > 0 {
		appendLog(cfg.LogPath, fmt.Sprintf("pruned audio from %d expired session(s)", count))
	}
}

// toggleArgs maps a gesture onto the toggle command surface. Both gestures
// paste, because a transcript that only reaches the clipboard still needs a
// keystroke to be useful; Shift adds the Return that submits it. Running
// risper-toggle with no arguments stays clipboard-only, which is the escape
// hatch for anywhere the configured paste keys do not work.
func toggleArgs(gesture hotkeys.Gesture) []string {
	switch gesture {
	case hotkeys.GestureShiftDoubleAlt:
		return []string{"--paste", "--enter"}
	case hotkeys.GestureDoubleAlt:
		return []string{"--paste"}
	default:
		return nil
	}
}

func startToggle(cfg config.Config, options Options, gesture hotkeys.Gesture) {
	command := options.ToggleCommand
	path, err := exec.LookPath(command)
	if err != nil {
		if executable, executableErr := os.Executable(); executableErr == nil {
			candidate := filepath.Join(filepath.Dir(executable), command)
			if _, statErr := os.Stat(candidate); statErr == nil {
				path = candidate
				err = nil
			}
		}
	}
	if err != nil {
		appendLog(cfg.LogPath, "double-alt toggle unavailable: "+err.Error())
		return
	}
	args := toggleArgs(gesture)
	cmd := exec.Command(path, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		appendLog(cfg.LogPath, "double-alt toggle failed to start: "+err.Error())
		return
	}
	if len(args) > 0 {
		appendLog(cfg.LogPath, "double-alt trigger "+strings.Join(args, " "))
	} else {
		appendLog(cfg.LogPath, "double-alt trigger")
	}
	go func() { _ = cmd.Wait() }()
}

func appendLog(path, message string) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	handle, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer handle.Close()
	_, _ = fmt.Fprintf(handle, "%s %s\n", time.Now().Format(time.RFC3339), message)
}
