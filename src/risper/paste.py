from __future__ import annotations

from .config import Config
from .platforms import current_platform


def attempt_paste(config: Config) -> tuple[bool, str]:
    return current_platform().attempt_paste(config.paste_mode)
