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
- `audio.wav`, `metadata.json`, `status.log`, `error.log`, and `pw-record.log` are present.
- `audio.wav` should be playable.
- Metadata status becomes `complete` or `paste_failed` after transcription.
- `transcript.raw.txt` and `transcript.clean.txt` are created.
- The audio remains available if transcription fails.
- Start/stop notifications and sounds are attempted.
- Overlay appears while recording if GTK can create a small Wayland window.
- Overlay shows a live mic-level bar when `pw-cat` can sample the microphone.
- Overlay changes state during transcription and briefly shows completion/failure.

History:

```bash
PYTHONPATH=src python3 -m risper.history
PYTHONPATH=src python3 -m risper.open recordings
PYTHONPATH=src python3 -m risper.open last-session
PYTHONPATH=src python3 -m risper.retranscribe last
PYTHONPATH=src python3 -m risper.model_cli list
PYTHONPATH=src python3 -m risper.status_window
PYTHONPATH=src python3 -m risper.benchmark last --profile whispercpp-base-en --profile parakeet-tdt-0-6b-v3
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
- Tray/status menu, because AppIndicator libraries are unavailable.
- True tray indicator, because AppIndicator libraries are unavailable. The GTK status window is the current fallback.
- Double Alt unless `double_alt_enabled = true` and the daemon has read access to `/dev/input/event*`.
