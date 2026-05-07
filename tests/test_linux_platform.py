from __future__ import annotations

import subprocess
import unittest
from unittest.mock import patch

from risper.platforms.linux import LinuxDesktopPlatform


class LinuxPasteTests(unittest.TestCase):
    def test_wayland_auto_uses_wtype_when_available(self) -> None:
        platform = LinuxDesktopPlatform()
        commands: list[list[str]] = []

        with (
            patch.object(platform, "session_type", return_value="wayland"),
            patch.object(platform, "_command_exists", side_effect=lambda name: name == "wtype"),
            patch.object(platform, "_command_path", side_effect=lambda name: name if name == "wtype" else None),
            patch("risper.platforms.linux.subprocess.run", side_effect=lambda command, **kwargs: commands.append(command)),
        ):
            self.assertEqual(platform.attempt_paste("auto"), (True, "paste attempted with wtype"))

        self.assertEqual(commands, [["wtype", "-M", "ctrl", "-k", "v", "-m", "ctrl"]])

    def test_wayland_auto_falls_through_failed_wtype_to_dotool(self) -> None:
        platform = LinuxDesktopPlatform()
        attempted: list[str] = []

        def fake_run(command, **kwargs):
            attempted.append(command[0])
            if command[0] == "wtype":
                raise subprocess.CalledProcessError(1, command)

        with (
            patch.object(platform, "session_type", return_value="wayland"),
            patch.object(platform, "_command_path", side_effect=lambda name: name if name in {"wtype", "dotool"} else None),
            patch("risper.platforms.linux.subprocess.run", side_effect=fake_run),
        ):
            self.assertEqual(platform.attempt_paste("auto"), (True, "paste attempted with dotool"))

        self.assertEqual(attempted, ["wtype", "dotool"])

    def test_manual_paste_mode_reports_missing_helper(self) -> None:
        platform = LinuxDesktopPlatform()

        with patch.object(platform, "_command_path", return_value=None):
            self.assertEqual(platform.attempt_paste("wtype"), (False, "wtype is not installed"))

    def test_command_path_falls_back_to_usr_bin_when_path_is_sparse(self) -> None:
        platform = LinuxDesktopPlatform()

        with (
            patch("risper.platforms.linux.shutil.which", return_value=None),
            patch("risper.platforms.linux.Path.exists", return_value=True),
            patch("risper.platforms.linux.os.access", return_value=True),
        ):
            self.assertEqual(platform._command_path("wtype"), "/usr/bin/wtype")


if __name__ == "__main__":
    unittest.main()
