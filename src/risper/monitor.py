from __future__ import annotations

import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .audiolevel import MicLevelMonitor, level_to_bars
from .config import Config, load_config
from .recorder import current_recording
from .sessions import append_event, last_session, load_session
from .util import append_log


ACTIVE_STATUSES = {"recording", "recorded", "transcribing", "pasting"}
TERMINAL_STATUSES = {"complete", "paste_failed", "failed", "recovered"}
TERMINAL_HOLD_SECONDS = 8.0
SPINNER_FRAMES = ("|", "/", "-", "\\")


@dataclass(frozen=True)
class StatusSnapshot:
    status: str
    title: str
    detail: str
    session_id: str | None
    metadata: dict[str, Any] | None
    visible: bool

    @property
    def key(self) -> str:
        return f"{self.session_id or '-'}:{self.status}:{self.visible}"


def _title_for_status(status: str) -> str:
    return {
        "idle": "Risper idle",
        "recording": "Risper listening",
        "recorded": "Risper finishing",
        "transcribing": "Risper transcribing",
        "pasting": "Risper pasting",
        "complete": "Risper copied",
        "paste_failed": "Risper copied",
        "failed": "Risper failed",
        "recovered": "Risper recovered",
    }.get(status, f"Risper {status}")


def _detail_for_status(status: str, metadata: dict[str, Any] | None = None) -> str:
    if status == "idle":
        return "ready"
    if status == "recording":
        return "listening to microphone"
    if status == "recorded":
        return "saving audio"
    if status == "transcribing":
        return "working on transcript"
    if status == "pasting":
        return "attempting paste"
    if status == "complete":
        return "transcript copied"
    errors = list((metadata or {}).get("errors") or [])
    if status == "paste_failed":
        return str(errors[-1]) if errors else "paste unavailable; transcript copied"
    if status == "failed":
        return str(errors[-1]) if errors else "see session diagnostics"
    if status == "recovered":
        return str(errors[-1]) if errors else "incomplete recording recovered"
    return status


def _raw_snapshot(config: Config) -> StatusSnapshot:
    state = current_recording(config)
    if state:
        session_dir = Path(str(state["session_dir"]))
        metadata = load_session(session_dir)
        session_id = str(metadata.get("session_id")) if metadata else session_dir.name
        return StatusSnapshot(
            status="recording",
            title=_title_for_status("recording"),
            detail=_detail_for_status("recording", metadata),
            session_id=session_id,
            metadata=metadata,
            visible=True,
        )

    metadata = last_session(config)
    if not metadata:
        return StatusSnapshot("idle", _title_for_status("idle"), _detail_for_status("idle"), None, None, False)

    status = str(metadata.get("status", "idle"))
    if status not in ACTIVE_STATUSES | TERMINAL_STATUSES:
        status = "idle"
    return StatusSnapshot(
        status=status,
        title=_title_for_status(status),
        detail=_detail_for_status(status, metadata),
        session_id=str(metadata.get("session_id") or ""),
        metadata=metadata,
        visible=status in ACTIVE_STATUSES | TERMINAL_STATUSES,
    )


class StatusModel:
    def __init__(self, terminal_hold_seconds: float = TERMINAL_HOLD_SECONDS) -> None:
        self.terminal_hold_seconds = terminal_hold_seconds
        self._initialized = False
        self._terminal_key: str | None = None
        self._terminal_since = 0.0

    def snapshot(self, config: Config, now: float | None = None) -> StatusSnapshot:
        now = time.monotonic() if now is None else now
        raw = _raw_snapshot(config)
        if raw.status in ACTIVE_STATUSES:
            self._initialized = True
            return raw
        if raw.status not in TERMINAL_STATUSES:
            return StatusSnapshot(raw.status, raw.title, raw.detail, raw.session_id, raw.metadata, False)

        terminal_key = f"{raw.session_id or '-'}:{raw.status}"
        if not self._initialized:
            self._initialized = True
            self._terminal_key = terminal_key
            self._terminal_since = now - self.terminal_hold_seconds - 1
        elif terminal_key != self._terminal_key:
            self._terminal_key = terminal_key
            self._terminal_since = now

        visible = now - self._terminal_since <= self.terminal_hold_seconds
        return StatusSnapshot(raw.status, raw.title, raw.detail, raw.session_id, raw.metadata, visible)


def _display_detail(snapshot: StatusSnapshot, mic_monitor: MicLevelMonitor | None, frame_index: int) -> str:
    if snapshot.status == "recording":
        if mic_monitor and mic_monitor.available:
            return f"mic {level_to_bars(mic_monitor.level)}"
        return "mic meter unavailable"
    if snapshot.status in {"recorded", "transcribing", "pasting"}:
        return f"{SPINNER_FRAMES[frame_index % len(SPINNER_FRAMES)]} {snapshot.detail}"
    return snapshot.detail


def _log_snapshot(config: Config, snapshot: StatusSnapshot, event: str) -> None:
    append_log(
        config.log_path,
        f"{event} status={snapshot.status} visible={snapshot.visible} session={snapshot.session_id or '-'}",
    )
    if snapshot.metadata:
        append_event(
            snapshot.metadata,
            event,
            status=snapshot.status,
            visible=snapshot.visible,
            session_id=snapshot.session_id,
        )


def _configure_status_window(window: Any, type_hint: Any | None = None) -> None:
    window.set_default_size(360, 92)
    window.set_resizable(False)
    window.set_keep_above(True)
    window.set_border_width(12)
    window.set_decorated(False)
    window.set_accept_focus(False)
    window.set_focus_on_map(False)
    window.set_skip_taskbar_hint(True)
    window.set_skip_pager_hint(True)
    if type_hint is not None:
        window.set_type_hint(type_hint)


def main() -> int:
    config = load_config()
    append_log(config.log_path, "status_window.started")

    try:
        import gi

        gi.require_version("Gtk", "3.0")
        gi.require_version("Gdk", "3.0")
        from gi.repository import Gdk, GLib, Gtk
    except Exception as exc:
        append_log(config.log_path, f"status_window.unavailable error={exc.__class__.__name__}: {exc}")
        print(f"GTK unavailable: {exc}", file=sys.stderr)
        return 1

    model = StatusModel()
    mic_monitor: MicLevelMonitor | None = None
    last_key: str | None = None
    frame_index = 0

    window = Gtk.Window(title="Risper status")
    _configure_status_window(window, Gdk.WindowTypeHint.NOTIFICATION)
    append_log(config.log_path, "status_window.non_focusable_configured")

    outer = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=6)
    window.add(outer)

    title_label = Gtk.Label()
    title_label.set_xalign(0)
    outer.pack_start(title_label, False, False, 0)

    detail_label = Gtk.Label()
    detail_label.set_xalign(0)
    detail_label.set_line_wrap(True)
    detail_label.set_max_width_chars(48)
    outer.pack_start(detail_label, False, False, 0)

    def on_map(_window, _event) -> bool:
        append_log(config.log_path, "status_window.mapped")
        return False

    def on_unmap(_window, _event) -> bool:
        append_log(config.log_path, "status_window.unmapped")
        return False

    def on_destroy(_window) -> None:
        nonlocal mic_monitor
        append_log(config.log_path, "status_window.closed")
        if mic_monitor:
            mic_monitor.stop()
            mic_monitor = None
        Gtk.main_quit()

    window.connect("map-event", on_map)
    window.connect("unmap-event", on_unmap)
    window.connect("destroy", on_destroy)

    def tick() -> bool:
        nonlocal frame_index, last_key, mic_monitor
        snapshot = model.snapshot(config)
        frame_index += 1

        if snapshot.status == "recording":
            if mic_monitor is None:
                mic_monitor = MicLevelMonitor()
                mic_monitor.start()
        elif mic_monitor is not None:
            mic_monitor.stop()
            mic_monitor = None

        if snapshot.key != last_key:
            last_key = snapshot.key
            _log_snapshot(config, snapshot, "status_window.state_changed")

        if snapshot.visible:
            title_label.set_markup(f"<b>{snapshot.title}</b>")
            detail_label.set_text(_display_detail(snapshot, mic_monitor, frame_index))
            if not window.get_visible():
                append_log(config.log_path, f"status_window.show_requested status={snapshot.status}")
                window.show_all()
        elif window.get_visible():
            append_log(config.log_path, "status_window.hide_requested")
            window.hide()
        return True

    tick()
    GLib.timeout_add(250, tick)
    Gtk.main()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
