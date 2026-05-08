from __future__ import annotations

import shlex
import subprocess
import os
from pathlib import Path
from typing import Callable

from .config import Config
from .models import ModelProfile, active_profile
from .util import atomic_write_text


class TranscriptionUnavailable(RuntimeError):
    pass


def transcribe(
    config: Config,
    audio_path: Path,
    raw_path: Path,
    clean_path: Path,
    profile: ModelProfile | None = None,
    on_process_start: Callable[[int], None] | None = None,
) -> str:
    """Run a configured local transcription command.

    The command may print transcript text to stdout, or write directly to
    {raw}/{clean}. No default model download is attempted here.
    """
    profile = profile or active_profile(config)

    rendered = profile.command.format(
        audio=str(audio_path),
        raw=str(raw_path),
        raw_no_txt=str(raw_path.with_suffix("")),
        clean=str(clean_path),
        clean_no_txt=str(clean_path.with_suffix("")),
        model=profile.model,
        language=profile.language,
    )
    rendered = str(Path(rendered).expanduser()) if rendered.startswith("~") and " " not in rendered else rendered
    process = subprocess.Popen(
        shlex.split(rendered),
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        start_new_session=(os.name == "posix"),
    )
    if on_process_start:
        on_process_start(process.pid)
    stdout_text, stderr_text = process.communicate(timeout=None)
    result = subprocess.CompletedProcess(
        args=process.args,
        returncode=process.returncode,
        stdout=stdout_text,
        stderr=stderr_text,
    )
    result.check_returncode()

    stdout = result.stdout.strip()
    if stdout:
        atomic_write_text(raw_path, stdout + "\n")
        atomic_write_text(clean_path, stdout + "\n")
        return stdout
    if raw_path.exists():
        text = raw_path.read_text(encoding="utf-8").strip()
        if text:
            atomic_write_text(clean_path, text + "\n")
            return text
    if clean_path.exists():
        text = clean_path.read_text(encoding="utf-8").strip()
        if text:
            atomic_write_text(raw_path, text + "\n")
            return text
    raise TranscriptionUnavailable("transcription command produced no transcript")
