# Troubleshooting

## No Audio File

Run:

```bash
risper-diagnose
```

Check whether `pw-record` is present. Session-specific recorder stderr is stored in `pw-record.log`.

## Transcription Failed

This is expected until `transcription_command` is configured in:

```text
~/.config/risper/config.toml
```

The recording is still saved. Open the last session with:

```bash
risper-open last-session
```

## Clipboard And Paste

On GNOME Wayland, apps generally cannot inject text globally without a helper. Risper therefore copies the transcript to the clipboard by default. Automatic paste is opt-in through `auto_paste_after_copy = true`.

Inspect the latest session:

```bash
risper-diagnose last
```

The per-session `events.jsonl` file records clipboard copy and either the skipped-paste decision or the opt-in paste attempt.

Older sessions may contain `paste_attempted`. That means a helper such as `ydotool` exited successfully, but target-app insertion was not verified. The transcript remained on the clipboard.

The paste verification helpers are available for experiments:

```bash
risper-paste-test
risper-paste-now --mode ydotool
```

## Status Feedback

Risper no longer starts a standalone status window during dictation. Ubuntu notifications and the GNOME microphone indicator are the intended lightweight feedback. Recording and transcription details are still logged in the session folder and `~/.local/state/risper/risper.log`.

## Cancel Transcription

If transcription is still running, trigger `risper-toggle` again. With Double Alt enabled this means Double Alt once to start recording, Double Alt again to stop and transcribe, then Double Alt once more to cancel that transcription.

## Daemon Is Not Running

The toggle command does not require the daemon. The daemon is useful for startup recovery:

```bash
systemctl --user status risper.service
systemctl --user enable --now risper.service
```
