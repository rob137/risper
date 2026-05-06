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

## Overlay Did Not Appear

The overlay uses GTK 3. It is a best-effort small window and may be constrained by the compositor. Recording is independent of the overlay.

## Daemon Is Not Running

The toggle command does not require the daemon. The daemon is useful for startup recovery:

```bash
systemctl --user status risper.service
systemctl --user enable --now risper.service
```
