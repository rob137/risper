from __future__ import annotations

import signal
import subprocess
import sys
import time

from .config import load_config
from .platforms import current_platform
from .sessions import mark_incomplete_recordings_recovered
from .util import append_log, notify


running = True


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
    notify("Risper double Alt unavailable", message)
    return None


def main() -> int:
    signal.signal(signal.SIGTERM, _stop)
    signal.signal(signal.SIGINT, _stop)
    config = load_config()
    recovered = mark_incomplete_recordings_recovered(config)
    append_log(config.log_path, f"daemon started; recovered={recovered}")
    if recovered:
        notify("Risper recovered sessions", f"{recovered} incomplete session(s) marked recovered.")
    hotkey_listener = _start_double_alt_listener(config)
    while running:
        time.sleep(1)
    if hotkey_listener:
        hotkey_listener.stop()
    append_log(config.log_path, "daemon stopped")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
