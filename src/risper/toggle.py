from __future__ import annotations

import sys
from pathlib import Path

from . import __version__
from .clipboard import copy_text
from .config import load_config
from .models import active_profile
from .recorder import current_recording, start_recording, stop_recording
from .sessions import append_event, update_metadata
from .sounds import play
from .transcription_state import (
    cancel_transcription,
    current_transcription,
    finish_transcription_state,
    set_transcription_worker_pid,
    start_transcription_state,
)
from .transcriber import transcribe
from .util import append_log, notify


def _finish_session(config, metadata: dict) -> int:
    session_dir = Path(str(metadata["audio_path"])).parent
    status_log = session_dir / "status.log"
    error_log = session_dir / "error.log"

    if metadata.get("status") != "recorded":
        append_event(metadata, "workflow.finish_rejected", status=metadata.get("status"))
        notify("Risper recording failed", "Audio was not captured cleanly; see session error log.")
        play(config, "error")
        return 1

    profile = active_profile(config)
    append_event(
        metadata,
        "transcription.starting",
        profile=profile.id,
        engine=profile.engine,
        model=profile.model,
        language=profile.language,
    )
    metadata = update_metadata(
        metadata,
        status="transcribing",
        transcription_engine=profile.engine,
        model=profile.model,
        language=profile.language,
    )
    append_log(status_log, "starting transcription")
    start_transcription_state(config, metadata, profile.id)
    notify("Risper transcribing", f"Using {profile.id}.")

    try:
        transcript = transcribe(
            config,
            Path(str(metadata["audio_path"])),
            Path(str(metadata["transcript_raw_path"])),
            Path(str(metadata["transcript_clean_path"])),
            profile=profile,
            on_process_start=lambda pid: set_transcription_worker_pid(config, pid),
        )
    except Exception as exc:
        message = f"transcription failed: {exc}"
        append_log(error_log, message)
        append_log(status_log, message)
        append_event(metadata, "transcription.failed", error=str(exc), error_type=exc.__class__.__name__)
        errors = list(metadata.get("errors", []))
        errors.append(message)
        update_metadata(metadata, status="failed", errors=errors)
        notify("Risper transcription failed", "Audio was saved. Configure a local engine and retranscribe.")
        play(config, "error")
        return 1
    finally:
        finish_transcription_state(config)

    append_event(
        metadata,
        "transcription.completed",
        raw_path=str(metadata["transcript_raw_path"]),
        clean_path=str(metadata["transcript_clean_path"]),
        transcript_chars=len(transcript),
    )
    copied, clipboard_message = copy_text(transcript)
    append_log(status_log, clipboard_message)
    append_event(
        metadata,
        "clipboard.copy",
        ok=copied,
        message=clipboard_message,
        transcript_chars=len(transcript),
    )
    if not copied:
        errors = list(metadata.get("errors", []))
        errors.append(clipboard_message)
        update_metadata(metadata, status="failed", errors=errors)
        notify("Risper clipboard failed", "Transcript was saved but not copied.")
        play(config, "error")
        return 1

    append_log(status_log, "automatic paste skipped; transcript left on clipboard")
    append_event(
        metadata,
        "paste.skipped",
        reason="automatic_paste_disabled",
        session_type=metadata.get("session_type"),
    )
    update_metadata(
        metadata,
        status="complete",
        paste_attempted=False,
        paste_helper_succeeded=False,
        paste_succeeded=False,
        paste_confirmation="not_attempted_automatic_paste_disabled",
    )
    notify("Risper copied", "Transcript is on the clipboard.")
    play(config, "stop")
    return 0


def main() -> int:
    config = load_config()
    transcription = current_transcription(config)
    if transcription:
        cancel_transcription(config, transcription)
        notify("Risper cancelled", "Transcription stopped.")
        play(config, "stop")
        return 0

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

    notify("Risper listening", "Run risper-toggle again to stop.")
    play(config, "start")
    print(f"Risper {__version__}: recording {state['session_dir']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
