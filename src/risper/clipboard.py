from __future__ import annotations

from .platforms import current_platform


def copy_text(text: str) -> tuple[bool, str]:
    return current_platform().copy_text(text)
