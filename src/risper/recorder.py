from __future__ import annotations

import time
from collections.abc import Sequence
from datetime import datetime
from pathlib import Path
from typing import Any

from .config import Config
from .recorders import MIC, default_recorder_backend, mix_sources, mixer_available
from .sessions import append_event, create_session, update_metadata
from .util import append_log, atomic_write_json, pid_alive, read_json, utc_now_iso


def _parse_dt(value: str) -> datetime:
    return datetime.fromisoformat(value)


def _duration_seconds(started_at: str, ended_at: str) -> float:
    return round((_parse_dt(ended_at) - _parse_dt(started_at)).total_seconds(), 2)


def _part_path(audio_path: Path, source: str, sources: Sequence[str]) -> Path:
    # a single source writes audio.wav directly, so dictation needs no mixing step
    if len(sources) == 1:
        return audio_path
    return audio_path.with_suffix(f".{source}.wav")


def _state_pids(state: dict[str, Any]) -> dict[str, int]:
    return {source: int(pid) for source, pid in (state.get("recorder_pids") or {}).items()}


def current_recording(config: Config) -> dict[str, Any] | None:
    if not config.current_state_path.exists():
        return None
    try:
        state = read_json(config.current_state_path)
    except Exception:
        config.current_state_path.unlink(missing_ok=True)
        return None
    # one source dying does not end the recording; the rest are still capturing
    if any(pid_alive(pid) for pid in _state_pids(state).values()):
        return state
    config.current_state_path.unlink(missing_ok=True)
    return None


def start_recording(config: Config, sources: Sequence[str] = (MIC,)) -> dict[str, Any]:
    backend = default_recorder_backend()
    if not backend.available():
        raise RuntimeError(f"{backend.name} is not installed; cannot record audio")
    unsupported = [source for source in sources if source not in backend.supported_sources]
    if unsupported:
        raise RuntimeError(f"{backend.name} cannot record: {', '.join(unsupported)}")
    if len(sources) > 1 and not mixer_available():
        raise RuntimeError("ffmpeg is not installed; cannot combine multiple audio sources")
    if current_recording(config):
        raise RuntimeError("recording is already active")

    metadata = create_session(config)
    metadata = update_metadata(metadata, audio_sources=list(sources))
    audio_path = Path(str(metadata["audio_path"]))
    session_dir = audio_path.parent
    status_log = session_dir / "status.log"
    part_paths = {source: _part_path(audio_path, source, sources) for source in sources}

    append_log(status_log, f"starting recorder backend={backend.name} sources={','.join(sources)}")
    append_event(
        metadata,
        "recorder.starting",
        backend=backend.name,
        sources=list(sources),
        audio_path=str(audio_path),
        part_paths={source: str(path) for source, path in part_paths.items()},
    )

    pids: dict[str, int] = {}
    try:
        for source in sources:
            process = backend.start(source, part_paths[source], session_dir / backend.log_name(source))
            pids[source] = process.pid
    except Exception:
        backend.stop_all(pids.values())
        append_event(metadata, "recorder.start_failed", sources=list(sources), started=sorted(pids))
        update_metadata(metadata, status="failed")
        raise

    state = {
        "session_dir": str(session_dir),
        "metadata_path": str(session_dir / "metadata.json"),
        "audio_path": str(audio_path),
        "sources": list(sources),
        "recorder_pids": pids,
        "part_paths": {source: str(path) for source, path in part_paths.items()},
        "recorder_backend": backend.name,
        "started_at": metadata["started_at"],
    }
    atomic_write_json(config.current_state_path, state)
    append_log(status_log, f"{backend.name} pids={pids}")
    append_event(metadata, "recorder.started", backend=backend.name, pids=pids)
    return state


def _combine_parts(
    metadata: dict[str, Any],
    state: dict[str, Any],
    audio_path: Path,
    status_log: Path,
) -> list[str]:
    """Mix per-source captures into audio.wav, returning any errors to record."""
    sources = list(state.get("sources") or [])
    raw_parts = state.get("part_paths") or {}
    parts = [Path(str(raw_parts[source])) for source in sources if source in raw_parts]
    if len(parts) < 2:
        return []

    try:
        used = mix_sources(parts, audio_path)
    except Exception as exc:
        message = f"could not combine audio sources: {exc}"
        append_log(status_log, message)
        append_event(metadata, "recorder.mix_failed", error=str(exc), parts=[str(part) for part in parts])
        return [message]

    used_sources = [source for source in sources if Path(str(raw_parts[source])) in used]
    dropped = [source for source in sources if source not in used_sources]
    append_log(status_log, f"combined sources={','.join(used_sources)} dropped={','.join(dropped) or 'none'}")
    append_event(metadata, "recorder.mixed", used_sources=used_sources, dropped_sources=dropped)
    for part in parts:
        part.unlink(missing_ok=True)
    if dropped:
        return [f"No audio captured from: {', '.join(dropped)}."]
    return []


def stop_recording(config: Config, state: dict[str, Any]) -> dict[str, Any]:
    metadata = read_json(Path(str(state["metadata_path"])))
    backend = default_recorder_backend()
    pids = _state_pids(state)
    status_log = Path(str(state["session_dir"])) / "status.log"
    backend_name = str(state.get("recorder_backend", backend.name))
    append_log(status_log, f"stopping recorder backend={backend_name} pids={pids}")
    append_event(metadata, "recorder.stopping", backend=backend_name, pids=pids)

    backend.stop_all(pids.values())
    for source, pid in pids.items():
        if pid_alive(pid):
            append_log(status_log, f"recorder backend did not exit cleanly source={source}")
            append_event(metadata, "recorder.stop_unclean", source=source, pid=pid)

    time.sleep(0.2)
    ended_at = utc_now_iso()
    audio_path = Path(str(metadata["audio_path"]))
    errors = list(metadata.get("errors", []))
    errors += _combine_parts(metadata, state, audio_path, status_log)

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
    append_event(
        metadata,
        "recorder.stopped",
        status=status,
        audio_path=str(audio_path),
        audio_bytes=audio_path.stat().st_size if audio_path.exists() else 0,
        duration_seconds=metadata.get("duration_seconds"),
    )
    return metadata
