from __future__ import annotations

import unittest

from risper.daemon import _refresh_reason


class DaemonRefreshTests(unittest.TestCase):
    def test_refreshes_after_suspend_like_wall_clock_jump(self) -> None:
        reason = _refresh_reason(
            last_wall=100.0,
            last_mono=50.0,
            previous_devices=(),
            current_devices=(),
            now_wall=500.0,
            now_mono=51.0,
        )

        self.assertEqual(reason, "resume detected")

    def test_refreshes_when_input_device_change_holds_for_a_tick(self) -> None:
        changed = (("/dev/input/event0", 1, 1), ("/dev/input/event1", 2, 2))

        reason = _refresh_reason(
            last_wall=100.0,
            last_mono=50.0,
            previous_devices=(("/dev/input/event0", 1, 1),),
            current_devices=changed,
            pending_devices=changed,
            now_wall=101.0,
            now_mono=51.0,
        )

        self.assertEqual(reason, "input devices changed")

    def test_does_not_refresh_on_the_first_tick_of_a_device_change(self) -> None:
        reason = _refresh_reason(
            last_wall=100.0,
            last_mono=50.0,
            previous_devices=(("/dev/input/event0", 1, 1),),
            current_devices=(("/dev/input/event0", 1, 1), ("/dev/input/event1", 2, 2)),
            pending_devices=None,
            now_wall=101.0,
            now_mono=51.0,
        )

        self.assertIsNone(reason)

    def test_does_not_refresh_when_devices_are_still_enumerating(self) -> None:
        reason = _refresh_reason(
            last_wall=100.0,
            last_mono=50.0,
            previous_devices=(("/dev/input/event0", 1, 1),),
            current_devices=(("/dev/input/event0", 1, 1), ("/dev/input/event1", 2, 2)),
            pending_devices=(("/dev/input/event0", 1, 1), ("/dev/input/event2", 3, 3)),
            now_wall=101.0,
            now_mono=51.0,
        )

        self.assertIsNone(reason)

    def test_does_not_refresh_during_normal_sleep_loop(self) -> None:
        reason = _refresh_reason(
            last_wall=100.0,
            last_mono=50.0,
            previous_devices=(("/dev/input/event0", 1, 1),),
            current_devices=(("/dev/input/event0", 1, 1),),
            now_wall=101.0,
            now_mono=51.0,
        )

        self.assertIsNone(reason)


if __name__ == "__main__":
    unittest.main()
