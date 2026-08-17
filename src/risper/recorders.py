from __future__ import annotations

import os
import signal
import subprocess
from collections.abc import Iterable
from pathlib import Path

from .config import command_exists
from .util import pid_alive, wait_until


MIC = "mic"
SYSTEM = "system"

WAV_HEADER_BYTES = 44


class RecorderBackend:
    name = "unknown"
    supported_sources: tuple[str, ...] = ()

    def available(self) -> bool:
        return False

    def log_name(self, source: str) -> str:
        return "recorder.log" if source == MIC else f"recorder.{source}.log"

    def start(self, source: str, audio_path: Path, stderr_path: Path) -> subprocess.Popen:
        raise NotImplementedError

    def stop_all(self, pids: Iterable[int]) -> None:
        # every source is signalled before any is waited on, so one slow exit
        # cannot leave the others recording past the end of the session
        for pid in pids:
            if pid_alive(pid):
                os.kill(pid, signal.SIGINT)

    def stop(self, pid: int) -> None:
        self.stop_all([pid])


class PipeWireRecorderBackend(RecorderBackend):
    name = "pw-record"
    supported_sources = (MIC, SYSTEM)

    def available(self) -> bool:
        return command_exists("pw-record")

    def log_name(self, source: str) -> str:
        return "pw-record.log" if source == MIC else f"pw-record.{source}.log"

    def start(self, source: str, audio_path: Path, stderr_path: Path) -> subprocess.Popen:
        if source not in self.supported_sources:
            raise ValueError(f"{self.name} cannot record source {source!r}")
        command = ["pw-record", "--rate", "16000", "--channels", "1", "--format", "s16"]
        if source == SYSTEM:
            # stream.capture.sink binds to the default sink's monitor rather than the
            # default source; with no --target it follows sink changes like the mic does
            command += ["-P", "{ stream.capture.sink=true }"]
        command.append(str(audio_path))
        with stderr_path.open("ab") as stderr:
            return subprocess.Popen(
                command,
                stdout=subprocess.DEVNULL,
                stderr=stderr,
                start_new_session=True,
            )

    def _signal(self, pid: int, number: int) -> None:
        try:
            os.killpg(pid, number)
        except ProcessLookupError:
            pass
        except Exception:
            os.kill(pid, number)

    def stop_all(self, pids: Iterable[int]) -> None:
        alive = [pid for pid in pids if pid_alive(pid)]
        for pid in alive:
            self._signal(pid, signal.SIGINT)
        for pid in alive:
            wait_until(lambda: not pid_alive(pid), timeout_seconds=4)
        stubborn = [pid for pid in alive if pid_alive(pid)]
        for pid in stubborn:
            self._signal(pid, signal.SIGTERM)
        for pid in stubborn:
            wait_until(lambda: not pid_alive(pid), timeout_seconds=2)


def has_audio(path: Path) -> bool:
    try:
        return path.stat().st_size > WAV_HEADER_BYTES
    except OSError:
        return False


def mixer_available() -> bool:
    return command_exists("ffmpeg")


def mix_sources(parts: list[Path], output: Path) -> list[Path]:
    """Combine per-source captures into one mono file, returning the parts used.

    Parts holding nothing but a WAV header are dropped, so a source that failed
    to capture leaves the rest of the recording usable.
    """
    usable = [path for path in parts if has_audio(path)]
    if not usable:
        raise RuntimeError("no source captured any audio")
    if len(usable) == 1:
        usable[0].replace(output)
        return usable

    inputs: list[str] = []
    for path in usable:
        inputs += ["-i", str(path)]
    subprocess.run(
        [
            "ffmpeg",
            "-hide_banner",
            "-loglevel",
            "error",
            "-y",
            *inputs,
            # normalize keeps both sides talking at once from clipping the sum
            "-filter_complex",
            f"amix=inputs={len(usable)}:duration=longest:normalize=1",
            "-ar",
            "16000",
            "-ac",
            "1",
            "-c:a",
            "pcm_s16le",
            str(output),
        ],
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return usable


def default_recorder_backend() -> RecorderBackend:
    return PipeWireRecorderBackend()
