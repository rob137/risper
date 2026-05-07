from __future__ import annotations

import argparse
import os
import platform
import shutil
import subprocess
from pathlib import Path

from .config import load_config
from .models import active_profile, load_profiles
from .platforms import current_platform
from .sessions import events_path, find_session, read_events, session_dir


def _run(command: list[str]) -> str:
    try:
        return subprocess.run(command, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=5).stdout.strip()
    except Exception as exc:
        return f"unavailable: {exc}"


def _tail(path: Path, lines: int = 8) -> list[str]:
    if not path.exists():
        return []
    return path.read_text(encoding="utf-8", errors="replace").splitlines()[-lines:]


def _print_session_diagnosis(session_id: str) -> int:
    config = load_config()
    metadata = find_session(config, session_id)
    if not metadata:
        print(f"No such session: {session_id}")
        return 1

    root = session_dir(metadata)
    audio_path = Path(str(metadata.get("audio_path", "")))
    raw_path = Path(str(metadata.get("transcript_raw_path", "")))
    clean_path = Path(str(metadata.get("transcript_clean_path", "")))
    print(f"Risper session diagnosis: {metadata.get('session_id')}")
    print("=" * 64)
    print(f"session_dir          {root}")
    print(f"status               {metadata.get('status')}")
    print(f"started_at           {metadata.get('started_at')}")
    print(f"ended_at             {metadata.get('ended_at')}")
    print(f"duration_seconds     {metadata.get('duration_seconds')}")
    print(f"session_type         {metadata.get('session_type')}")
    print(f"engine               {metadata.get('transcription_engine')}")
    print(f"model                {metadata.get('model')}")
    print(f"language             {metadata.get('language')}")
    print(f"paste_attempted      {metadata.get('paste_attempted')}")
    print(f"paste_succeeded      {metadata.get('paste_succeeded')}")
    print(f"errors               {len(metadata.get('errors') or [])}")
    for error in metadata.get("errors") or []:
        print(f"  - {error}")
    print()
    print("Files:")
    for label, path in [
        ("audio", audio_path),
        ("raw transcript", raw_path),
        ("clean transcript", clean_path),
        ("metadata", root / "metadata.json"),
        ("events", events_path(metadata)),
        ("status log", root / "status.log"),
        ("error log", root / "error.log"),
        ("recorder log", root / "pw-record.log"),
    ]:
        size = path.stat().st_size if path.exists() else 0
        print(f"  {label:<16} {'yes' if path.exists() else 'no ':<3} {size:>9}  {path}")
    print()
    print("Recent events:")
    for event in read_events(metadata, limit=12):
        detail = {key: value for key, value in event.items() if key not in {"timestamp", "event"}}
        print(f"  {event.get('timestamp', '')} {event.get('event', '')} {detail}")
    print()
    print("Status log tail:")
    for line in _tail(root / "status.log"):
        print(f"  {line}")
    print()
    print("Error log tail:")
    for line in _tail(root / "error.log"):
        print(f"  {line}")
    return 0


def _print_environment_diagnosis() -> int:
    desktop = current_platform()
    print("Risper diagnosis")
    print("=====================")
    print(f"Platform: {desktop.name}")
    print(_run(["lsb_release", "-a"]) or platform.platform())
    print()
    print(f"XDG_SESSION_TYPE={os.environ.get('XDG_SESSION_TYPE','')}")
    print(f"XDG_CURRENT_DESKTOP={os.environ.get('XDG_CURRENT_DESKTOP','')}")
    print(f"DESKTOP_SESSION={os.environ.get('DESKTOP_SESSION','')}")
    print()
    print(_run(["gnome-shell", "--version"]))
    print()
    print("Commands:")
    for command in desktop.diagnostic_commands():
        print(f"  {command:<18} {shutil.which(command) or '-'}")
    print()
    print(f"Python: {platform.python_version()}")
    for module in ["gi", "faster_whisper", "whisper", "sounddevice", "numpy"]:
        try:
            __import__(module)
            print(f"  module {module:<15} yes")
        except Exception as exc:
            print(f"  module {module:<15} no ({exc.__class__.__name__})")
    print()
    config = load_config()
    print("Risper config:")
    print(f"  config              {config.config_path}")
    print(f"  sessions            {config.sessions_dir}")
    print(f"  transcription       {config.transcription_engine}")
    profiles = load_profiles(config)
    print(f"  models file         {config.models_path}")
    print(f"  model profiles      {len(profiles)}")
    if profiles:
        profile = active_profile(config)
        print(f"  selected model      {profile.id}")
        print(f"  selected engine     {profile.engine}")
        print(f"  selected model name {profile.model}")
        binary = profile.command.split(" ", 1)[0]
        print(f"  command binary      {binary} ({'yes' if Path(binary).exists() else 'missing'})")
    print(f"  paste mode          {config.paste_mode}")
    print(f"  double Alt          {'enabled' if config.double_alt_enabled else 'disabled'}")
    print(f"  double Alt window   {config.double_alt_window_ms} ms")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Print Risper environment or session diagnostics.")
    parser.add_argument("session_id", nargs="?", help="Session id to inspect, or 'last'. Omit for environment diagnostics.")
    args = parser.parse_args(argv)
    if args.session_id:
        return _print_session_diagnosis(args.session_id)
    return _print_environment_diagnosis()


if __name__ == "__main__":
    raise SystemExit(main())
