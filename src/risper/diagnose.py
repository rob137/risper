from __future__ import annotations

import os
import platform
import shutil
import subprocess
from pathlib import Path

from .config import load_config
from .models import active_profile, load_profiles
from .platforms import current_platform


def _run(command: list[str]) -> str:
    try:
        return subprocess.run(command, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=5).stdout.strip()
    except Exception as exc:
        return f"unavailable: {exc}"


def main() -> int:
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


if __name__ == "__main__":
    raise SystemExit(main())
