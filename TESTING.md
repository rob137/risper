# Risper Testing

Run diagnostics:

```bash
cd ~/personal/risper
PYTHONPATH=src python3 -m risper.diagnose
```

Run unit tests:

```bash
./scripts/test.sh
```

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

History:

```bash
PYTHONPATH=src python3 -m risper.history
PYTHONPATH=src python3 -m risper.open recordings
PYTHONPATH=src python3 -m risper.open last-session
PYTHONPATH=src python3 -m risper.retranscribe last
PYTHONPATH=src python3 -m risper.model_cli list
PYTHONPATH=src python3 -m risper.status_window
PYTHONPATH=src python3 -m risper.paste_test
PYTHONPATH=src python3 -m risper.paste_now --mode ydotool
PYTHONPATH=src python3 -m risper.benchmark last --profile whispercpp-base-en --profile parakeet-tdt-0-6b-v3
```

Inspect the latest diagnostic trail:

```bash
PYTHONPATH=src python3 -m risper.diagnose last
```

Parakeet profile check:

```bash
./scripts/add-parakeet-profile.sh
risper-models list
```

Daemon recovery smoke test:

```bash
PYTHONPATH=src python3 -m risper.toggle
pkill -INT pw-record
PYTHONPATH=src python3 -m risper.daemon
```

The daemon should mark any still-`recording` sessions as `recovered`.

Still environment-limited:

- Automatic paste into arbitrary Wayland apps.
- True tray indicator or standalone recording window. Ubuntu notifications and the GNOME microphone indicator are the current feedback path.
- Double Alt unless `double_alt_enabled = true` and the daemon has read access to `/dev/input/event*`.
