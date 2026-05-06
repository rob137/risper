from __future__ import annotations

import unittest

from risper.status_window import format_duration, format_started


class StatusWindowHelperTests(unittest.TestCase):
    def test_format_duration_handles_missing_value(self) -> None:
        self.assertEqual(format_duration({}), "")

    def test_format_duration_adds_seconds_suffix(self) -> None:
        self.assertEqual(format_duration({"duration_seconds": 2.5}), "2.5s")

    def test_format_started_uses_session_id(self) -> None:
        self.assertEqual(format_started({"session_id": "2026-01-01_10-00-00"}), "2026-01-01_10-00-00")


if __name__ == "__main__":
    unittest.main()
