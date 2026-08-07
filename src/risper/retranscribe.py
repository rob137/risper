from __future__ import annotations

import argparse
import sys
from pathlib import Path

from .clipboard import copy_text
from .config import load_config
from .models import active_profile
from .paste import attempt_paste
from .sessions import append_event, find_session, missing_audio_message, update_metadata
from .sounds import play
from .transcriber import transcribe
from .util import append_log, notify


def retranscribe_session(session_id: str, copy: bool = False, paste: bool = False) -> int:
    config = load_config()
    metadata = find_session(config, session_id)
    if not metadata:
        print(f"No such session: {session_id}", file=sys.stderr)
        return 1

    audio_path = Path(str(metadata["audio_path"]))
    if not audio_path.exists():
        print(missing_audio_message(metadata), file=sys.stderr)
        return 1

    session_dir = audio_path.parent
    status_log = session_dir / "status.log"
    error_log = session_dir / "error.log"
    profile = active_profile(config)
    metadata = update_metadata(
        metadata,
        status="transcribing",
        transcription_engine=profile.engine,
        model=profile.model,
        language=profile.language,
    )
    append_log(status_log, "starting retranscription")
    append_event(
        metadata,
        "retranscription.starting",
        profile=profile.id,
        engine=profile.engine,
        model=profile.model,
        language=profile.language,
    )
    play(config, "transcription_start")

    try:
        transcript = transcribe(
            config,
            audio_path,
            Path(str(metadata["transcript_raw_path"])),
            Path(str(metadata["transcript_clean_path"])),
            profile=profile,
        )
    except Exception as exc:
        message = f"retranscription failed: {exc}"
        append_log(error_log, message)
        append_log(status_log, message)
        append_event(metadata, "retranscription.failed", error=str(exc), error_type=exc.__class__.__name__)
        errors = list(metadata.get("errors", []))
        errors.append(message)
        update_metadata(metadata, status="failed", errors=errors)
        notify("⚠ Risper retranscription failed", "Audio was kept; see session error log.")
        play(config, "error")
        return 1

    append_event(
        metadata,
        "retranscription.completed",
        raw_path=str(metadata["transcript_raw_path"]),
        clean_path=str(metadata["transcript_clean_path"]),
        transcript_chars=len(transcript),
    )
    paste_attempted = False
    paste_succeeded = False
    errors = [err for err in list(metadata.get("errors", [])) if "transcription failed:" not in str(err)]

    if copy or paste:
        copied, clipboard_message = copy_text(transcript)
        append_log(status_log, clipboard_message)
        append_event(metadata, "clipboard.copy", ok=copied, message=clipboard_message, transcript_chars=len(transcript))
        if not copied:
            errors.append(clipboard_message)
            update_metadata(metadata, status="failed", errors=errors)
            play(config, "error")
            return 1
        if paste:
            paste_attempted = True
            append_event(
                metadata,
                "paste.attempting",
                mode=config.paste_mode,
                session_type=metadata.get("session_type"),
            )
            paste_succeeded, paste_message = attempt_paste(config)
            append_log(status_log, paste_message)
            confirmation = (
                "helper_returned_success_target_unverified"
                if paste_succeeded
                else "not_pasted_clipboard_retained"
            )
            append_event(
                metadata,
                "paste.result",
                ok=paste_succeeded,
                mode=config.paste_mode,
                session_type=metadata.get("session_type"),
                message=paste_message,
                confirmation=confirmation,
            )
            if not paste_succeeded:
                errors.append(paste_message)
        else:
            confirmation = "not_attempted"
    else:
        confirmation = "not_attempted"

    status = "complete" if not paste_attempted else "paste_attempted" if paste_succeeded else "paste_failed"
    update_metadata(
        metadata,
        status=status,
        paste_attempted=paste_attempted,
        paste_helper_succeeded=paste_succeeded,
        paste_succeeded=False if paste_attempted else paste_succeeded,
        paste_confirmation=confirmation,
        errors=errors,
    )
    play(config, "error" if status == "paste_failed" else "success")
    print(transcript)
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Retranscribe a saved Risper session.")
    parser.add_argument("session_id", nargs="?", default="last", help="Session id, or 'last'.")
    parser.add_argument("--copy", action="store_true", help="Copy transcript after retranscribing.")
    parser.add_argument("--paste", action="store_true", help="Copy and attempt paste after retranscribing.")
    args = parser.parse_args(argv)
    return retranscribe_session(args.session_id, copy=args.copy or args.paste, paste=args.paste)


if __name__ == "__main__":
    raise SystemExit(main())
