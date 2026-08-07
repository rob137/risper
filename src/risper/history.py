from __future__ import annotations

import argparse
import sys
from pathlib import Path

from .config import load_config
from .platforms import current_platform
from .session_actions import copy_transcript, open_session, play_audio, transcript_preview
from .sessions import all_sessions, prune_expired_audio


def _preview(metadata: dict) -> str:
    return transcript_preview(metadata)


def _print_table(limit: int) -> int:
    config = load_config()
    sessions = all_sessions(config)[:limit]
    if not sessions:
        print("No Risper sessions yet.")
        return 0
    print(f"{'session':<22} {'status':<14} {'dur':>7}  preview")
    for metadata in sessions:
        duration = metadata.get("duration_seconds")
        duration_text = "" if duration is None else f"{duration}s"
        print(
            f"{metadata.get('session_id',''):<22} "
            f"{metadata.get('status',''):<14} "
            f"{duration_text:>7}  "
            f"{_preview(metadata)}"
        )
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Show recent Risper dictations.")
    parser.add_argument("--limit", type=int, default=20)
    parser.add_argument("--open", dest="open_session", help="Open a session folder by id")
    parser.add_argument("--play", dest="play_session", help="Open/play a session audio file by id")
    parser.add_argument("--copy", dest="copy_session", help="Copy a session transcript by id")
    parser.add_argument("--retranscribe", dest="retranscribe_session", help="Retranscribe a session by id")
    parser.add_argument("--delete", dest="delete_session", help="Move a session to trash by id")
    parser.add_argument(
        "--prune-audio",
        action="store_true",
        help="Delete audio past audio_retention now, keeping transcripts",
    )
    args = parser.parse_args(argv)

    config = load_config()
    sessions = all_sessions(config)
    by_id = {str(item.get("session_id")): item for item in sessions}

    if args.prune_audio:
        if config.audio_retention_seconds is None:
            print("audio_retention is 'never'; nothing to prune.", file=sys.stderr)
            return 1
        print(f"Pruned audio from {prune_expired_audio(config)} session(s).")
        return 0

    if args.open_session:
        metadata = by_id.get(args.open_session)
        if not metadata:
            print(f"No such session: {args.open_session}", file=sys.stderr)
            return 1
        ok, message = open_session(metadata)
        if not ok:
            print(message, file=sys.stderr)
            return 1
        return 0

    if args.play_session:
        metadata = by_id.get(args.play_session)
        if not metadata:
            print(f"No such session: {args.play_session}", file=sys.stderr)
            return 1
        ok, message = play_audio(metadata)
        if not ok:
            print(message, file=sys.stderr)
            return 1
        return 0

    if args.copy_session:
        metadata = by_id.get(args.copy_session)
        if not metadata:
            print(f"No such session: {args.copy_session}", file=sys.stderr)
            return 1
        ok, message = copy_transcript(metadata)
        print(message)
        return 0 if ok else 1

    if args.retranscribe_session:
        from .retranscribe import retranscribe_session

        return retranscribe_session(args.retranscribe_session)

    if args.delete_session:
        metadata = by_id.get(args.delete_session)
        if not metadata:
            print(f"No such session: {args.delete_session}", file=sys.stderr)
            return 1
        session_dir = Path(str(metadata["audio_path"])).parent
        answer = input(f"Move {session_dir} to trash? Type DELETE to confirm: ")
        if answer != "DELETE":
            print("Cancelled.")
            return 1
        ok, message = current_platform().trash_path(session_dir)
        if not ok:
            shutil.rmtree(session_dir)
            message = f"removed permanently: {session_dir}"
        print(message)
        print(f"Deleted {metadata['session_id']}")
        return 0

    return _print_table(args.limit)


if __name__ == "__main__":
    raise SystemExit(main())
