from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path
from typing import Any

from .base import DesktopPlatform


class LinuxDesktopPlatform(DesktopPlatform):
    name = "linux"

    def _command_exists(self, name: str) -> bool:
        return self._command_path(name) is not None

    def _command_path(self, name: str) -> str | None:
        found = shutil.which(name)
        if found:
            return found
        for directory in ("/usr/bin", "/usr/local/bin", str(Path.home() / ".local" / "bin")):
            candidate = Path(directory) / name
            if candidate.exists() and os.access(candidate, os.X_OK):
                return str(candidate)
        return None

    def session_type(self) -> str:
        return os.environ.get("XDG_SESSION_TYPE", "unknown")

    def copy_text(self, text: str) -> tuple[bool, str]:
        session_type = self.session_type().lower()
        candidates: list[list[str]] = []
        if session_type == "wayland":
            candidates.append(["wl-copy"])
        candidates.extend([["xclip", "-selection", "clipboard"], ["xsel", "--clipboard", "--input"]])

        last_error = "no clipboard command available"
        for command in candidates:
            command_path = self._command_path(command[0])
            if not command_path:
                continue
            try:
                subprocess.run([command_path, *command[1:]], input=text, text=True, check=True, timeout=5)
                return True, f"copied with {command[0]}"
            except Exception as exc:
                last_error = f"{command[0]} failed: {exc}"
        return False, last_error

    def _paste_candidates(self, mode: str) -> list[str]:
        if mode == "clipboard_only":
            return []
        if mode != "auto":
            return [mode]
        session_type = self.session_type().lower()
        if session_type == "x11":
            return ["xdotool", "dotool", "ydotool"]
        if session_type == "wayland":
            return ["wtype", "dotool", "ydotool"]
        return ["dotool", "ydotool", "wtype"]

    def _run_checked(self, command: list[str], **kwargs: Any) -> None:
        kwargs.setdefault("text", True)
        subprocess.run(
            command,
            check=True,
            timeout=5,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            **kwargs,
        )

    def _command_failure(self, exc: Exception) -> str:
        if isinstance(exc, subprocess.CalledProcessError):
            output = str(exc.stdout or exc.output or "").strip()
            if output:
                return output
        return str(exc)

    def attempt_paste(self, mode: str) -> tuple[bool, str]:
        if mode == "clipboard_only":
            return False, "paste disabled by paste_mode=clipboard_only"

        candidates = self._paste_candidates(mode)
        if not candidates:
            return False, "paste disabled by paste_mode=clipboard_only"
        session_type = self.session_type().lower()

        commands = {
            "xdotool": ["xdotool", "key", "--clearmodifiers", "ctrl+v"],
            "wtype": ["wtype", "-M", "ctrl", "-k", "v", "-m", "ctrl"],
            "dotool": ["dotool"],
            "ydotool": ["ydotool", "key", "29:1", "47:1", "47:0", "29:0"],
        }
        missing: list[str] = []
        attempted: list[str] = []
        last_error = "no safe paste helper available; transcript remains on clipboard"
        for selected in candidates:
            command = commands.get(selected)
            if not command:
                last_error = f"{selected} is not supported on {session_type}"
                continue
            command_path = self._command_path(command[0])
            if not command_path:
                missing.append(selected)
                if not attempted:
                    last_error = f"{selected} is not installed"
                continue
            command = [command_path, *command[1:]]
            attempted.append(selected)

            try:
                if selected == "dotool":
                    self._run_checked(command, input="key ctrl+v\n", text=True)
                else:
                    self._run_checked(command)
                return True, f"paste attempted with {selected}"
            except Exception as exc:
                last_error = f"{selected} paste failed: {self._command_failure(exc)}"
                if mode != "auto":
                    return False, last_error

        if mode == "auto" and missing and not attempted:
            return False, f"no installed paste helper from: {', '.join(candidates)}"
        return False, last_error

    def notify(self, title: str, body: str = "") -> None:
        if not title or not self._command_exists("notify-send"):
            return
        subprocess.Popen(
            ["notify-send", title, body],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )

    def play_sound(self, kind: str) -> None:
        if not self._command_exists("canberra-gtk-play"):
            return
        event = {
            "start": "message-new-instant",
            "stop": "complete",
            "error": "dialog-error",
        }.get(kind, "message")
        subprocess.Popen(
            ["canberra-gtk-play", "-i", event, "-V", "-18", "-d", f"Risper {kind}"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )

    def open_path(self, path: Path) -> tuple[bool, str]:
        if not path.exists():
            return False, f"not found: {path}"
        if not self._command_exists("gio"):
            return False, "gio is not installed"
        subprocess.Popen(["gio", "open", str(path)], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        return True, f"opened {path}"

    def trash_path(self, path: Path) -> tuple[bool, str]:
        if not path.exists():
            return False, f"not found: {path}"
        if self._command_exists("gio"):
            subprocess.run(["gio", "trash", str(path)], check=True)
            return True, f"moved to trash: {path}"
        return False, "gio is not installed"

    def diagnostic_commands(self) -> list[str]:
        return [
            "pw-record",
            "arecord",
            "ffmpeg",
            "wl-copy",
            "wtype",
            "xclip",
            "xsel",
            "xdotool",
            "ydotool",
            "dotool",
            "notify-send",
            "paplay",
            "canberra-gtk-play",
            "gio",
            "gtk-launch",
            "python3",
            "pipx",
            "pip",
        ]
