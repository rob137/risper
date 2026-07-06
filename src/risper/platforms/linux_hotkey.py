from __future__ import annotations

import glob
import os
import selectors
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
SELECT_TIMEOUT_SECONDS = 0.5


class LinuxDoubleAltListener:
    """Reads every input device from a single selector-driven thread.

    One thread per listener, not per device: a per-device thread blocked in
    read() on a quiet device (lid switch, sleep button) never notices a closed
    handle, so it leaked on every resume/device-change restart of the daemon.
    Non-blocking reads plus a select timeout mean stop() always ends the thread.
    """

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
        self._thread: threading.Thread | None = None
        self._selector: selectors.BaseSelector | None = None
        self._files = []

    def start(self) -> tuple[bool, str]:
        paths = self.device_paths or [Path(path) for path in sorted(glob.glob("/dev/input/event*"))]
        self._selector = selectors.DefaultSelector()
        failures: list[str] = []
        for path in paths:
            try:
                handle = path.open("rb", buffering=0)
                os.set_blocking(handle.fileno(), False)
            except OSError as exc:
                failures.append(f"{path.name}: {exc.strerror or exc}")
                continue
            self._files.append(handle)
            self._selector.register(handle, selectors.EVENT_READ)

        if self._files:
            self._thread = threading.Thread(target=self._read_loop, daemon=True)
            self._thread.start()
            return True, f"double-alt listener reading {len(self._files)} input device(s)"

        self._selector.close()
        self._selector = None
        detail = "; ".join(failures[:3]) if failures else "no /dev/input/event* devices found"
        return False, f"double-alt listener unavailable: {detail}"

    def stop(self) -> None:
        self._stop.set()
        if self._thread:
            self._thread.join(timeout=SELECT_TIMEOUT_SECONDS * 4)
            self._thread = None
        if self._selector:
            self._selector.close()
            self._selector = None
        for handle in self._files:
            try:
                handle.close()
            except OSError:
                pass
        self._files = []

    def _read_loop(self) -> None:
        while not self._stop.is_set():
            for key, _ in self._selector.select(timeout=SELECT_TIMEOUT_SECONDS):
                self._drain(key.fileobj)

    def _drain(self, handle) -> None:
        while not self._stop.is_set():
            try:
                data = handle.read(INPUT_EVENT.size)
            except OSError:
                self._selector.unregister(handle)
                return
            if data is None:
                return  # non-blocking read, nothing buffered
            if len(data) != INPUT_EVENT.size:
                self._selector.unregister(handle)
                return  # EOF or short read: device went away
            _sec, _usec, event_type, code, value = INPUT_EVENT.unpack(data)
            if event_type != EV_KEY or value not in {KEY_PRESS, KEY_RELEASE}:
                continue
            triggered = self.detector.handle_key(
                code,
                pressed=value == KEY_PRESS,
                timestamp_ms=time.monotonic() * 1000,
            )
            if triggered:
                self.on_trigger()


def has_input_devices() -> bool:
    return any(os.access(path, os.R_OK) for path in glob.glob("/dev/input/event*"))
