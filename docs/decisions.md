# Decisions

## 2026-05-06 Initial Implementation

- Project name, package, commands, config paths, and data paths use `risper`.
- Recording uses `pw-record` because it is installed and fits the GNOME Wayland/PipeWire environment.
- The first implementation did not download a transcription model or install Python packages. After the follow-up request to continue, whisper.cpp was installed user-locally and the `base.en` model was downloaded.
- Transcription is a local external command hook pointed at whisper.cpp. This keeps the core recoverable while leaving engine choice reversible.
- Model selection is profile-based in `~/.config/risper/models.toml`. This is deliberately lighter than a settings UI and makes future engines such as Parakeet addable via a wrapper command.
- Desktop integration is behind `platforms/`, and recording is behind `recorders.py`. Linux is implemented now; macOS/Windows have starter adapters so future portability work has a clear target.
- Parakeet works on this laptop, but process-per-dictation performance is poor compared with whisper.cpp. Keep Parakeet available, but consider whisper.cpp the fast default unless a persistent worker is added.
- Paste is fail-soft. On this Wayland setup, no `wtype`, `ydotool`, `dotool`, or X11 `xdotool` path exists, so clipboard fallback is expected.
- The daemon is deliberately small. Its current useful job is startup recovery; the toggle command is independently usable for GNOME custom shortcuts.
- AppIndicator tray work is deferred because the current Python environment lacks AppIndicator/Ayatana namespaces.
- Double Alt is deferred because implementing it correctly on Wayland requires input-event access or a lower-level key remapper. That should be explicitly approved before setup.
