from __future__ import annotations

import os
import subprocess
from pathlib import Path

from .base import DesktopPlatform


class WindowsDesktopPlatform(DesktopPlatform):
    name = "windows"

    def copy_text(self, text: str) -> tuple[bool, str]:
        try:
            subprocess.run(["clip"], input=text, text=True, check=True, timeout=5)
            return True, "copied with clip"
        except Exception as exc:
            return False, f"clip failed: {exc}"

    def attempt_paste(self, mode: str) -> tuple[bool, str]:
        return False, "paste is not implemented on Windows yet"

    def open_path(self, path: Path) -> tuple[bool, str]:
        if not path.exists():
            return False, f"not found: {path}"
        os.startfile(path)  # type: ignore[attr-defined]
        return True, f"opened {path}"

    def trash_path(self, path: Path) -> tuple[bool, str]:
        return False, "trash_path is not implemented on Windows yet"

    def diagnostic_commands(self) -> list[str]:
        return ["clip", "python"]
