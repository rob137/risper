from __future__ import annotations

import time
from datetime import datetime
from pathlib import Path
from typing import Any

from .config import Config
from .recorders import default_recorder_backend
from .sessions import create_session, update_metadata
from .util import append_log, atomic_write_json, pid_alive, read_json, utc_now_iso


def _parse_dt(value: str) -> datetime:
    return datetime.fromisoformat(value)


def _duration_seconds(started_at: str, ended_at: str) -> float:
    return round((_parse_dt(ended_at) - _parse_dt(started_at)).total_seconds(), 2)


def current_recording(config: Config) -> dict[str, Any] | None:
    if not config.current_state_path.exists():
        return None
    try:
        state = read_json(config.current_state_path)
    except Exception:
        config.current_state_path.unlink(missing_ok=True)
        return None
    pid = int(state.get("recorder_pid", 0))
    if pid and pid_alive(pid):
        return state
    config.current_state_path.unlink(missing_ok=True)
    return None


def start_recording(config: Config) -> dict[str, Any]:
    backend = default_recorder_backend()
    if not backend.available():
        raise RuntimeError(f"{backend.name} is not installed; cannot record audio")
    if current_recording(config):
        raise RuntimeError("recording is already active")

    metadata = create_session(config)
    audio_path = Path(str(metadata["audio_path"]))
    status_log = audio_path.parent / "status.log"
    append_log(status_log, f"starting recorder backend={backend.name}")

    proc = backend.start(audio_path, audio_path.parent / backend.log_name)
    state = {
        "session_dir": str(audio_path.parent),
        "metadata_path": str(audio_path.parent / "metadata.json"),
        "audio_path": str(audio_path),
        "recorder_pid": proc.pid,
        "recorder_backend": backend.name,
        "started_at": metadata["started_at"],
    }
    atomic_write_json(config.current_state_path, state)
    append_log(status_log, f"{backend.name} pid={proc.pid}")
    return state


def stop_recording(config: Config, state: dict[str, Any]) -> dict[str, Any]:
    metadata = read_json(Path(str(state["metadata_path"])))
    backend = default_recorder_backend()
    pid = int(state["recorder_pid"])
    status_log = Path(str(state["session_dir"])) / "status.log"
    append_log(status_log, f"stopping recorder backend={state.get('recorder_backend', backend.name)} pid={pid}")

    backend.stop(pid)
    if pid_alive(pid):
        append_log(status_log, "recorder backend did not exit cleanly")

    time.sleep(0.2)
    ended_at = utc_now_iso()
    audio_path = Path(str(metadata["audio_path"]))
    errors = list(metadata.get("errors", []))
    if not audio_path.exists() or audio_path.stat().st_size == 0:
        errors.append("Recording stopped but audio file was missing or empty.")
        status = "failed"
    else:
        status = "recorded"

    metadata = update_metadata(
        metadata,
        ended_at=ended_at,
        duration_seconds=_duration_seconds(str(metadata["started_at"]), ended_at),
        status=status,
        errors=errors,
    )
    config.current_state_path.unlink(missing_ok=True)
    append_log(status_log, f"recording stopped status={status}")
    return metadata
