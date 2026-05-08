from __future__ import annotations

import os
import signal
import time
from pathlib import Path
from typing import Any

from .config import Config
from .sessions import append_event, update_metadata
from .util import append_log, atomic_write_json, pid_alive, read_json, utc_now_iso


def _alive_pid(state: dict[str, Any]) -> int | None:
    for key in ("worker_pid", "controller_pid"):
        pid = int(state.get(key) or 0)
        if pid and pid_alive(pid):
            return pid
    return None


def current_transcription(config: Config) -> dict[str, Any] | None:
    if not config.current_transcription_path.exists():
        return None
    try:
        state = read_json(config.current_transcription_path)
    except Exception:
        config.current_transcription_path.unlink(missing_ok=True)
        return None
    if _alive_pid(state):
        return state
    config.current_transcription_path.unlink(missing_ok=True)
    return None


def start_transcription_state(config: Config, metadata: dict[str, Any], profile_id: str) -> None:
    state = {
        "session_dir": str(Path(str(metadata["audio_path"])).parent),
        "metadata_path": str(Path(str(metadata["audio_path"])).parent / "metadata.json"),
        "controller_pid": os.getpid(),
        "worker_pid": None,
        "profile": profile_id,
        "started_at": utc_now_iso(),
    }
    atomic_write_json(config.current_transcription_path, state)


def set_transcription_worker_pid(config: Config, worker_pid: int) -> None:
    state = current_transcription(config)
    if not state:
        return
    state["worker_pid"] = worker_pid
    atomic_write_json(config.current_transcription_path, state)


def finish_transcription_state(config: Config) -> None:
    config.current_transcription_path.unlink(missing_ok=True)


def _terminate_pid(pid: int, timeout_seconds: float = 1.0) -> None:
    if not pid or not pid_alive(pid):
        return
    try:
        os.kill(pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        if not pid_alive(pid):
            return
        time.sleep(0.05)
    try:
        os.kill(pid, signal.SIGKILL)
    except ProcessLookupError:
        return


def _signal_pid(pid: int, sig: signal.Signals) -> None:
    if not pid or not pid_alive(pid):
        return
    try:
        os.kill(pid, sig)
    except ProcessLookupError:
        return


def _terminate_process_group(pid: int) -> None:
    if not pid or os.name != "posix":
        _terminate_pid(pid)
        return
    try:
        os.killpg(pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    except PermissionError:
        _terminate_pid(pid)
        return
    deadline = time.monotonic() + 1.0
    while time.monotonic() < deadline:
        if not pid_alive(pid):
            return
        time.sleep(0.05)
    try:
        os.killpg(pid, signal.SIGKILL)
    except ProcessLookupError:
        return
    except PermissionError:
        _terminate_pid(pid, timeout_seconds=0.2)


def cancel_transcription(config: Config, state: dict[str, Any]) -> bool:
    metadata_path = Path(str(state.get("metadata_path", "")))
    try:
        metadata = read_json(metadata_path)
    except Exception:
        metadata = None

    message = "transcription cancelled by user"
    if metadata:
        status_log = Path(str(metadata["audio_path"])).parent / "status.log"
        append_log(status_log, message)
        append_event(
            metadata,
            "transcription.cancel_requested",
            controller_pid=state.get("controller_pid"),
            worker_pid=state.get("worker_pid"),
        )
        errors = list(metadata.get("errors", []))
        errors.append(message)
        update_metadata(metadata, status="cancelled", errors=errors)

    worker_pid = int(state.get("worker_pid") or 0)
    controller_pid = int(state.get("controller_pid") or 0)
    if controller_pid and controller_pid != os.getpid():
        _signal_pid(controller_pid, signal.SIGTERM)
    if worker_pid:
        _terminate_process_group(worker_pid)
    if controller_pid and controller_pid != os.getpid():
        _terminate_pid(controller_pid, timeout_seconds=0.2)
    config.current_transcription_path.unlink(missing_ok=True)
    append_log(config.log_path, message)
    return True
