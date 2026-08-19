# Risper Testing

Run diagnostics:

```bash
cd ~/personal/risper
PYTHONPATH=src python3 -m risper.diagnose
```

Run unit tests:

```bash
./scripts/test.sh
go test ./...
```

`go test ./...` includes the Phase 2 functional cycle and Phase 3 command tests. It places temporary
stubs for `pw-record`, `ffmpeg`, `whisper-cli`, `wl-copy`, `notify-send`, and
`canberra-gtk-play` on `PATH`, then runs the Go toggle through recording,
mixing, mic-only transcription, mixed transcription, clipboard copy, and
event checks. It does not read or write the live sessions under
`~/.local/share/risper/sessions`.

Run mutation smoke:

```bash
./scripts/mutation-smoke.sh
./scripts/mutmut.sh run
```

The mutation smoke copies the repo to `/tmp`, deliberately breaks selected-model resolution, and expects the tests to fail.

Start recording:

```bash
PYTHONPATH=src python3 -m risper.toggle
```

Speak for a few seconds, then stop:

```bash
PYTHONPATH=src python3 -m risper.toggle
```

Expected:

- A session folder appears immediately under `~/.local/share/risper/sessions`.
- `audio.wav`, `metadata.json`, `events.jsonl`, `status.log`, `error.log`, and `pw-record.log` are present.
- `audio.wav` should be playable.
- Metadata status becomes `complete` after successful transcription and clipboard copy.
- `transcript.raw.txt` and `transcript.clean.txt` are created.
- `events.jsonl` records recorder, transcription, clipboard, and skipped-paste boundary events without storing transcript text.
- The audio remains available if transcription fails.
- Start/stop notifications and sounds are attempted.
- A transcription-start notification appears after stopping recording.
- Running `risper-toggle` during transcription cancels the active transcription.
- No daemon-owned status window should appear during dictation.

System audio, with something playing through the current output device:

```bash
PYTHONPATH=src python3 -m risper.toggle --system
PYTHONPATH=src python3 -m risper.toggle
```

Expected:

- Two recorder processes while recording: `pgrep -a pw-record` shows one with `stream.capture.sink=true`.
- `pw-record.system.log` is present and `risper diagnose last` reports `audio_sources mic,system`.
- `audio.wav`, `audio.mic.wav`, and `audio.system.wav` remain available until `audio_retention` prunes them.
- The transcript contains both your own words and what was playing.
- `events.jsonl` records `recorder.mixed` with the sources used.

History:

```bash
go run ./cmd/risper history
go run ./cmd/risper open recordings
go run ./cmd/risper open last-session
go run ./cmd/risper retranscribe last
go run ./cmd/risper models list
PYTHONPATH=src python3 -m risper.status_window
PYTHONPATH=src python3 -m risper.paste_test
go run ./cmd/risper benchmark last --profile whispercpp-small-en
```

Inspect the latest diagnostic trail:

```bash
go run ./cmd/risper diagnose last
```

Daemon recovery smoke test:

```bash
go run ./cmd/risper-toggle
pkill -INT pw-record
go run ./cmd/risper-daemon
```

The daemon should mark any still-`recording` sessions as `recovered`.

Still environment-limited:

- Automatic paste into arbitrary Wayland apps.
- True tray indicator or standalone recording window. Ubuntu notifications and the GNOME microphone indicator are the current feedback path.
- Double Alt unless `double_alt_enabled = true` and the daemon has read access to `/dev/input/event*`.
