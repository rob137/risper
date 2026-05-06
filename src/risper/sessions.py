from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from .config import Config
from .platforms import current_platform
from .util import atomic_write_json, session_id_from_now, utc_now_iso


def create_session(config: Config) -> dict[str, Any]:
    session_id = session_id_from_now()
    session_dir = config.sessions_dir / session_id
    counter = 1
    while session_dir.exists():
        counter += 1
        session_dir = config.sessions_dir / f"{session_id}-{counter}"
    session_dir.mkdir(parents=True)

    metadata = {
        "session_id": session_dir.name,
        "started_at": utc_now_iso(),
        "ended_at": None,
        "duration_seconds": None,
        "status": "recording",
        "audio_path": str(session_dir / "audio.wav"),
        "transcript_raw_path": str(session_dir / "transcript.raw.txt"),
        "transcript_clean_path": str(session_dir / "transcript.clean.txt"),
        "transcription_engine": config.transcription_engine,
        "model": config.model,
        "language": config.language,
        "paste_attempted": False,
        "paste_succeeded": False,
        "session_type": current_platform().session_type(),
        "target_app": None,
        "errors": [],
    }
    atomic_write_json(session_dir / "metadata.json", metadata)
    (session_dir / "status.log").write_text(f"{utc_now_iso()} session created\n", encoding="utf-8")
    (session_dir / "error.log").touch()
    return metadata


def session_dir(metadata: dict[str, Any]) -> Path:
    return Path(str(metadata["audio_path"])).parent


def metadata_path(metadata: dict[str, Any]) -> Path:
    return session_dir(metadata) / "metadata.json"


def update_metadata(metadata: dict[str, Any], **updates: Any) -> dict[str, Any]:
    metadata.update(updates)
    atomic_write_json(metadata_path(metadata), metadata)
    return metadata


def load_session(path: Path) -> dict[str, Any] | None:
    metadata_file = path / "metadata.json"
    if not metadata_file.exists():
        return None
    try:
        return json.loads(metadata_file.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return None


def all_sessions(config: Config) -> list[dict[str, Any]]:
    sessions: list[dict[str, Any]] = []
    if not config.sessions_dir.exists():
        return sessions
    for path in config.sessions_dir.iterdir():
        if path.is_dir():
            metadata = load_session(path)
            if metadata:
                sessions.append(metadata)
    return sorted(sessions, key=lambda item: str(item.get("started_at", "")), reverse=True)


def last_session(config: Config) -> dict[str, Any] | None:
    sessions = all_sessions(config)
    return sessions[0] if sessions else None


def find_session(config: Config, session_id: str) -> dict[str, Any] | None:
    if session_id == "last":
        return last_session(config)
    for metadata in all_sessions(config):
        if metadata.get("session_id") == session_id:
            return metadata
    return None


def mark_incomplete_recordings_recovered(config: Config) -> int:
    count = 0
    for metadata in all_sessions(config):
        if metadata.get("status") == "recording":
            errors = list(metadata.get("errors", []))
            errors.append("Recovered incomplete recording after startup; audio may be partial.")
            update_metadata(
                metadata,
                status="recovered",
                ended_at=metadata.get("ended_at") or utc_now_iso(),
                errors=errors,
            )
            count += 1
    return count
