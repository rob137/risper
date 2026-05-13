from __future__ import annotations

import subprocess
from pathlib import Path

from .base import DesktopPlatform


class MacOSDesktopPlatform(DesktopPlatform):
    name = "macos"

    def copy_text(self, text: str) -> tuple[bool, str]:
        try:
            subprocess.run(["pbcopy"], input=text, text=True, check=True, timeout=5)
            return True, "copied with pbcopy"
        except Exception as exc:
            return False, f"pbcopy failed: {exc}"

    def attempt_paste(self, mode: str) -> tuple[bool, str]:
        if mode == "clipboard_only":
            return False, "paste disabled by paste_mode=clipboard_only"
        try:
            subprocess.run(
                ["osascript", "-e", 'tell application "System Events" to keystroke "v" using command down'],
                check=True,
                timeout=5,
            )
            return True, "paste attempted with osascript"
        except Exception as exc:
            return False, f"osascript paste failed: {exc}"

    def notify(self, title: str, body: str = "") -> None:
        script = f'display notification {body!r} with title {title!r}'
        subprocess.Popen(["osascript", "-e", script], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    def play_sound(self, kind: str) -> None:
        sound = {
            "recording_start": "Pop",
            "transcription_start": "Tink",
            "success": "Hero",
            "cancel": "Bottle",
            "error": "Glass",
            # Backwards-compatible aliases for older callers or scripts.
            "start": "Pop",
            "stop": "Hero",
        }.get(kind, "Pop")
        subprocess.Popen(["afplay", f"/System/Library/Sounds/{sound}.aiff"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    def open_path(self, path: Path) -> tuple[bool, str]:
        if not path.exists():
            return False, f"not found: {path}"
        subprocess.Popen(["open", str(path)], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        return True, f"opened {path}"

    def trash_path(self, path: Path) -> tuple[bool, str]:
        return False, "trash_path is not implemented on macOS yet"

    def diagnostic_commands(self) -> list[str]:
        return ["pbcopy", "osascript", "afplay", "open", "python3"]
