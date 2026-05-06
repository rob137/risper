from __future__ import annotations

import sys
import time
from pathlib import Path

from .audiolevel import MicLevelMonitor, level_to_bars
from .sessions import load_session
from .util import pid_alive


TERMINAL_STATUSES = {"complete", "paste_failed", "failed", "recovered"}
BUSY_STATUSES = {"recorded", "transcribing", "pasting"}
SPINNER_FRAMES = ("|", "/", "-", "\\")


def _status_text(status: str) -> str:
    if status == "recording":
        return "● Listening"
    if status == "recorded":
        return "◌ Finishing..."
    if status == "transcribing":
        return "◌ Transcribing..."
    if status == "pasting":
        return "◌ Pasting..."
    if status == "complete":
        return "✓ Copied"
    if status == "paste_failed":
        return "✓ Copied - paste unavailable"
    if status == "failed":
        return "⚠ Failed"
    return status or "Risper"


def _activity_line(status: str, elapsed: int) -> str:
    if status not in BUSY_STATUSES:
        return f"{elapsed}s"
    frame = SPINNER_FRAMES[elapsed % len(SPINNER_FRAMES)]
    label = {
        "recorded": "saving audio",
        "transcribing": "working on transcript",
        "pasting": "attempting paste",
    }[status]
    return f"{elapsed}s  {frame} {label}"


def _read_status(session_dir: Path | None) -> str:
    if not session_dir:
        return "recording"
    metadata = load_session(session_dir)
    return str(metadata.get("status", "recording")) if metadata else "recording"


def main(argv: list[str] | None = None) -> int:
    argv = argv or sys.argv[1:]
    if not argv:
        return 2
    recorder_pid = int(argv[0])
    session_dir = Path(argv[1]) if len(argv) > 1 else None
    try:
        import gi

        gi.require_version("Gtk", "3.0")
        from gi.repository import Gdk, GLib, Gtk
    except Exception:
        while pid_alive(recorder_pid) or _read_status(session_dir) not in TERMINAL_STATUSES:
            time.sleep(0.5)
        return 0

    window = Gtk.Window(type=Gtk.WindowType.TOPLEVEL)
    window.set_title("Risper")
    window.set_decorated(False)
    window.set_keep_above(True)
    window.set_resizable(False)
    window.set_skip_taskbar_hint(True)
    window.set_skip_pager_hint(True)
    window.set_type_hint(Gdk.WindowTypeHint.NOTIFICATION)
    window.set_border_width(12)
    window.set_opacity(0.92)

    label = Gtk.Label()
    label.set_xalign(0)
    label.set_markup("<b>● Listening</b>\ninitialising meter")
    window.add(label)
    window.show_all()

    started = time.monotonic()
    terminal_since: float | None = None
    monitor = MicLevelMonitor()
    monitor.start()

    def tick() -> bool:
        nonlocal terminal_since
        status = _read_status(session_dir)
        recorder_alive = pid_alive(recorder_pid)
        if recorder_alive:
            status = "recording"
        if status in TERMINAL_STATUSES:
            terminal_since = terminal_since or time.monotonic()
        if terminal_since and time.monotonic() - terminal_since > 2.5:
            monitor.stop()
            Gtk.main_quit()
            return False
        elapsed = int(time.monotonic() - started)
        if status == "recording" and monitor.available:
            meter = level_to_bars(monitor.level)
        elif status == "recording":
            meter = "meter unavailable"
        else:
            meter = ""
        line_2 = f"{elapsed}s  {meter}" if meter else _activity_line(status, elapsed)
        label.set_markup(f"<b>{_status_text(status)}</b>\n{line_2}")
        return True

    GLib.timeout_add(250, tick)
    Gtk.main()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
