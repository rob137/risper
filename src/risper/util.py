from __future__ import annotations

import json
import os
import threading
import time
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Callable, Iterator


def utc_now_iso() -> str:
    return datetime.now(timezone.utc).astimezone().isoformat(timespec="seconds")


def session_id_from_now() -> str:
    return datetime.now().strftime("%Y-%m-%d_%H-%M-%S")


def atomic_write_text(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_name(f".{path.name}.tmp")
    tmp.write_text(text, encoding="utf-8")
    os.replace(tmp, path)


def atomic_write_json(path: Path, data: dict[str, Any]) -> None:
    atomic_write_text(path, json.dumps(data, indent=2, sort_keys=True) + "\n")


def read_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def append_log(path: Path, message: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as handle:
        handle.write(f"{utc_now_iso()} {message}\n")


def notify(title: str, body: str = "") -> None:
    if not title:
        return
    from .platforms import current_platform

    current_platform().notify(title, body)


@contextmanager
def notify_heartbeat(
    title: str,
    body: str,
    interval: float = 10.0,
    on_beat: Callable[[], None] | None = None,
) -> Iterator[None]:
    stop = threading.Event()
    started = time.monotonic()

    def beat() -> None:
        while not stop.wait(interval):
            elapsed = int(time.monotonic() - started)
            notify(title, f"{body} {elapsed}s elapsed.")
            if on_beat:
                on_beat()

    thread = threading.Thread(target=beat, daemon=True)
    thread.start()
    try:
        yield
    finally:
        stop.set()
        # Wait out any in-flight beat so it cannot replace the notification sent after this block.
        thread.join(timeout=5)


def pid_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def wait_until(predicate, timeout_seconds: float, interval: float = 0.05) -> bool:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        if predicate():
            return True
        time.sleep(interval)
    return predicate()
