package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rob137/risper/config"
	"github.com/rob137/risper/hotkeys"
	"github.com/rob137/risper/platforms"
	"github.com/rob137/risper/session"
	"github.com/rob137/risper/voice"
)

type fakeListener struct {
	started bool
	stopped bool
	logger  func(string)
}

type fakeVoiceListener struct {
	started bool
	stopped bool
	logger  func(string)
}

func (listener *fakeVoiceListener) Start() (bool, string) {
	listener.started = true
	return true, "fake voice trigger listener started"
}

func (listener *fakeVoiceListener) Stop() { listener.stopped = true }

func (listener *fakeVoiceListener) SetLogger(logger func(string)) { listener.logger = logger }

func (listener *fakeListener) Start() (bool, string) {
	listener.started = true
	return true, "fake double-alt listener started"
}

func (listener *fakeListener) Stop() { listener.stopped = true }

func (listener *fakeListener) SetLogger(logger func(string)) { listener.logger = logger }

func daemonConfig(t *testing.T) config.Config {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestRunRecoversSessionsAndPrunesAudioAtStartup(t *testing.T) {
	cfg := daemonConfig(t)
	retention := 3600.0
	cfg.AudioRetentionSeconds = &retention
	cfg.DoubleAltEnabled = false
	now := time.Now()
	incomplete, err := session.CreateAt(cfg, now.Add(-2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	old, err := session.CreateAt(cfg, now.Add(-3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old.AudioPath, []byte("old audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	old.AudioSourcePaths = map[string]string{
		"mic": filepath.Join(session.SessionDir(old), "audio.mic.wav"),
	}
	if err := os.WriteFile(old.AudioSourcePaths["mic"], []byte("old mic"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := session.SaveMetadata(old); err != nil {
		t.Fatal(err)
	}
	stateData, err := json.Marshal(map[string]any{"recorder_pids": map[string]int{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.CurrentStatePath, stateData, 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := now.Add(-3 * time.Hour)
	for _, path := range []string{old.AudioPath, old.AudioSourcePaths["mic"]} {
		if err := os.Chtimes(path, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := RunWithOptions(ctx, cfg, Options{Notify: func(config.Config, string, string) {}}); err != nil {
		t.Fatal(err)
	}

	recovered, err := session.LoadSession(session.SessionDir(incomplete))
	if err != nil || recovered == nil || recovered.Status != "recovered" {
		t.Fatalf("incomplete session = %#v, %v", recovered, err)
	}
	pruned, err := session.LoadSession(session.SessionDir(old))
	if err != nil || pruned == nil || pruned.AudioPrunedAt == nil {
		t.Fatalf("pruned session = %#v, %v", pruned, err)
	}
	if _, err := os.Stat(old.AudioPath); !os.IsNotExist(err) {
		t.Fatalf("old audio still exists: %v", err)
	}
	if _, err := os.Stat(cfg.CurrentStatePath); !os.IsNotExist(err) {
		t.Fatalf("stale recording state still exists: %v", err)
	}
	var event map[string]any
	data, err := os.ReadFile(filepath.Join(session.SessionDir(old), session.EventsFile))
	if err != nil {
		t.Fatal(err)
	}
	last := lastLine(string(data))
	if err := json.Unmarshal([]byte(last), &event); err != nil || event["event"] != "audio.pruned" {
		t.Fatalf("last prune event = %s, %v", last, err)
	}
}

func TestRunStartsAndStopsConfiguredListener(t *testing.T) {
	cfg := daemonConfig(t)
	cfg.DoubleAltEnabled = true
	ctx, cancel := context.WithCancel(context.Background())
	listener := &fakeListener{}
	var factory platforms.DoubleAltListener
	factory = listener
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	if err := RunWithOptions(ctx, cfg, Options{
		ListenerFactory: func(int, func(hotkeys.Gesture)) platforms.DoubleAltListener { return factory },
		Notify:          func(config.Config, string, string) {},
		RetryInterval:   time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}
	if !listener.started || !listener.stopped {
		t.Fatalf("listener lifecycle started=%v stopped=%v", listener.started, listener.stopped)
	}
}

func TestRunStartsAndStopsConfiguredVoiceListener(t *testing.T) {
	cfg := daemonConfig(t)
	cfg.VoiceTriggersEnabled = true
	ctx, cancel := context.WithCancel(context.Background())
	listener := &fakeVoiceListener{}
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	if err := RunWithOptions(ctx, cfg, Options{
		VoiceListenerFactory: func(config.Config, func(voice.Action)) voice.Listener { return listener },
		Notify:               func(config.Config, string, string) {},
		RetryInterval:        time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}
	if !listener.started || !listener.stopped {
		t.Fatalf("voice listener lifecycle started=%v stopped=%v", listener.started, listener.stopped)
	}
}

func TestVoiceActionsMapToSafeToggleArguments(t *testing.T) {
	for _, test := range []struct {
		action voice.Action
		want   []string
	}{
		{voice.ActionStart, nil},
		{voice.ActionStop, []string{"--paste", "--voice-stop"}},
		{voice.ActionSend, []string{"--paste", "--enter", "--voice-send"}},
	} {
		got := voiceToggleArgs(test.action)
		if strings.Join(got, " ") != strings.Join(test.want, " ") {
			t.Errorf("voiceToggleArgs(%v) = %#v, want %#v", test.action, got, test.want)
		}
	}
}

func TestGesturesReachTheToggleCommand(t *testing.T) {
	cfg := daemonConfig(t)
	cfg.DoubleAltEnabled = true
	bin := t.TempDir()
	argsLog := filepath.Join(bin, "args.log")
	script := "#!/bin/sh\nprintf '[%s]\\n' \"$*\" >> " + argsLog + "\n"
	toggle := filepath.Join(bin, "risper-toggle")
	if err := os.WriteFile(toggle, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	triggers := make(chan func(hotkeys.Gesture), 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		trigger := <-triggers
		trigger(hotkeys.GestureDoubleAlt)
		trigger(hotkeys.GestureShiftDoubleAlt)
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	if err := RunWithOptions(ctx, cfg, Options{
		ListenerFactory: func(_ int, onTrigger func(hotkeys.Gesture)) platforms.DoubleAltListener {
			triggers <- onTrigger
			return &fakeListener{}
		},
		Notify:        func(config.Config, string, string) {},
		ToggleCommand: toggle,
		RetryInterval: time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	sort.Strings(lines)
	want := []string{"[--paste --enter]", "[--paste]"}
	if len(lines) != len(want) || lines[0] != want[0] || lines[1] != want[1] {
		t.Fatalf("toggle invocations = %#v, want %#v", lines, want)
	}
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return lines[len(lines)-1]
}
