from __future__ import annotations

import contextlib
import io
import unittest
from unittest.mock import patch

from risper.paste_test import _run_manual, make_marker


class PasteTestTests(unittest.TestCase):
    def test_marker_has_expected_prefix(self) -> None:
        self.assertTrue(make_marker().startswith("RISPER_PASTE_TEST_"))

    def test_manual_mode_reports_failed_helper(self) -> None:
        output = io.StringIO()
        with (
            patch("risper.paste_test.time.sleep"),
            patch("risper.paste_test.make_marker", return_value="RISPER_PASTE_TEST_FIXED"),
            patch("risper.paste_test._attempt_marker_paste", return_value=(False, "wtype failed")),
            contextlib.redirect_stdout(output),
        ):
            code = _run_manual(0)

        self.assertEqual(code, 1)
        self.assertIn("RISPER_PASTE_TEST_FIXED", output.getvalue())
        self.assertIn("wtype failed", output.getvalue())


if __name__ == "__main__":
    unittest.main()
