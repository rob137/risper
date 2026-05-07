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

## Paste Failed

On GNOME Wayland, apps generally cannot inject text globally without a helper. Risper copies the transcript first and only then attempts paste, so the recovery path is normal paste from clipboard.

Inspect the exact paste decision path for the latest session:

```bash
risper-diagnose last
```

The per-session `events.jsonl` file records the configured paste mode, session type, paste helper result, and whether Risper only confirmed helper launch rather than target-app insertion.

If `wtype` is installed but the message says the compositor does not support the virtual keyboard protocol, GNOME Wayland is rejecting that paste route. The transcript is still on the clipboard.

The current preferred fallback is `ydotool`, which uses Linux uinput instead of GNOME's virtual-keyboard protocol:

```bash
cd ~/personal/risper
./scripts/setup-ydotool.sh
risper-paste-test
```

## Status Window Did Not Appear

The daemon starts `risper-monitor`, a persistent GTK status process. It is configured as a non-focusable notification-style window, but GNOME Wayland can still affect placement and visibility. Recording is independent of the status window.

Inspect the UI lifecycle trail:

```bash
risper-diagnose last
tail -n 40 ~/.local/state/risper/risper.log
tail -n 40 ~/.local/state/risper/status-window.stderr.log
```

Useful lines include `status-window process started`, `status_window.started`, `status_window.show_requested`, `status_window.mapped`, and `status_window.state_changed`.

## Daemon Is Not Running

The toggle command does not require the daemon. The daemon is useful for startup recovery:

```bash
systemctl --user status risper.service
systemctl --user enable --now risper.service
```
