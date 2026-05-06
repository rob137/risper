from __future__ import annotations

import os
import signal
import subprocess
from pathlib import Path

from .config import command_exists
from .util import pid_alive, wait_until


class RecorderBackend:
    name = "unknown"
    log_name = "recorder.log"

    def available(self) -> bool:
        return False

    def start(self, audio_path: Path, stderr_path: Path) -> subprocess.Popen:
        raise NotImplementedError

    def stop(self, pid: int) -> None:
        if pid_alive(pid):
            os.kill(pid, signal.SIGINT)


class PipeWireRecorderBackend(RecorderBackend):
    name = "pw-record"
    log_name = "pw-record.log"

    def available(self) -> bool:
        return command_exists("pw-record")

    def start(self, audio_path: Path, stderr_path: Path) -> subprocess.Popen:
        return subprocess.Popen(
            ["pw-record", "--rate", "16000", "--channels", "1", "--format", "s16", str(audio_path)],
            stdout=subprocess.DEVNULL,
            stderr=stderr_path.open("ab"),
            start_new_session=True,
        )

    def stop(self, pid: int) -> None:
        if pid_alive(pid):
            try:
                os.killpg(pid, signal.SIGINT)
            except ProcessLookupError:
                pass
            except Exception:
                os.kill(pid, signal.SIGINT)
            wait_until(lambda: not pid_alive(pid), timeout_seconds=4)
        if pid_alive(pid):
            try:
                os.killpg(pid, signal.SIGTERM)
            except Exception:
                os.kill(pid, signal.SIGTERM)
            wait_until(lambda: not pid_alive(pid), timeout_seconds=2)


def default_recorder_backend() -> RecorderBackend:
    return PipeWireRecorderBackend()
