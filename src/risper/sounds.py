from __future__ import annotations

from .config import Config
from .platforms import current_platform


def play(config: Config, kind: str) -> None:
    if not config.play_sounds:
        return
    current_platform().play_sound(kind)
