from __future__ import annotations

import struct
import unittest

from risper.audiolevel import level_to_bars, pcm16_rms_level
from risper.overlay import _activity_line, _status_text, _visible_status


class AudioLevelTests(unittest.TestCase):
    def test_silence_has_zero_level(self) -> None:
        self.assertEqual(pcm16_rms_level(b"\x00\x00" * 100), 0.0)

    def test_loud_pcm_has_higher_level_than_quiet_pcm(self) -> None:
        quiet = struct.pack("<100h", *([1000] * 100))
        loud = struct.pack("<100h", *([12000] * 100))

        self.assertGreater(pcm16_rms_level(loud), pcm16_rms_level(quiet))

    def test_maximum_pcm_clips_to_one(self) -> None:
        maximum = struct.pack("<100h", *([32767] * 100))

        self.assertAlmostEqual(pcm16_rms_level(maximum), 1.0, places=3)

    def test_odd_trailing_byte_is_ignored(self) -> None:
        one_sample = struct.pack("<h", 12000)

        self.assertEqual(pcm16_rms_level(one_sample + b"x"), pcm16_rms_level(one_sample))

    def test_level_to_bars_is_stable_width(self) -> None:
        self.assertEqual(len(level_to_bars(0.0, width=8)), 8)
        self.assertEqual(len(level_to_bars(1.0, width=8)), 8)
        self.assertNotEqual(level_to_bars(0.0, width=8), level_to_bars(1.0, width=8))

    def test_level_to_bars_clamps_extreme_inputs(self) -> None:
        self.assertEqual(level_to_bars(-1.0, width=4), level_to_bars(0.0, width=4))
        self.assertEqual(level_to_bars(2.0, width=4), level_to_bars(1.0, width=4))


class OverlayStatusTests(unittest.TestCase):
    def test_user_facing_statuses(self) -> None:
        self.assertIn("Listening", _status_text("recording"))
        self.assertIn("Transcribing", _status_text("transcribing"))
        self.assertIn("Pasting", _status_text("pasting"))
        self.assertIn("Copied", _status_text("complete"))
        self.assertIn("paste unavailable", _status_text("paste_failed"))
        self.assertIn("Failed", _status_text("failed"))

    def test_busy_statuses_have_activity_line(self) -> None:
        self.assertIn("working on transcript", _activity_line("transcribing", 1, 1))
        self.assertIn("attempting paste", _activity_line("pasting", 2, 2))
        self.assertEqual(_activity_line("complete", 3), "3s")

    def test_metadata_status_wins_after_recording_stops(self) -> None:
        self.assertEqual(_visible_status("recording", True), "recording")
        self.assertEqual(_visible_status("transcribing", True), "transcribing")
        self.assertEqual(_visible_status("pasting", True), "pasting")


if __name__ == "__main__":
    unittest.main()
