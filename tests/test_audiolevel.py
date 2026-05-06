from __future__ import annotations

import struct
import unittest

from risper.audiolevel import level_to_bars, pcm16_rms_level
from risper.overlay import _activity_line, _status_text


class AudioLevelTests(unittest.TestCase):
    def test_silence_has_zero_level(self) -> None:
        self.assertEqual(pcm16_rms_level(b"\x00\x00" * 100), 0.0)

    def test_loud_pcm_has_higher_level_than_quiet_pcm(self) -> None:
        quiet = struct.pack("<100h", *([1000] * 100))
        loud = struct.pack("<100h", *([12000] * 100))

        self.assertGreater(pcm16_rms_level(loud), pcm16_rms_level(quiet))

    def test_level_to_bars_is_stable_width(self) -> None:
        self.assertEqual(len(level_to_bars(0.0, width=8)), 8)
        self.assertEqual(len(level_to_bars(1.0, width=8)), 8)
        self.assertNotEqual(level_to_bars(0.0, width=8), level_to_bars(1.0, width=8))


class OverlayStatusTests(unittest.TestCase):
    def test_user_facing_statuses(self) -> None:
        self.assertIn("Listening", _status_text("recording"))
        self.assertIn("Transcribing", _status_text("transcribing"))
        self.assertIn("Pasting", _status_text("pasting"))
        self.assertIn("Copied", _status_text("complete"))
        self.assertIn("paste unavailable", _status_text("paste_failed"))
        self.assertIn("Failed", _status_text("failed"))

    def test_busy_statuses_have_activity_line(self) -> None:
        self.assertIn("working on transcript", _activity_line("transcribing", 1))
        self.assertIn("attempting paste", _activity_line("pasting", 2))
        self.assertEqual(_activity_line("complete", 3), "3s")


if __name__ == "__main__":
    unittest.main()
