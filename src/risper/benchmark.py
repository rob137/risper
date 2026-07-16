from __future__ import annotations

import argparse
import json
import resource
import shlex
import subprocess
import tempfile
import time
from pathlib import Path

from .config import load_config
from .models import active_profile, load_profiles
from .sessions import find_session


def _run_profile(profile, audio_path: Path) -> dict:
    with tempfile.TemporaryDirectory(prefix="risper-bench-") as tmp:
        root = Path(tmp)
        raw_path = root / "transcript.raw.txt"
        clean_path = root / "transcript.clean.txt"
        command = profile.command.format(
            audio=str(audio_path),
            raw=str(raw_path),
            raw_no_txt=str(raw_path.with_suffix("")),
            clean=str(clean_path),
            clean_no_txt=str(clean_path.with_suffix("")),
            model=profile.model,
            language=profile.language,
            prompt=profile.prompt,
        )
        before = resource.getrusage(resource.RUSAGE_CHILDREN)
        started = time.monotonic()
        result = subprocess.run(
            shlex.split(command),
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        elapsed = time.monotonic() - started
        after = resource.getrusage(resource.RUSAGE_CHILDREN)
        stdout = result.stdout.strip()
        if raw_path.exists():
            transcript = raw_path.read_text(encoding="utf-8").strip()
        elif clean_path.exists():
            transcript = clean_path.read_text(encoding="utf-8").strip()
        else:
            transcript = stdout
        user_seconds = after.ru_utime - before.ru_utime
        system_seconds = after.ru_stime - before.ru_stime
        cpu_percent = ((user_seconds + system_seconds) / elapsed * 100.0) if elapsed else 0.0
        return {
            "profile": profile.id,
            "engine": profile.engine,
            "model": profile.model,
            "returncode": result.returncode,
            "elapsed_seconds": round(elapsed, 3),
            "user_seconds": round(user_seconds, 3),
            "system_seconds": round(system_seconds, 3),
            "cpu_percent": round(cpu_percent, 1),
            "max_rss_mb": round(after.ru_maxrss / 1024.0, 1),
            "transcript_chars": len(transcript),
            "transcript_preview": " ".join(transcript.split())[:100],
            "stderr_tail": result.stderr.strip().splitlines()[-5:],
        }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Benchmark Risper transcription profiles.")
    parser.add_argument("session_or_audio", help="Session id, 'last', or path to an audio file.")
    parser.add_argument("--profile", action="append", help="Profile id to benchmark. Repeatable. Defaults to active profile.")
    parser.add_argument("--repeat", type=int, default=1)
    parser.add_argument("--json", action="store_true", help="Emit JSON instead of a table.")
    args = parser.parse_args(argv)

    config = load_config()
    session = find_session(config, args.session_or_audio)
    audio_path = Path(str(session["audio_path"])) if session else Path(args.session_or_audio).expanduser()
    if not audio_path.exists():
        raise SystemExit(f"Audio not found: {audio_path}")

    profiles = load_profiles(config)
    selected_profiles = args.profile or [active_profile(config).id]
    results = []
    for profile_id in selected_profiles:
        if profile_id not in profiles:
            raise SystemExit(f"No such profile: {profile_id}")
        for index in range(args.repeat):
            result = _run_profile(profiles[profile_id], audio_path)
            result["repeat"] = index + 1
            result["audio_path"] = str(audio_path)
            results.append(result)

    if args.json:
        print(json.dumps(results, indent=2))
        return 0

    print(f"{'profile':<24} {'rep':>3} {'wall':>8} {'cpu%':>7} {'rss_mb':>8} {'chars':>6}  preview")
    for result in results:
        print(
            f"{result['profile']:<24} "
            f"{result['repeat']:>3} "
            f"{result['elapsed_seconds']:>8.3f} "
            f"{result['cpu_percent']:>7.1f} "
            f"{result['max_rss_mb']:>8.1f} "
            f"{result['transcript_chars']:>6}  "
            f"{result['transcript_preview']}"
        )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
