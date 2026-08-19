# Risper

Risper is a local-first Ubuntu dictation utility built around durable session folders. Recording creates the folder and metadata before audio capture starts, and failures leave recoverable files behind.

The Go rewrite is complete. Go owns recording, mixing, local transcription, clipboard copy, notifications, sounds, the command surface, daemon recovery, audio retention, and the optional Linux Double Alt listener.

## Commands

- `risper`: enables autostart and starts the user daemon. `risper kill` stops it temporarily.
- `risper toggle`: starts recording, stops it on the next run, or cancels an active transcription. Transcription includes the microphone and computer-output capture.
- `risper history`: lists recent sessions and opens, plays, copies, retranscribes, deletes, or prunes them.
- `risper open`: opens recordings, the last session, the last transcript, the last audio file, or the config.
- `risper retranscribe`: retranscribes a saved session from its mixed audio.
- `risper models`: lists, selects, and adds local transcription model profiles.
- `risper benchmark`: measures transcription profile wall time, CPU use, and peak RSS.
- `risper diagnose`: prints environment or session diagnostics.
- `risper paste-test`: copies a marker to the clipboard so clipboard access can be checked manually.
- `risper daemon`: runs the daemon in the foreground.

The installed compatibility commands are thin wrappers over those subcommands:
`risper-toggle`, `risper-daemon`, `risper-open`, `risper-paste-test`,
`risper-history`, `risper-retranscribe`, `risper-models`, `risper-status`,
`risper-benchmark`, and `risper-diagnose`.

`risper-status` is the service-status alias. The desktop launcher opens the terminal history view; there is no separate status window in the default workflow.

## Install

```bash
cd ~/personal/risper
./install-user.sh
```

The installer builds the Go binary, installs it and the compatibility wrappers in `~/.local/bin`, installs the user service and desktop launcher, then reloads and restarts the daemon. Run it again after source changes: installed commands are compiled binaries, so edits in the checkout are not live until the next install.

It uses the existing Go toolchain and does not install dependencies or touch recordings, transcripts, or config.

For a checkout-only development run:

```bash
go run ./cmd/risper diagnose
go run ./cmd/risper toggle
go run ./cmd/risper daemon
```

## Verification

```bash
./scripts/test.sh
go test ./...
./scripts/mutation-smoke.sh
```

The mutation smoke copies the repository, breaks selected-model resolution in the copy, and expects the Go tests to fail. No general mutation runner is part of the project.

Performance measurements:

```bash
risper benchmark last --profile whispercpp-small-en
```

See `docs/performance.md` for benchmark context. The focused mutation smoke is
kept as `./scripts/mutation-smoke.sh`; it does not require a general mutation
runner.

## Configure

The first command run creates:

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

Each profile is a local command with metadata. It can use these placeholders:

```text
{audio} {raw} {raw_no_txt} {clean} {clean_no_txt} {model} {language} {prompt}
```

Installed whisper.cpp shape:

```toml
transcription_engine = "whisper.cpp"
transcription_command = "/home/robert-kirby/.local/share/risper/engines/whisper.cpp/build/bin/whisper-cli -m /home/robert-kirby/.local/share/risper/engines/whisper.cpp/models/ggml-small.en.bin -f {audio} -l {language} -t 8 -nt -otxt -of {raw_no_txt} -mc 0"
```

List or select profiles:

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

Every Go recording captures microphone and computer output separately, then blends the sources into `audio.wav`. Transcription always reads that mixed file, while `audio.mic.wav` and `audio.system.wav` remain available for a later re-read. If one source is silent, it is dropped from the mix, so it adds no transcription content or cost. Audio remains until `audio_retention` prunes it.

`events.jsonl` is the structured debugging trail. It records workflow boundaries without storing full transcript text by default. Transcripts and metadata are retained indefinitely.

Inspect the latest session without dumping transcript contents:

```bash
risper diagnose last
```

## Shortcut

Bind this command in GNOME Settings, Keyboard, View and Customize Shortcuts, Custom Shortcuts:

```text
risper-toggle
```

Both sources are captured and included for every recording; the ordinary `risper-toggle` shortcut is all that is needed.

Double Alt is an optional Linux input-event listener in `risper-daemon`, disabled by default. It needs read access to `/dev/input/event*`; see `docs/double-alt.md`.

## Environment

Risper targets Rob's Ubuntu/GNOME/Wayland setup. The Linux integration uses `pw-record`, `ffmpeg`, `wl-copy`, `notify-send`, `canberra-gtk-play`, and `gio`. Automatic paste into arbitrary Wayland apps and a standalone tray/status window are deliberately outside the default workflow; completed transcripts remain on the clipboard.

## Uninstall

```bash
cd ~/personal/risper
./uninstall-user.sh
```

Uninstall keeps config, state, recordings, and transcripts.
