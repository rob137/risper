# Risper

Risper is a local-first Ubuntu dictation utility. It is built around durable session folders: recording creates the folder and metadata before audio capture starts, and failures leave recoverable files behind.

Current state: phase 4 of the Go rewrite. Go owns the command surface, recording, mixing, local transcription, daemon recovery, audio retention, notifications, sounds, clipboard copy, and the optional Linux Double Alt listener. The Python implementation remains available during the migration; phase 5 removes it.
Risper deliberately leaves completed transcripts on the clipboard instead of trying to inject text into the focused app. On GNOME Wayland that path was not reliable enough to keep in the default workflow.

## Commands

- `risper`: enables autostart and starts the user daemon. `risper kill` stops it temporarily. The same binary provides the subcommands below.
- `risper toggle`: the Go recording/transcription cycle. `risper-toggle` remains the shortcut-compatible Go entry point.
- `risper-toggle`: start recording, stop recording on the next run, or cancel an active transcription. Both mic and computer output are captured into separate tracks; `--system` asks transcription to use their mixed audio.
- `risper history`: prints recent sessions and can open, play, copy, retranscribe, or delete a session by id, and `--prune-audio` applies `audio_retention` on demand.
- `risper open`: opens recordings, last session, last transcript, last audio, config, or copies the last transcript.
- `risper retranscribe`: retranscribes a saved session by id, or the last session by default. It uses the mic track by default; `--mixed` (or `--system`) selects the mixed mic-and-system audio.
- `risper models`: lists, selects, and adds local transcription model profiles.
- `risper benchmark`: measures transcription profile wall time, CPU use, and peak RSS.
- `risper diagnose`: prints OS checks, or `risper diagnose last` for a compact session diagnostic.
- `risper-daemon`: the Go daemon performs startup recovery, audio retention pruning, and the optional Linux Double Alt listener.
- `risper-status` and `risper-paste-test` remain Python-only and are intentionally not part of the Go port.

## Install

```bash
cd ~/personal/risper
./install-user.sh
```

This builds commands in `~/.local/bin` and installs a reversible user service file. It does not use root and does not install dependencies.
It also enables and starts the user daemon, so Risper starts automatically with your user session.

Manual development run without installing:

```bash
cd ~/personal/risper
PYTHONPATH=src python3 -m risper.diagnose
PYTHONPATH=src python3 -m risper.toggle
go run ./cmd/risper-daemon
```

## Verification

```bash
./scripts/test.sh
./scripts/mutation-smoke.sh
./scripts/mutmut.sh run
```

The mutation smoke deliberately breaks a copied version of the model-selection code and expects the tests to fail.
For proper mutation testing options, see `docs/mutation-testing.md`.

Performance measurements:

```bash
risper benchmark last --profile whispercpp-small-en
```

See `docs/performance.md`.

## Configure

The first run creates:

```text
~/.config/risper/config.toml
```

Important settings:

```toml
sessions_dir = "~/.local/share/risper/sessions"
selected_model = "whispercpp-small-en"
transcription_engine = "external"
transcription_command = ""
model = "small.en"
language = "en"
paste_mode = "clipboard_only"
play_sounds = true
double_alt_enabled = false
double_alt_window_ms = 350
audio_retention = "never" # never | <count>h | <count>d | <count>w
```

Model profiles live in:

```text
~/.config/risper/models.toml
```

Each profile is just a local command with metadata. It can use placeholders:

```text
{audio} {raw} {raw_no_txt} {clean} {clean_no_txt} {model} {language} {prompt}
```

Installed whisper.cpp shape:

```toml
transcription_engine = "whisper.cpp"
transcription_command = "/home/robert-kirby/.local/share/risper/engines/whisper.cpp/build/bin/whisper-cli -m /home/robert-kirby/.local/share/risper/engines/whisper.cpp/models/ggml-small.en.bin -f {audio} -l {language} -t 8 -nt -otxt -of {raw_no_txt}"
```

If the command writes `{raw}` or `{clean}` itself, Risper preserves those files. If it prints transcript text to stdout, Risper writes both `transcript.raw.txt` and `transcript.clean.txt`.

List/select profiles:

```bash
risper models list
risper models select whispercpp-small-en
```

Add another local backend profile:

```bash
risper models add-external my-engine \
  --engine some-engine \
  --model some-local-model \
  --language en \
  --command "/path/to/local-wrapper --model {model} --audio {audio}" \
  --select
```

The wrapper can either print transcript text to stdout or write `{raw}` directly. Risper does not care whether the backend is whisper.cpp, faster-whisper, or a future local engine as long as it is a local command.

To install or refresh whisper.cpp locally:

```bash
cd ~/personal/risper
./scripts/install-whispercpp.sh small.en
```

## Data

Sessions are stored as:

```text
~/.local/share/risper/sessions/
  2026-05-06_12-00-00/
    audio.wav
    audio.mic.wav
    audio.system.wav
    transcript.raw.txt
    transcript.clean.txt
    metadata.json
    events.jsonl
    status.log
    error.log
    pw-record.log
```

Every Go recording captures each source to `audio.mic.wav` and `audio.system.wav`, then blends them into `audio.wav`. The default transcription reads the mic track; `--system` reads the mixed track. All three files remain available until `audio_retention` prunes the session's audio.

`events.jsonl` is the structured debugging trail. It records workflow boundaries such as recorder start/stop, transcription, clipboard copy, skipped paste, and recovery. It does not store full transcript text by default.

Inspect the latest session without dumping transcript contents:

```bash
risper diagnose last
```

Transcripts and metadata are kept indefinitely. Audio is pruned at startup and hourly when `audio_retention` is set, and can also be pruned on demand with `risper history --prune-audio`.

## Shortcut

Bind this command in GNOME Settings, Keyboard, View and Customize Shortcuts, Custom Shortcuts:

```text
risper-toggle
```

Bind a second shortcut to `risper-toggle --system` for calls. Both sources are captured for every recording; the flag selects mixed transcription and is remembered if it was used to start the recording. Either shortcut stops a recording, because the sources are read back from the session state rather than the command line.

Double Alt is implemented as an optional Linux input-event listener in the Go `risper-daemon`, but it is disabled by default. It needs read access to `/dev/input/event*`; on GNOME Wayland that usually means explicit input-group or udev-rule setup. See `docs/double-alt.md`.

When Double Alt is enabled, tapping it during transcription cancels the active transcription instead of starting a new recording.

## Current Environment Findings

Dated snapshot, last verified 2026-08-17.

- Ubuntu 24.04.4 LTS, GNOME 46, Wayland.
- `pw-record`, `pw-play`, `pw-link`, `pw-dump`, `wpctl`, `ffmpeg`, `wl-copy`, `wtype`, `ydotool`, `notify-send`, `gio`, GTK 3, and `canberra-gtk-play` are available.
- `pactl`, `parec`, `sox`, `xdotool`, `dotool`, AppIndicator, `pip`, `faster-whisper`, and Python `whisper` are not available.
- System audio is captured by giving `pw-record` the `stream.capture.sink=true` property. With no `--target` it follows the default sink's monitor, so it tracks output device changes the way mic capture tracks the default source. Passing `--target` alone is not enough; without the property `pw-record` silently records the default source instead.
- Monitor capture is unity gain and reads before the output volume control, so turning the volume down does not quieten the recording.
- Changing the output device mid-recording is handled by PipeWire: the capture relinks to the new default sink's monitor and keeps going, including when a mono monitor is replaced by a stereo one.
- whisper.cpp with `ggml-small.en.bin` (default) and `ggml-base.en.bin` (fast fallback) is installed under `~/.local/share/risper/engines`.
- Automatic paste on GNOME Wayland was tested and removed from the default workflow because helper success did not reliably mean text appeared in the intended target. The transcript is copied to the clipboard instead.
- True tray/status-window UI is not part of the default workflow. Ubuntu notifications and the GNOME microphone indicator provide the lightweight feedback.

## Platform scope

The Go rewrite targets Rob's Ubuntu machine. Linux input handling has a small
platform boundary in `platforms/`; macOS and Windows starter adapters remain
Python-era material and are deliberately not part of this phase.

## License

MIT. See `LICENSE`.

## Uninstall

```bash
cd ~/personal/risper
./uninstall-user.sh
```

Uninstall keeps config, state, recordings, and transcripts.
