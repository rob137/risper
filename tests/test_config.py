from __future__ import annotations

import unittest

from risper.config import parse_audio_retention


class AudioRetentionParsingTests(unittest.TestCase):
    def test_units_are_converted_to_seconds(self) -> None:
        self.assertEqual(parse_audio_retention("12h"), 12 * 3600)
        self.assertEqual(parse_audio_retention("7d"), 7 * 86400)
        self.assertEqual(parse_audio_retention("2w"), 2 * 604800)
        self.assertEqual(parse_audio_retention("0.5d"), 43200)

    def test_surrounding_whitespace_and_case_are_ignored(self) -> None:
        self.assertEqual(parse_audio_retention("  7D  "), 7 * 86400)

    def test_unusable_values_keep_audio_forever(self) -> None:
        for value in ("never", "", "7", "d", "soon", "-3d", "0d", "7 days"):
            with self.subTest(value=value):
                self.assertIsNone(parse_audio_retention(value))


if __name__ == "__main__":
    unittest.main()
