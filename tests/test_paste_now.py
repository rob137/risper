from __future__ import annotations

import contextlib
import io
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from helpers import write_test_config
from risper.paste_now import main


class FakePlatform:
    def attempt_paste(self, mode: str) -> tuple[bool, str]:
        raise AssertionError(mode)


class PasteNowTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.old_env = {
            key: os.environ.get(key)
            for key in ("XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME")
        }
        os.environ["XDG_CONFIG_HOME"] = str(self.root / "config")
        os.environ["XDG_DATA_HOME"] = str(self.root / "data")
        os.environ["XDG_STATE_HOME"] = str(self.root / "state")
        write_test_config(self.root)

    def tearDown(self) -> None:
        for key, value in self.old_env.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value
        self.tempdir.cleanup()

    def test_clipboard_only_config_uses_auto_for_manual_paste_command(self) -> None:
        platform = FakePlatform()
        with (
            patch("risper.paste_now.current_platform", return_value=platform),
            patch.object(platform, "attempt_paste", return_value=(True, "paste attempted with ydotool")) as paste,
        ):
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                code = main([])

        self.assertEqual(code, 0)
        paste.assert_called_once_with("auto")
        self.assertIn("paste attempted with ydotool", output.getvalue())

    def test_mode_argument_is_respected(self) -> None:
        platform = FakePlatform()
        with (
            patch("risper.paste_now.current_platform", return_value=platform),
            patch.object(platform, "attempt_paste", return_value=(False, "ydotool paste failed")) as paste,
        ):
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                code = main(["--mode", "ydotool"])

        self.assertEqual(code, 1)
        paste.assert_called_once_with("ydotool")


if __name__ == "__main__":
    unittest.main()
