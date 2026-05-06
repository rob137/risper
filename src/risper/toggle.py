from __future__ import annotations

import subprocess
import sys
from pathlib import Path

from . import __version__
from .clipboard import copy_text
from .config import load_config
from .models import active_profile
from .paste import attempt_paste
from .recorder import current_recording, start_recording, stop_recording
from .sessions import update_metadata
from .sounds import play
from .transcriber import transcribe
from .util import append_log, notify


def _launch_overlay(recorder_pid: int, session_dir: str) -> None:
    subprocess.Popen(
        [sys.executable, "-m", "risper.overlay", str(recorder_pid), session_dir],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )


def _finish_session(config, metadata: dict) -> int:
    session_dir = Path(str(metadata["audio_path"])).parent
    status_log = session_dir / "status.log"
    error_log = session_dir / "error.log"

    if metadata.get("status") != "recorded":
        notify("Risper recording failed", "Audio was not captured cleanly; see session error log.")
        play(config, "error")
        return 1

    profile = active_profile(config)
    metadata = update_metadata(
        metadata,
        status="transcribing",
        transcription_engine=profile.engine,
        model=profile.model,
        language=profile.language,
    )
    append_log(status_log, "starting transcription")

    try:
        transcript = transcribe(
            config,
            Path(str(metadata["audio_path"])),
            Path(str(metadata["transcript_raw_path"])),
            Path(str(metadata["transcript_clean_path"])),
            profile=profile,
        )
    except Exception as exc:
        message = f"transcription failed: {exc}"
        append_log(error_log, message)
        append_log(status_log, message)
        errors = list(metadata.get("errors", []))
        errors.append(message)
        update_metadata(metadata, status="failed", errors=errors)
        notify("Risper transcription failed", "Audio was saved. Configure a local engine and retranscribe.")
        play(config, "error")
        return 1

    copied, clipboard_message = copy_text(transcript)
    append_log(status_log, clipboard_message)
    if not copied:
        errors = list(metadata.get("errors", []))
        errors.append(clipboard_message)
        update_metadata(metadata, status="failed", errors=errors)
        notify("Risper clipboard failed", "Transcript was saved but not copied.")
        play(config, "error")
        return 1

    metadata = update_metadata(metadata, status="pasting", paste_attempted=True)
    pasted, paste_message = attempt_paste(config)
    append_log(status_log, paste_message)
    status = "complete" if pasted else "paste_failed"
    errors = list(metadata.get("errors", []))
    if not pasted:
        errors.append(paste_message)

    update_metadata(
        metadata,
        status=status,
        paste_attempted=True,
        paste_succeeded=pasted,
        errors=errors,
    )
    if pasted:
        notify("Risper complete", "Transcript copied and paste attempted.")
    else:
        notify("Risper copied", "Paste was unavailable; transcript is on the clipboard.")
    play(config, "stop")
    return 0


def main() -> int:
    config = load_config()
    state = current_recording(config)
    if state:
        play(config, "stop")
        metadata = stop_recording(config, state)
        return _finish_session(config, metadata)

    try:
        state = start_recording(config)
    except Exception as exc:
        notify("Risper could not start", str(exc))
        print(f"risper-toggle: {exc}", file=sys.stderr)
        play(config, "error")
        return 1

    if config.show_overlay:
        _launch_overlay(int(state["recorder_pid"]), str(state["session_dir"]))
    notify("Risper listening", "Run risper-toggle again to stop.")
    play(config, "start")
    print(f"Risper {__version__}: recording {state['session_dir']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
