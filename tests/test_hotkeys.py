from __future__ import annotations

import unittest

from risper.hotkeys import DoubleAltDetector


class DoubleAltDetectorTests(unittest.TestCase):
    def test_double_alt_within_window_triggers(self) -> None:
        detector = DoubleAltDetector(window_ms=350)

        self.assertFalse(detector.handle_key(56, True, 0))
        self.assertFalse(detector.handle_key(56, False, 10))
        self.assertFalse(detector.handle_key(56, True, 100))
        self.assertTrue(detector.handle_key(56, False, 120))

    def test_double_alt_outside_window_does_not_trigger(self) -> None:
        detector = DoubleAltDetector(window_ms=350)

        detector.handle_key(56, True, 0)
        detector.handle_key(56, False, 10)
        detector.handle_key(56, True, 500)

        self.assertFalse(detector.handle_key(56, False, 520))

    def test_alt_combination_resets_sequence(self) -> None:
        detector = DoubleAltDetector(window_ms=350)

        detector.handle_key(56, True, 0)
        detector.handle_key(15, True, 10)
        detector.handle_key(15, False, 20)
        self.assertFalse(detector.handle_key(56, False, 30))
        detector.handle_key(56, True, 100)
        self.assertFalse(detector.handle_key(56, False, 120))

    def test_right_alt_counts_as_alt_tap(self) -> None:
        detector = DoubleAltDetector(window_ms=350)

        detector.handle_key(100, True, 0)
        detector.handle_key(100, False, 10)
        detector.handle_key(100, True, 100)

        self.assertTrue(detector.handle_key(100, False, 120))


if __name__ == "__main__":
    unittest.main()
