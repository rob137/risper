from __future__ import annotations

import contextlib
import io
import unittest
from unittest.mock import patch

from risper.cli import main


class CliTests(unittest.TestCase):
    def test_default_starts_and_enables_daemon(self) -> None:
        with patch("risper.cli.subprocess.run") as run:
            run.return_value.returncode = 0
            output = io.StringIO()

            with contextlib.redirect_stdout(output):
                code = main([])

        self.assertEqual(code, 0)
        run.assert_called_once_with(
            ["systemctl", "--user", "enable", "--now", "risper.service"],
            check=False,
        )
        self.assertIn("enabled and running", output.getvalue())

    def test_kill_stops_daemon_without_disabling_autostart(self) -> None:
        with patch("risper.cli.subprocess.run") as run:
            run.return_value.returncode = 0
            output = io.StringIO()

            with contextlib.redirect_stdout(output):
                code = main(["kill"])

        self.assertEqual(code, 0)
        run.assert_called_once_with(
            ["systemctl", "--user", "stop", "risper.service"],
            check=False,
        )
        self.assertIn("Autostart remains enabled", output.getvalue())


if __name__ == "__main__":
    unittest.main()
