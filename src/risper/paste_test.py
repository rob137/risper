from __future__ import annotations

import argparse
import sys
import time

from .clipboard import copy_text
from .config import load_config
from .paste import attempt_paste
from .util import append_log


def make_marker() -> str:
    return f"RISPER_PASTE_TEST_{int(time.time())}"


def _attempt_marker_paste(marker: str) -> tuple[bool, str]:
    config = load_config()
    copied, copy_message = copy_text(marker)
    append_log(config.log_path, f"paste_test.copy ok={copied} message={copy_message}")
    if not copied:
        return False, copy_message

    pasted, paste_message = attempt_paste(config)
    append_log(config.log_path, f"paste_test.helper ok={pasted} message={paste_message}")
    if not pasted:
        return False, paste_message
    return True, paste_message


def _run_manual(delay_seconds: float) -> int:
    marker = make_marker()
    print(f"Marker: {marker}")
    print(f"Focus the target text field within {delay_seconds:g}s.")
    time.sleep(delay_seconds)
    ok, message = _attempt_marker_paste(marker)
    print(message)
    if ok:
        print("Check the target field for the marker above.")
        return 0
    return 1


def _run_gtk() -> int:
    marker = make_marker()
    config = load_config()
    append_log(config.log_path, f"paste_test.gtk started marker={marker}")

    try:
        import gi

        gi.require_version("Gtk", "3.0")
        from gi.repository import GLib, Gtk
    except Exception as exc:
        append_log(config.log_path, f"paste_test.gtk unavailable error={exc.__class__.__name__}: {exc}")
        print(f"GTK unavailable: {exc}", file=sys.stderr)
        return 1

    result: dict[str, object] = {"ok": False, "message": "paste test did not complete"}

    window = Gtk.Window(title="Risper paste test")
    window.set_default_size(520, 120)
    window.set_border_width(12)
    window.set_keep_above(True)

    box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=8)
    window.add(box)

    label = Gtk.Label(label="Risper paste verification")
    label.set_xalign(0)
    box.pack_start(label, False, False, 0)

    entry = Gtk.Entry()
    box.pack_start(entry, False, False, 0)

    status = Gtk.Label(label=f"Marker: {marker}")
    status.set_xalign(0)
    box.pack_start(status, False, False, 0)

    def finish(ok: bool, message: str) -> bool:
        result["ok"] = ok
        result["message"] = message
        append_log(config.log_path, f"paste_test.gtk result ok={ok} message={message}")
        status.set_text(message)
        GLib.timeout_add(900, Gtk.main_quit)
        return False

    def poll_for_marker(deadline: float) -> bool:
        current = entry.get_text()
        if marker in current:
            return finish(True, "verified: marker appeared in GTK text field")
        if time.monotonic() >= deadline:
            return finish(False, f"not verified: helper returned but field contains {current!r}")
        return True

    def attempt() -> bool:
        entry.grab_focus()
        ok, message = _attempt_marker_paste(marker)
        if not ok:
            return finish(False, message)
        GLib.timeout_add(100, poll_for_marker, time.monotonic() + 2.0)
        return False

    def start(_window, _event) -> bool:
        append_log(config.log_path, "paste_test.gtk mapped")
        entry.grab_focus()
        GLib.timeout_add(650, attempt)
        return False

    window.connect("map-event", start)
    window.connect("destroy", Gtk.main_quit)
    window.show_all()
    Gtk.main()

    print(result["message"])
    return 0 if result["ok"] else 1


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Verify Risper paste helpers.")
    parser.add_argument(
        "mode",
        nargs="?",
        choices=("gtk", "manual"),
        default="gtk",
        help="gtk verifies paste into a focused GTK field; manual gives you time to focus another app.",
    )
    parser.add_argument("--delay", type=float, default=5.0, help="Manual mode delay before paste attempt.")
    args = parser.parse_args(argv)

    if args.mode == "manual":
        return _run_manual(args.delay)
    return _run_gtk()


if __name__ == "__main__":
    raise SystemExit(main())
