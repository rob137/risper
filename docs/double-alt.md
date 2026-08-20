# Double Alt

Double Alt is the preferred trigger, but GNOME Wayland intentionally restricts global keyboard capture and synthetic input.

Current implementation:

- The normal GNOME custom shortcut remains the safest default.
- `risper-daemon` can optionally start a Linux `/dev/input/event*` listener.
- It is disabled by default: set `double_alt_enabled = true` in `~/.config/risper/config.toml`.
- It requires read access to input event devices, which normally needs explicit input-group or udev-rule setup.
- It starts `risper-toggle` when a clean double Alt tap is detected.
- Holding Shift across both taps starts `risper-toggle --paste --enter` instead.

Correct double Alt semantics:

- Count only Alt press and release with no other key participating.
- Shift is the one exception: held before both Alt presses, it selects the
  paste variant rather than discarding the gesture.
- Ignore Alt+Tab, Alt+F4, menu shortcuts, and other combinations.
- Use a configurable double-tap window, default 350 ms.
- Do not swallow or remap Alt globally without explicit approval.

## Shift double Alt

Shift double Alt finishes the recording and then replays a paste, followed by
Return, into whatever window holds focus when transcription completes. That is
usually seconds after the gesture, so the transcript lands wherever the cursor
is *then*, not where it was when Rob stopped talking.

Injection goes through `ydotool`, which needs `ydotoold` running and write
access to `/dev/uinput`:

```bash
systemctl --user enable --now ydotoold
```

`ydotool` exits zero whatever it is handed, so a successful run means the keys
were sent, not that the target accepted them. The transcript stays on the
clipboard either way, and `paste_succeeded` in session metadata stays false.

The sequence is `paste_keys` in config, default `ctrl+v`. Terminals want
`ctrl+shift+v`; there is one setting, so pick the one that matches where
dictation usually lands.

ydotool 0.1.8 does not validate key names and mis-parses the long ones: it
turns `leftshift` into `KEY_L` and `f13` into `KEY_F`, exiting zero either way.
Stick to the short aliases. `ctrl+v`, `ctrl+shift+v`, and `enter` were checked
against the emitted keycodes; anything else needs the same check:

```bash
sudo evtest /dev/input/eventN   # the "ydotoold virtual device" node
```

Possible future approaches:

- A packaged helper reading `/dev/input/event*`; this usually needs input group access or udev rules.
- `keyd` or interception-tools config; these are system-level and require careful approval.
- GNOME Shell extension; this may fit Wayland better but is more specialized.
