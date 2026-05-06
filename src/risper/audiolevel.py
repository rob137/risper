from __future__ import annotations

import math
import shutil
import struct
import subprocess
import threading
import time


def pcm16_rms_level(data: bytes) -> float:
    """Return a 0..1-ish RMS level for signed 16-bit little-endian PCM."""
    sample_count = len(data) // 2
    if sample_count == 0:
        return 0.0
    samples = struct.unpack(f"<{sample_count}h", data[: sample_count * 2])
    square_sum = sum(sample * sample for sample in samples)
    rms = math.sqrt(square_sum / sample_count)
    return min(1.0, rms / 32768.0)


def level_to_bars(level: float, width: int = 12) -> str:
    blocks = "▁▂▃▄▅▆▇█"
    level = max(0.0, min(1.0, level))
    active = round(level * width)
    if active <= 0:
        return blocks[0] * width
    high = blocks[min(len(blocks) - 1, max(0, round(level * (len(blocks) - 1))))]
    return high * active + blocks[0] * (width - active)


class MicLevelMonitor:
    def __init__(self, rate: int = 16000, chunk_bytes: int = 4096) -> None:
        self.rate = rate
        self.chunk_bytes = chunk_bytes
        self.level = 0.0
        self.available = shutil.which("pw-cat") is not None
        self._stop = threading.Event()
        self._thread: threading.Thread | None = None
        self._proc: subprocess.Popen | None = None

    def start(self) -> None:
        if not self.available or self._thread:
            return
        self._thread = threading.Thread(target=self._run, name="risper-mic-level", daemon=True)
        self._thread.start()

    def stop(self) -> None:
        self._stop.set()
        proc = self._proc
        if proc and proc.poll() is None:
            proc.terminate()
        if self._thread:
            self._thread.join(timeout=1)

    def _run(self) -> None:
        try:
            self._proc = subprocess.Popen(
                [
                    "pw-cat",
                    "--record",
                    "--rate",
                    str(self.rate),
                    "--channels",
                    "1",
                    "--format",
                    "s16",
                    "-",
                ],
                stdout=subprocess.PIPE,
                stderr=subprocess.DEVNULL,
            )
        except Exception:
            self.available = False
            return

        assert self._proc.stdout is not None
        while not self._stop.is_set() and self._proc.poll() is None:
            data = self._proc.stdout.read(self.chunk_bytes)
            if not data:
                time.sleep(0.05)
                continue
            current = pcm16_rms_level(data)
            self.level = max(current, self.level * 0.72)

        if self._proc.poll() is None:
            self._proc.terminate()
