# Portability

Risper is Ubuntu-first today, but the code should keep platform-specific work behind small adapters.

Portable core:

- session folders and metadata
- model profile registry
- transcription command contract
- history/retranscribe logic
- config/state paths through XDG-style helpers

Platform-specific surfaces:

- recording backend: `recorders.py`
- clipboard, paste, notify, sound, open/trash: `platforms/`
- overlay/tray/global hotkey: future platform modules or separate helpers
- install/autostart scripts: `install-user.sh`, `systemd/`, `desktop/`

Current support:

- Linux/GNOME/Wayland: implemented with `pw-record`, `wl-copy`, `notify-send`, `canberra-gtk-play`, and `gio`.
- macOS: starter adapter for clipboard, paste, notifications, sound, and open. Recording/autostart/hotkey still need implementation.
- Windows: starter adapter for clipboard and open. Recording, paste, notifications, autostart, and hotkey still need implementation.

Rules for adding a platform:

1. Do not put platform commands directly in `toggle.py`, `retranscribe.py`, `history.py`, or `sessions.py`.
2. Add desktop behavior in `platforms/<platform>.py`.
3. Add audio capture behavior as a `RecorderBackend`.
4. Keep transcription as a model profile command unless a backend needs a real Python API wrapper.
5. Keep every session file-compatible across platforms.

Likely future backends:

- macOS recording: `ffmpeg` with AVFoundation, or a small native helper.
- Windows recording: `ffmpeg` with WASAPI, or a small native helper.
- macOS hotkey: app/service using Carbon/EventTap or a tiny native helper.
- Windows hotkey: `RegisterHotKey` helper or a small tray app.

The intended migration path is not “rewrite in Electron.” It is to keep the core boring and add thin platform-specific adapters.
