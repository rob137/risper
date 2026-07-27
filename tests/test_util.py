from __future__ import annotations

import time
import unittest
from unittest.mock import patch

from risper.util import notify_heartbeat


class NotifyHeartbeatTests(unittest.TestCase):
    def test_beats_with_elapsed_time_while_block_runs(self) -> None:
        on_beat_calls: list[None] = []
        with patch("risper.util.notify") as notify:
            with notify_heartbeat(
                "⏳ Title",
                "Body.",
                interval=0.05,
                on_beat=lambda: on_beat_calls.append(None),
            ):
                time.sleep(0.2)
        self.assertGreaterEqual(notify.call_count, 2)
        self.assertEqual(len(on_beat_calls), notify.call_count)
        title, body = notify.call_args[0]
        self.assertEqual(title, "⏳ Title")
        self.assertRegex(body, r"^Body\. \d+s elapsed\.$")

    def test_no_beats_after_block_exits(self) -> None:
        with patch("risper.util.notify") as notify:
            with notify_heartbeat("⏳ Title", "Body.", interval=0.05):
                pass
            count_at_exit = notify.call_count
            time.sleep(0.15)
        self.assertEqual(notify.call_count, count_at_exit)

    def test_stops_on_exception(self) -> None:
        with patch("risper.util.notify") as notify:
            with self.assertRaises(RuntimeError):
                with notify_heartbeat("⏳ Title", "Body.", interval=0.05):
                    raise RuntimeError("boom")
            count_at_exit = notify.call_count
            time.sleep(0.15)
        self.assertEqual(notify.call_count, count_at_exit)


if __name__ == "__main__":
    unittest.main()
