from __future__ import annotations

import glob
import os
import struct
import threading
import time
from collections.abc import Callable
from pathlib import Path

from ..hotkeys import DoubleAltDetector


EV_KEY = 1
KEY_RELEASE = 0
KEY_PRESS = 1
INPUT_EVENT = struct.Struct("llHHI")


class LinuxDoubleAltListener:
    def __init__(
        self,
        window_ms: int,
        on_trigger: Callable[[], None],
        device_paths: list[Path] | None = None,
    ) -> None:
        self.detector = DoubleAltDetector(window_ms=window_ms)
        self.on_trigger = on_trigger
        self.device_paths = device_paths
        self._stop = threading.Event()
        self._lock = threading.Lock()
        self._threads: list[threading.Thread] = []
        self._files = []

    def start(self) -> tuple[bool, str]:
        paths = self.device_paths or [Path(path) for path in sorted(glob.glob("/dev/input/event*"))]
        opened = 0
        failures: list[str] = []
        for path in paths:
            try:
                handle = path.open("rb", buffering=0)
            except OSError as exc:
                failures.append(f"{path.name}: {exc.strerror or exc}")
                continue
            self._files.append(handle)
            thread = threading.Thread(target=self._read_loop, args=(handle,), daemon=True)
            thread.start()
            self._threads.append(thread)
            opened += 1

        if opened:
            return True, f"double-alt listener reading {opened} input device(s)"
        detail = "; ".join(failures[:3]) if failures else "no /dev/input/event* devices found"
        return False, f"double-alt listener unavailable: {detail}"

    def stop(self) -> None:
        self._stop.set()
        for handle in self._files:
            try:
                handle.close()
            except OSError:
                pass

    def _read_loop(self, handle) -> None:
        while not self._stop.is_set():
            try:
                data = handle.read(INPUT_EVENT.size)
            except OSError:
                return
            if not data or len(data) != INPUT_EVENT.size:
                return
            _sec, _usec, event_type, code, value = INPUT_EVENT.unpack(data)
            if event_type != EV_KEY or value not in {KEY_PRESS, KEY_RELEASE}:
                continue
            with self._lock:
                triggered = self.detector.handle_key(
                    code,
                    pressed=value == KEY_PRESS,
                    timestamp_ms=time.monotonic() * 1000,
                )
            if triggered:
                self.on_trigger()


def has_input_devices() -> bool:
    return any(os.access(path, os.R_OK) for path in glob.glob("/dev/input/event*"))
