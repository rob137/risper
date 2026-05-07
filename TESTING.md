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
- Metadata status becomes `complete` or `paste_failed` after transcription.
- `transcript.raw.txt` and `transcript.clean.txt` are created.
- `events.jsonl` records recorder, transcription, clipboard, and paste boundary events without storing transcript text.
- The audio remains available if transcription fails.
- Start/stop notifications and sounds are attempted.
- The daemon-owned status monitor appears while recording/transcribing/pasting if GTK can create a normal Wayland window.
- The status monitor shows a live mic-level bar when `pw-cat` can sample the microphone.
- The status monitor logs `status_window.*` lifecycle and state-change lines in `~/.local/state/risper/risper.log`.

History:

```bash
PYTHONPATH=src python3 -m risper.history
PYTHONPATH=src python3 -m risper.open recordings
PYTHONPATH=src python3 -m risper.open last-session
PYTHONPATH=src python3 -m risper.retranscribe last
PYTHONPATH=src python3 -m risper.model_cli list
PYTHONPATH=src python3 -m risper.status_window
PYTHONPATH=src python3 -m risper.monitor
PYTHONPATH=src python3 -m risper.paste_test
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

- Paste into arbitrary Wayland apps unless `wtype`, `dotool`, or `ydotool` is installed and permitted.
- True tray indicator, because AppIndicator libraries are unavailable. The GTK status window is the current fallback.
- Double Alt unless `double_alt_enabled = true` and the daemon has read access to `/dev/input/event*`.
