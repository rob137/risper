# Portability

Risper is Ubuntu-first today, with platform-specific work kept behind small Go adapters.

Portable core:

- session folders and metadata
- model profile registry
- transcription command contract
- history and retranscription logic
- config and state paths through XDG-style helpers

Platform-specific surfaces:

- recording backend: `recording/`
- clipboard, notifications, sounds, opening, and trash: `desktop/`
- Linux input events: `platforms/`
- install and autostart: `install-user.sh` and `systemd/`

Current support:

- Linux/GNOME/Wayland: implemented with `pw-record`, `ffmpeg`, `wl-copy`, `notify-send`, `canberra-gtk-play`, and `gio`.

Rules for adding a platform:

1. Keep platform commands behind the relevant package boundary rather than placing them in workflow orchestration.
2. Add desktop behavior in `desktop/` and input behavior in `platforms/`.
3. Add audio capture behavior as a recorder backend and declare which sources it can capture.
4. Keep transcription as a model profile command unless a backend genuinely needs an in-process implementation.
5. Keep every session file-compatible across platforms.

Likely future backends:

- macOS recording: `ffmpeg` with AVFoundation, or a small native helper.
- Windows recording: `ffmpeg` with WASAPI, or a small native helper.
- macOS hotkey: an EventTap helper or a small native service.
- Windows hotkey: a `RegisterHotKey` helper or a small tray app.

The intended migration path is to keep the core boring and add thin platform-specific adapters.
