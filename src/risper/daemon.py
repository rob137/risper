from __future__ import annotations

from dataclasses import dataclass
import signal
import subprocess
import sys
import time
from typing import IO

from .config import load_config
from .platforms import current_platform
from .sessions import mark_incomplete_recordings_recovered
from .util import append_log, notify


running = True


@dataclass
class ChildProcess:
    name: str
    process: subprocess.Popen
    log_handle: IO[bytes]

    def close(self) -> None:
        self.log_handle.close()


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


def _start_status_window(config) -> ChildProcess | None:
    if not config.show_overlay:
        append_log(config.log_path, "status-window disabled by show_overlay=false")
        return None
    stderr_path = config.state_dir / "status-window.stderr.log"
    stderr_path.parent.mkdir(parents=True, exist_ok=True)
    handle = stderr_path.open("ab")
    process = subprocess.Popen(
        [sys.executable, "-m", "risper.monitor"],
        stdout=handle,
        stderr=handle,
    )
    append_log(config.log_path, f"status-window process started pid={process.pid} log={stderr_path}")
    return ChildProcess("status-window", process, handle)


def _stop_child(config, child: ChildProcess | None) -> None:
    if not child:
        return
    if child.process.poll() is None:
        child.process.terminate()
        try:
            child.process.wait(timeout=2)
        except subprocess.TimeoutExpired:
            child.process.kill()
            child.process.wait(timeout=2)
    child.close()
    append_log(config.log_path, f"{child.name} process stopped")


def main() -> int:
    signal.signal(signal.SIGTERM, _stop)
    signal.signal(signal.SIGINT, _stop)
    config = load_config()
    recovered = mark_incomplete_recordings_recovered(config)
    append_log(config.log_path, f"daemon started; recovered={recovered}")
    if recovered:
        notify("Risper recovered sessions", f"{recovered} incomplete session(s) marked recovered.")
    status_window = _start_status_window(config)
    status_window_restarts = 0
    hotkey_listener = _start_double_alt_listener(config)
    while running:
        if status_window and status_window.process.poll() is not None:
            code = status_window.process.returncode
            status_window.close()
            append_log(config.log_path, f"status-window process exited code={code}")
            status_window = None
            if config.show_overlay and status_window_restarts < 3:
                status_window_restarts += 1
                status_window = _start_status_window(config)
        time.sleep(1)
    if hotkey_listener:
        hotkey_listener.stop()
    _stop_child(config, status_window)
    append_log(config.log_path, "daemon stopped")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
