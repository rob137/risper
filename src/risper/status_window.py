from __future__ import annotations

import subprocess
import sys
import threading
from pathlib import Path

from .config import load_config
from .recorder import current_recording
from .retranscribe import retranscribe_session
from .session_actions import copy_transcript, open_session, play_audio, transcript_preview
from .sessions import all_sessions


def format_duration(metadata: dict) -> str:
    duration = metadata.get("duration_seconds")
    return "" if duration is None else f"{duration}s"


def format_started(metadata: dict) -> str:
    return str(metadata.get("session_id", ""))


def main() -> int:
    try:
        import gi

        gi.require_version("Gtk", "3.0")
        from gi.repository import GLib, Gtk
    except Exception as exc:
        print(f"GTK unavailable: {exc}", file=sys.stderr)
        return 1

    config = load_config()
    selected: dict | None = None

    window = Gtk.Window(title="Risper")
    window.set_default_size(820, 420)
    window.set_border_width(10)
    window.connect("destroy", Gtk.main_quit)

    outer = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=8)
    window.add(outer)

    header = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=8)
    outer.pack_start(header, False, False, 0)

    status_label = Gtk.Label()
    status_label.set_xalign(0)
    header.pack_start(status_label, True, True, 0)

    toggle_button = Gtk.Button(label="Start")
    refresh_button = Gtk.Button(label="Refresh")
    recordings_button = Gtk.Button(label="Recordings")
    header.pack_start(toggle_button, False, False, 0)
    header.pack_start(refresh_button, False, False, 0)
    header.pack_start(recordings_button, False, False, 0)

    store = Gtk.ListStore(str, str, str, str, str)
    tree = Gtk.TreeView(model=store)
    tree.set_headers_visible(True)
    for index, title in enumerate(["Session", "Status", "Duration", "Preview"]):
        renderer = Gtk.CellRendererText()
        if title == "Preview":
            renderer.set_property("ellipsize", 3)
        column = Gtk.TreeViewColumn(title, renderer, text=index)
        column.set_resizable(True)
        tree.append_column(column)

    selection = tree.get_selection()
    scroller = Gtk.ScrolledWindow()
    scroller.set_policy(Gtk.PolicyType.AUTOMATIC, Gtk.PolicyType.AUTOMATIC)
    scroller.add(tree)
    outer.pack_start(scroller, True, True, 0)

    actions = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=8)
    outer.pack_start(actions, False, False, 0)
    open_button = Gtk.Button(label="Open")
    play_button = Gtk.Button(label="Play")
    copy_button = Gtk.Button(label="Copy")
    retranscribe_button = Gtk.Button(label="Retranscribe")
    for button in (open_button, play_button, copy_button, retranscribe_button):
        actions.pack_start(button, False, False, 0)

    message_label = Gtk.Label()
    message_label.set_xalign(0)
    outer.pack_start(message_label, False, False, 0)

    sessions_by_id: dict[str, dict] = {}

    def set_message(message: str) -> None:
        message_label.set_text(message)

    def refresh() -> bool:
        nonlocal sessions_by_id, selected
        state = current_recording(config)
        if state:
            status_label.set_markup(f"<b>Recording</b>  {state['session_dir']}")
            toggle_button.set_label("Stop")
        else:
            status_label.set_markup("<b>Idle</b>")
            toggle_button.set_label("Start")

        sessions = all_sessions(config)[:50]
        selected_id = selected.get("session_id") if selected else None
        sessions_by_id = {str(item.get("session_id")): item for item in sessions}
        store.clear()
        selected_iter = None
        for item in sessions:
            row = [
                str(item.get("session_id", "")),
                str(item.get("status", "")),
                format_duration(item),
                transcript_preview(item, limit=140),
                str(item.get("audio_path", "")),
            ]
            iterator = store.append(row)
            if row[0] == selected_id:
                selected_iter = iterator
        if selected_iter:
            selection.select_iter(selected_iter)
        return True

    def selected_metadata() -> dict | None:
        model, iterator = selection.get_selected()
        if iterator is None:
            return None
        session_id = model[iterator][0]
        return sessions_by_id.get(session_id)

    def on_selection_changed(_selection) -> None:
        nonlocal selected
        selected = selected_metadata()

    def run_action(action, success_prefix: str) -> None:
        metadata = selected_metadata()
        if not metadata:
            set_message("Select a session first.")
            return
        ok, message = action(metadata)
        set_message(f"{success_prefix}: {message}" if ok else message)

    def run_retranscribe() -> None:
        metadata = selected_metadata()
        if not metadata:
            set_message("Select a session first.")
            return
        session_id = str(metadata.get("session_id"))
        set_message(f"Retranscribing {session_id}...")

        def worker() -> None:
            code = retranscribe_session(session_id, copy=False, paste=False)
            GLib.idle_add(set_message, "Retranscribed." if code == 0 else "Retranscription failed.")
            GLib.idle_add(refresh)

        threading.Thread(target=worker, daemon=True).start()

    def run_toggle(_button) -> None:
        subprocess.Popen(
            [sys.executable, "-m", "risper.toggle"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        GLib.timeout_add(600, refresh)

    def open_recordings(_button) -> None:
        from .platforms import current_platform

        ok, message = current_platform().open_path(config.sessions_dir)
        set_message(message if ok else f"Could not open recordings: {message}")

    selection.connect("changed", on_selection_changed)
    toggle_button.connect("clicked", run_toggle)
    refresh_button.connect("clicked", lambda _button: refresh())
    recordings_button.connect("clicked", open_recordings)
    open_button.connect("clicked", lambda _button: run_action(open_session, "Open"))
    play_button.connect("clicked", lambda _button: run_action(play_audio, "Play"))
    copy_button.connect("clicked", lambda _button: run_action(copy_transcript, "Copy"))
    retranscribe_button.connect("clicked", lambda _button: run_retranscribe())

    refresh()
    GLib.timeout_add(1500, refresh)
    window.show_all()
    Gtk.main()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
