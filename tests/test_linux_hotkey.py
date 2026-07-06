from __future__ import annotations

import os
import tempfile
import threading
import unittest
from pathlib import Path

from risper.platforms.linux_hotkey import (
    EV_KEY,
    INPUT_EVENT,
    KEY_PRESS,
    KEY_RELEASE,
    LinuxDoubleAltListener,
)


KEY_LEFTALT = 56


class ListenerLifecycleTests(unittest.TestCase):
    def _fifo_device(self, tempdir: str) -> tuple[Path, int]:
        path = Path(tempdir) / "event0"
        os.mkfifo(path)
        # keep a writer open so the listener's read-only open doesn't block
        writer = os.open(path, os.O_RDWR)
        return path, writer

    def test_stop_ends_the_reader_thread_even_when_the_device_is_quiet(self) -> None:
        with tempfile.TemporaryDirectory() as tempdir:
            path, writer = self._fifo_device(tempdir)
            try:
                listener = LinuxDoubleAltListener(350, lambda: None, device_paths=[path])
                ok, _message = listener.start()
                self.assertTrue(ok)
                thread = listener._thread
                self.assertIsNotNone(thread)
                self.assertTrue(thread.is_alive())

                listener.stop()

                self.assertFalse(thread.is_alive())
                self.assertIsNone(listener._thread)
                self.assertEqual(listener._files, [])
            finally:
                os.close(writer)

    def test_double_alt_from_device_events_triggers(self) -> None:
        with tempfile.TemporaryDirectory() as tempdir:
            path, writer = self._fifo_device(tempdir)
            triggered = threading.Event()
            listener = LinuxDoubleAltListener(350, triggered.set, device_paths=[path])
            try:
                ok, _message = listener.start()
                self.assertTrue(ok)

                for value in (KEY_PRESS, KEY_RELEASE, KEY_PRESS, KEY_RELEASE):
                    os.write(writer, INPUT_EVENT.pack(0, 0, EV_KEY, KEY_LEFTALT, value))

                self.assertTrue(triggered.wait(timeout=5.0))
            finally:
                listener.stop()
                os.close(writer)

    def test_start_reports_unreadable_devices(self) -> None:
        missing = Path(tempfile.gettempdir()) / "risper-no-such-device"

        listener = LinuxDoubleAltListener(350, lambda: None, device_paths=[missing])
        ok, message = listener.start()

        self.assertFalse(ok)
        self.assertIn("unavailable", message)


if __name__ == "__main__":
    unittest.main()
