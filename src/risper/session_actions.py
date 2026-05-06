from __future__ import annotations

from pathlib import Path

from .clipboard import copy_text
from .config import Config
from .platforms import current_platform
from .sessions import find_session


def transcript_path(metadata: dict) -> Path | None:
    for key in ("transcript_clean_path", "transcript_raw_path"):
        value = str(metadata.get(key, ""))
        if not value:
            continue
        path = Path(value)
        if path.exists():
            return path
    return None


def transcript_preview(metadata: dict, limit: int = 76) -> str:
    path = transcript_path(metadata)
    if path:
        text = " ".join(path.read_text(encoding="utf-8").split())
        return text[:limit]
    errors = metadata.get("errors") or []
    return str(errors[-1])[:limit] if errors else ""


def open_session(metadata: dict) -> tuple[bool, str]:
    return current_platform().open_path(Path(str(metadata["audio_path"])).parent)


def play_audio(metadata: dict) -> tuple[bool, str]:
    audio_path = Path(str(metadata["audio_path"]))
    if not audio_path.exists():
        return False, f"Audio missing: {audio_path}"
    return current_platform().open_path(audio_path)


def copy_transcript(metadata: dict) -> tuple[bool, str]:
    path = transcript_path(metadata)
    if not path:
        return False, "Session has no transcript."
    return copy_text(path.read_text(encoding="utf-8"))


def find_session_or_error(config: Config, session_id: str) -> tuple[dict | None, str]:
    metadata = find_session(config, session_id)
    if not metadata:
        return None, f"No such session: {session_id}"
    return metadata, ""
