# Double Alt

Double Alt is the preferred trigger, but GNOME Wayland intentionally restricts global keyboard capture and synthetic input.

Current implementation:

- The normal GNOME custom shortcut remains the safest default.
- `risper-daemon` can optionally start a Linux `/dev/input/event*` listener.
- It is disabled by default: set `double_alt_enabled = true` in `~/.config/risper/config.toml`.
- It requires read access to input event devices, which normally needs explicit input-group or udev-rule setup.
- It starts `risper-toggle` when a clean double Alt tap is detected.

Correct double Alt semantics:

- Count only Alt press and release with no other key participating.
- Ignore Alt+Tab, Alt+F4, menu shortcuts, and other combinations.
- Use a configurable double-tap window, default 350 ms.
- Do not swallow or remap Alt globally without explicit approval.

Possible future approaches:

- A packaged helper reading `/dev/input/event*`; this usually needs input group access or udev rules.
- `keyd` or interception-tools config; these are system-level and require careful approval.
- GNOME Shell extension; this may fit Wayland better but is more specialized.
