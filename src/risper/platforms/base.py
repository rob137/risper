from __future__ import annotations

from pathlib import Path


class DesktopPlatform:
    name = "unknown"

    def copy_text(self, text: str) -> tuple[bool, str]:
        return False, f"{self.name}: clipboard is not implemented"

    def attempt_paste(self, mode: str) -> tuple[bool, str]:
        return False, f"{self.name}: paste is not implemented"

    def notify(self, title: str, body: str = "") -> None:
        return None

    def play_sound(self, kind: str) -> None:
        return None

    def open_path(self, path: Path) -> tuple[bool, str]:
        return False, f"{self.name}: open_path is not implemented"

    def trash_path(self, path: Path) -> tuple[bool, str]:
        return False, f"{self.name}: trash_path is not implemented"

    def session_type(self) -> str:
        return self.name

    def diagnostic_commands(self) -> list[str]:
        return []
