# Decisions

## 2026-05-06 Initial Implementation

- Project name, package, commands, config paths, and data paths use `risper`.
- Recording uses `pw-record` because it is installed and fits the GNOME Wayland/PipeWire environment.
- The first implementation did not download a transcription model or install Python packages. After the follow-up request to continue, whisper.cpp was installed user-locally and the `base.en` model was downloaded.
- Transcription is a local external command hook pointed at whisper.cpp. This keeps the core recoverable while leaving engine choice reversible.
- Model selection is profile-based in `~/.config/risper/models.toml`. This is deliberately lighter than a settings UI and makes future engines such as Parakeet addable via a wrapper command.
- Desktop integration is behind `platforms/`, and recording is behind `recorders.py`. Linux is implemented now; macOS/Windows have starter adapters so future portability work has a clear target.
- Parakeet works on this laptop, but process-per-dictation performance is poor compared with whisper.cpp. It is currently selected for quality testing; consider whisper.cpp the fast fallback unless a persistent worker is added.
- Paste is fail-soft. On this Wayland setup, no `wtype`, `ydotool`, `dotool`, or X11 `xdotool` path exists, so clipboard fallback is expected.
- The daemon is deliberately small. Its current useful job is startup recovery; the toggle command is independently usable for GNOME custom shortcuts.
- AppIndicator tray work is deferred because the current Python environment lacks AppIndicator/Ayatana namespaces.
- Double Alt is deferred because implementing it correctly on Wayland requires input-event access or a lower-level key remapper. That should be explicitly approved before setup.

## 2026-07-06 Audit pass

- The rename to `risper` and the publish to `github.com/rob137/risper` both completed in May 2026; their one-shot task briefs (`docs/rename-to-risper.md`, `docs/publish.md`) are folded into this line and deleted.
- The standalone status monitor/overlay chain (`monitor.py`, `overlay.py`, `audiolevel.py`, the `show_overlay` config knob) is removed. It was dead: nothing in `src/` imported it and the daemon explicitly ignored the knob. `status_window.py` (`risper-status`) is the one status UI. If a mic-level display comes back, resurrect from git history rather than keeping unreferenced code warm.
- Retention stays `retention = "never"`: recordings are still never deleted automatically. Runaway forgotten-toggle sessions (multi-hour WAVs whose transcription was cancelled) get pruned by hand; automatic audio expiry is deferred until manual pruning actually hurts.
