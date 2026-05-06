from __future__ import annotations

import argparse
import sys
from pathlib import Path

from .config import load_config
from .platforms import current_platform
from .session_actions import copy_transcript, transcript_path
from .sessions import last_session


def _open_path(path: Path) -> int:
    ok, message = current_platform().open_path(path)
    if not ok:
        print(message, file=sys.stderr)
        return 1
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Open Risper files and folders.")
    parser.add_argument(
        "target",
        nargs="?",
        default="recordings",
        choices=["recordings", "last-session", "last-transcript", "last-audio", "play-last", "config", "copy-last"],
    )
    args = parser.parse_args(argv)
    config = load_config()

    if args.target == "recordings":
        return _open_path(config.sessions_dir)
    if args.target == "config":
        return _open_path(config.config_path)

    metadata = last_session(config)
    if not metadata:
        print("No Risper sessions yet.", file=sys.stderr)
        return 1
    session_dir = Path(str(metadata["audio_path"])).parent

    if args.target == "last-session":
        return _open_path(session_dir)
    if args.target in {"last-audio", "play-last"}:
        return _open_path(Path(str(metadata["audio_path"])))
    if args.target == "last-transcript":
        transcript = transcript_path(metadata)
        if not transcript:
            print("Last session has no transcript.", file=sys.stderr)
            return 1
        return _open_path(transcript)
    if args.target == "copy-last":
        ok, message = copy_transcript(metadata)
        print(message)
        return 0 if ok else 1
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
