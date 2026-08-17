# Troubleshooting

## No Audio File

Run:

```bash
risper-diagnose
```

Check whether `pw-record` is present. Session-specific recorder stderr is stored in `pw-record.log`, and in `pw-record.system.log` for the `--system` source.

## Nothing From The Other Side Of A Call

`risper-diagnose last` reports `audio_sources`, and a source that captured nothing is listed in the session errors. Check that the far end was actually playing through the default sink at the time: `wpctl status` marks it with `*`, and `pw-link -l` shows what was linked to it. Output volume is not the cause, because monitor capture reads before the volume control.

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

On GNOME Wayland, apps generally cannot inject text globally without a helper. Risper therefore copies the transcript to the clipboard and does not attempt automatic paste during normal dictation.

Inspect the latest session:

```bash
risper-diagnose last
```

The per-session `events.jsonl` file records clipboard copy and the skipped-paste decision.

Older sessions may contain `paste_attempted`. That means a helper such as `ydotool` exited successfully, but target-app insertion was not verified. The transcript remained on the clipboard.

The old paste verification harness is still available for experiments:

```bash
risper-paste-test
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
