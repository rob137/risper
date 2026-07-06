from __future__ import annotations

import glob
import signal
import subprocess
import sys
import time
from pathlib import Path
from typing import Any

from .config import load_config
from .platforms import current_platform
from .sessions import mark_incomplete_recordings_recovered
from .util import append_log, notify


running = True
RESUME_GAP_SECONDS = 30.0


def _stop(_signum, _frame) -> None:
    global running
    running = False


def _start_double_alt_listener(config):
    if not config.double_alt_enabled:
        return None
    if current_platform().name != "linux":
        append_log(config.log_path, "double-alt disabled; platform input listener is Linux-only")
        return None

    from .platforms.linux_hotkey import LinuxDoubleAltListener

    def trigger_toggle() -> None:
        append_log(config.log_path, "double-alt trigger")
        subprocess.Popen(
            [sys.executable, "-m", "risper.toggle"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )

    listener = LinuxDoubleAltListener(config.double_alt_window_ms, trigger_toggle)
    ok, message = listener.start()
    append_log(config.log_path, message)
    if ok:
        return listener
    notify("⚠ Risper double Alt unavailable", message)
    return None


def _input_device_signature() -> tuple[tuple[str, int, int], ...]:
    signature: list[tuple[str, int, int]] = []
    for path in sorted(glob.glob("/dev/input/event*")):
        try:
            details = Path(path).stat()
        except OSError:
            continue
        signature.append((path, details.st_rdev, details.st_mtime_ns))
    return tuple(signature)


def _refresh_reason(
    last_wall: float,
    last_mono: float,
    previous_devices: tuple[tuple[str, int, int], ...],
    current_devices: tuple[tuple[str, int, int], ...],
    *,
    pending_devices: tuple[tuple[str, int, int], ...] | None = None,
    now_wall: float | None = None,
    now_mono: float | None = None,
    resume_gap_seconds: float = RESUME_GAP_SECONDS,
) -> str | None:
    now_wall = time.time() if now_wall is None else now_wall
    now_mono = time.monotonic() if now_mono is None else now_mono
    wall_elapsed = now_wall - last_wall
    mono_elapsed = now_mono - last_mono
    if wall_elapsed - mono_elapsed > resume_gap_seconds:
        return "resume detected"
    # devices must hold the same changed shape for a full tick before we
    # restart: dock/undock enumerates devices one by one and used to fire
    # several restarts within a second
    if current_devices != previous_devices and current_devices == pending_devices:
        return "input devices changed"
    return None


def _stop_listener(listener: Any | None) -> None:
    if not listener:
        return
    try:
        listener.stop()
    except Exception:
        return


def _restart_double_alt_listener(config, listener, reason: str):
    append_log(config.log_path, f"double-alt listener restarting: {reason}")
    _stop_listener(listener)
    return _start_double_alt_listener(config)


def main() -> int:
    signal.signal(signal.SIGTERM, _stop)
    signal.signal(signal.SIGINT, _stop)
    config = load_config()
    recovered = mark_incomplete_recordings_recovered(config)
    append_log(config.log_path, f"daemon started; recovered={recovered}")
    if recovered:
        notify("♻ Risper recovered sessions", f"{recovered} incomplete session(s) marked recovered.")
    if config.show_overlay:
        append_log(config.log_path, "status-window ignored; standalone monitor is disabled")
    hotkey_listener = _start_double_alt_listener(config)
    last_wall = time.time()
    last_mono = time.monotonic()
    input_signature = _input_device_signature()
    pending_signature = None
    while running:
        time.sleep(1)
        current_signature = _input_device_signature()
        reason = _refresh_reason(
            last_wall,
            last_mono,
            input_signature,
            current_signature,
            pending_devices=pending_signature,
        )
        if reason and config.double_alt_enabled and current_platform().name == "linux":
            hotkey_listener = _restart_double_alt_listener(config, hotkey_listener, reason)
            input_signature = _input_device_signature()
            pending_signature = None
        elif current_signature != input_signature:
            pending_signature = current_signature
        else:
            pending_signature = None
        last_wall = time.time()
        last_mono = time.monotonic()
    if hotkey_listener:
        hotkey_listener.stop()
    append_log(config.log_path, "daemon stopped")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
