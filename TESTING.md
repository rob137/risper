# Risper Testing

Run the automated checks from the repository root:

```bash
./scripts/test.sh
go test ./...
./scripts/mutation-smoke.sh
```

`go test ./...` includes the recording/transcription cycle and command tests. The functional tests put temporary stubs for `pw-record`, `ffmpeg`, `whisper-cli`, `wl-copy`, `notify-send`, and `canberra-gtk-play` on `PATH`. They do not read or write the live sessions under `~/.local/share/risper/sessions`.

The mutation smoke copies the repository, deliberately breaks selected-model resolution, and expects the Go tests to fail.

## Manual microphone check

Use the installed command surface for this check:

```bash
risper diagnose
risper-toggle
```

Speak for a few seconds, then stop:

```bash
risper-toggle
```

Expected:

- A session folder appears immediately under `~/.local/share/risper/sessions`.
- `audio.wav`, `metadata.json`, `events.jsonl`, `status.log`, `error.log`, and `pw-record.log` are present.
- `audio.wav` is playable.
- Metadata becomes `complete` after successful transcription and clipboard copy.
- `transcript.raw.txt` and `transcript.clean.txt` are created.
- `events.jsonl` records recorder, transcription, clipboard, and skipped-paste boundaries without transcript text.
- Audio remains available if transcription fails.
- Recording-start, transcription-start, success/error notifications and sounds are attempted; stopping moves directly into transcription rather than showing a separate stop notification.
- A transcription-start notification appears after stopping recording.
- Running `risper-toggle` during transcription cancels it.
- No daemon-owned status window appears during dictation.

## System audio

With something playing through the current output device, use `--system` to select the mixed mic-and-system track for transcription:

```bash
risper-toggle --system
risper-toggle
```

Expected:

- Two recorder processes are present while recording; `pgrep -a pw-record` shows one with `stream.capture.sink=true`.
- `pw-record.system.log` is present and `risper diagnose last` reports `audio_sources mic,system`.
- `audio.wav`, `audio.mic.wav`, and `audio.system.wav` remain available until retention prunes them.
- The transcript contains your words and the playing audio.
- `events.jsonl` records `recorder.mixed` with the sources used.

## History and aliases

```bash
risper history
risper open recordings
risper open last-session
risper retranscribe last
risper models list
risper benchmark last --profile whispercpp-small-en
risper diagnose last
risper-paste-test
```

The `risper-*` commands in `~/.local/bin` are compatibility wrappers over the matching Go subcommands. `risper-status` reports `risper.service` status; it does not open a GTK status window. `risper-paste-test` copies a marker to the clipboard for a manual check; it does not run the old paste experiment, and automatic paste is not part of normal dictation.

## Daemon recovery smoke test

Only run this when it is safe to exercise the daemon manually:

```bash
risper-toggle
pkill -INT pw-record
risper-daemon
```

The daemon should mark any still-`recording` sessions as `recovered`; stop the foreground daemon with Ctrl-C when the check is complete.

Still environment-limited:

- Automatic paste into arbitrary Wayland apps.
- A standalone tray indicator or recording window. Notifications and the GNOME microphone indicator are the current feedback path.
- Double Alt unless `double_alt_enabled = true` and the daemon can read `/dev/input/event*`.
