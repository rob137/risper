# Risper

Risper is a local-first Ubuntu dictation utility. It is built around durable session folders: recording creates the folder and metadata before audio capture starts, and failures leave recoverable files behind.

Current state: phase 1/2 with a small daemon and CLI history. Recording, local transcription via selectable local models, session metadata, notifications, sounds, CLI history, and clipboard copy are implemented.
Risper deliberately leaves completed transcripts on the clipboard instead of trying to inject text into the focused app. On GNOME Wayland that path was not reliable enough to keep in the default workflow.

## Commands

- `risper`: enables autostart and starts the user daemon. `risper kill` stops it temporarily.
- `risper-toggle`: start recording, stop recording on the next run, or cancel an active transcription.
- `risper-daemon`: marks incomplete sessions recovered on startup and stays alive for systemd.
- `risper-open`: opens recordings, last session, last transcript, last audio, config, or copies the last transcript.
- `risper-paste-test`: diagnostic helper for paste experiments with a real focused GTK text field.
- `risper-history`: prints recent sessions and can open, play, copy, retranscribe, or delete a session by id, and `--prune-audio` applies `audio_retention` on demand.
- `risper-retranscribe`: retranscribes a saved session by id, or the last session by default.
- `risper-models`: lists, selects, and adds local transcription model profiles.
- `risper-status`: opens the GTK control/history window.
- `risper-benchmark`: measures transcription profile wall time, CPU use, and peak RSS.
- `risper-diagnose`: prints OS checks, or `risper-diagnose last` for a compact session diagnostic.

## Install

```bash
cd ~/personal/risper
./install-user.sh
```

This creates wrappers in `~/.local/bin` and installs a reversible user service file. It does not use root and does not install dependencies.
It also enables and starts the user daemon, so Risper starts automatically with your user session.

Manual development run without installing:

```bash
cd ~/personal/risper
PYTHONPATH=src python3 -m risper.diagnose
PYTHONPATH=src python3 -m risper.toggle
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
risper-benchmark last --profile whispercpp-small-en
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
risper-models list
risper-models select whispercpp-small-en
```

Add another local backend profile:

```bash
risper-models add-external my-engine \
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
    transcript.raw.txt
    transcript.clean.txt
    metadata.json
    events.jsonl
    status.log
    error.log
    pw-record.log
```

`events.jsonl` is the structured debugging trail. It records workflow boundaries such as recorder start/stop, transcription, clipboard copy, skipped paste, and recovery. It does not store full transcript text by default.

Inspect the latest session without dumping transcript contents:

```bash
risper-diagnose last
```

Recordings are never deleted automatically.

## Shortcut

Bind this command in GNOME Settings, Keyboard, View and Customize Shortcuts, Custom Shortcuts:

```text
risper-toggle
```

Double Alt is implemented as an optional Linux input-event listener in `risper-daemon`, but it is disabled by default. It needs read access to `/dev/input/event*`; on GNOME Wayland that usually means explicit input-group or udev-rule setup. See `docs/double-alt.md`.

When Double Alt is enabled, tapping it during transcription cancels the active transcription instead of starting a new recording.

## Current Environment Findings

Dated snapshot, last verified 2026-07-06.

- Ubuntu 24.04.4 LTS, GNOME 46, Wayland.
- `pw-record`, `wl-copy`, `wtype`, `ydotool`, `notify-send`, `gio`, GTK 3, and `canberra-gtk-play` are available.
- `pactl`, `ffmpeg`, `xdotool`, `dotool`, AppIndicator, `pip`, `faster-whisper`, and Python `whisper` are not available.
- whisper.cpp with `ggml-small.en.bin` (default) and `ggml-base.en.bin` (fast fallback) is installed under `~/.local/share/risper/engines`.
- Automatic paste on GNOME Wayland was tested and removed from the default workflow because helper success did not reliably mean text appeared in the intended target. The transcript is copied to the clipboard instead.
- True tray/status-window UI is not part of the default workflow. Ubuntu notifications and the GNOME microphone indicator provide the lightweight feedback.

## Portability

Risper is Ubuntu-first today, but the code now keeps desktop integration behind `src/risper/platforms/` and audio capture behind `src/risper/recorders.py`. See `docs/portability.md`.

Future macOS/Windows work should add platform adapters and recorder backends rather than changing the dictation/session/transcription flow.

## License

MIT. See `LICENSE`.

## Uninstall

```bash
cd ~/personal/risper
./uninstall-user.sh
```

Uninstall keeps config, state, recordings, and transcripts.
