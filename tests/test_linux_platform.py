from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from unittest.mock import call, patch

from risper.platforms.linux import LinuxDesktopPlatform


class LinuxPasteTests(unittest.TestCase):
    def test_wayland_auto_uses_wtype_when_available(self) -> None:
        platform = LinuxDesktopPlatform()
        commands: list[list[str]] = []

        with (
            patch.object(platform, "session_type", return_value="wayland"),
            patch.object(platform, "_command_exists", side_effect=lambda name: name == "wtype"),
            patch.object(platform, "_command_path", side_effect=lambda name: name if name == "wtype" else None),
            patch.object(platform, "_run_checked", side_effect=lambda command, **kwargs: commands.append(command)),
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
            patch.object(platform, "_run_checked", side_effect=fake_run),
        ):
            self.assertEqual(platform.attempt_paste("auto"), (True, "paste attempted with dotool"))

        self.assertEqual(attempted, ["wtype", "dotool"])

    def test_manual_paste_mode_reports_missing_helper(self) -> None:
        platform = LinuxDesktopPlatform()

        with patch.object(platform, "_command_path", return_value=None):
            self.assertEqual(platform.attempt_paste("wtype"), (False, "wtype is not installed"))

    def test_auto_reports_installed_helper_failure_before_missing_fallbacks(self) -> None:
        platform = LinuxDesktopPlatform()

        with (
            patch.object(platform, "session_type", return_value="wayland"),
            patch.object(platform, "_command_path", side_effect=lambda name: name if name == "wtype" else None),
            patch.object(
                platform,
                "_run_checked",
                side_effect=subprocess.CalledProcessError(
                    1,
                    ["wtype"],
                    output="Compositor does not support the virtual keyboard protocol\n",
                ),
            ),
        ):
            ok, message = platform.attempt_paste("auto")

        self.assertFalse(ok)
        self.assertIn("wtype paste failed", message)
        self.assertIn("Compositor does not support", message)

    def test_wayland_auto_uses_documented_ydotool_key_sequence(self) -> None:
        platform = LinuxDesktopPlatform()
        commands: list[list[str]] = []

        with (
            patch.object(platform, "session_type", return_value="wayland"),
            patch.object(platform, "_command_path", side_effect=lambda name: name if name == "ydotool" else None),
            patch.object(platform, "_run_checked", side_effect=lambda command, **kwargs: commands.append(command)),
        ):
            self.assertEqual(platform.attempt_paste("auto"), (True, "paste attempted with ydotool"))

        self.assertEqual(commands, [["ydotool", "key", "ctrl+v"]])

    def test_command_path_falls_back_to_usr_bin_when_path_is_sparse(self) -> None:
        platform = LinuxDesktopPlatform()

        with (
            patch("risper.platforms.linux.shutil.which", return_value=None),
            patch("risper.platforms.linux.Path.exists", return_value=True),
            patch("risper.platforms.linux.os.access", return_value=True),
        ):
            self.assertEqual(platform._command_path("wtype"), "/usr/bin/wtype")

    def test_notify_replaces_previous_risper_notification(self) -> None:
        platform = LinuxDesktopPlatform()
        calls: list[list[str]] = []

        def fake_run(command, **_kwargs):
            calls.append(command)
            completed = subprocess.CompletedProcess(command, 0, stdout=f"{100 + len(calls)}\n")
            return completed

        with tempfile.TemporaryDirectory() as tempdir:
            old_state = os.environ.get("XDG_STATE_HOME")
            os.environ["XDG_STATE_HOME"] = tempdir
            try:
                with (
                    patch.object(platform, "_command_path", return_value="notify-send"),
                    patch("risper.platforms.linux.subprocess.run", side_effect=fake_run),
                ):
                    platform.notify("🎙 Risper listening", "Run risper-toggle again to stop.")
                    platform.notify("⏳ Risper transcribing", "Using whispercpp-base-en.")
            finally:
                if old_state is None:
                    os.environ.pop("XDG_STATE_HOME", None)
                else:
                    os.environ["XDG_STATE_HOME"] = old_state

        self.assertNotIn("--replace-id=101", calls[0])
        self.assertIn("--replace-id=101", calls[1])

    def test_play_sound_uses_distinct_events_for_workflow_outcomes(self) -> None:
        platform = LinuxDesktopPlatform()

        with (
            patch.object(platform, "_command_exists", return_value=True),
            patch("risper.platforms.linux.subprocess.Popen") as popen,
        ):
            for kind in ("recording_start", "transcription_start", "success", "cancel", "error"):
                platform.play_sound(kind)

        self.assertEqual(
            popen.call_args_list,
            [
                call(
                    ["canberra-gtk-play", "-i", "message-new-instant", "-V", "-18", "-d", "Risper recording_start"],
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.DEVNULL,
                ),
                call(
                    ["canberra-gtk-play", "-i", "service-login", "-V", "-18", "-d", "Risper transcription_start"],
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.DEVNULL,
                ),
                call(
                    ["canberra-gtk-play", "-i", "complete", "-V", "-18", "-d", "Risper success"],
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.DEVNULL,
                ),
                call(
                    ["canberra-gtk-play", "-i", "service-logout", "-V", "-18", "-d", "Risper cancel"],
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.DEVNULL,
                ),
                call(
                    ["canberra-gtk-play", "-i", "dialog-error", "-V", "-18", "-d", "Risper error"],
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.DEVNULL,
                ),
            ],
        )


if __name__ == "__main__":
    unittest.main()
