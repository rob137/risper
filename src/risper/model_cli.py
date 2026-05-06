from __future__ import annotations

import argparse
import sys

from .config import load_config
from .models import ModelProfile, active_profile, load_profiles, select_profile, write_profile


def _list() -> int:
    config = load_config()
    profiles = load_profiles(config)
    active = active_profile(config).id if profiles else ""
    if not profiles:
        print(f"No profiles configured. Edit {config.models_path}.")
        return 0
    print(f"{'id':<24} {'engine':<14} {'model':<16} language")
    for profile_id, profile in sorted(profiles.items()):
        marker = "*" if profile_id == active else " "
        print(f"{marker} {profile_id:<22} {profile.engine:<14} {profile.model:<16} {profile.language}")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Manage Risper model profiles.")
    sub = parser.add_subparsers(dest="action")
    sub.add_parser("list")
    sub.add_parser("current")

    select = sub.add_parser("select")
    select.add_argument("profile_id")

    add = sub.add_parser("add-external")
    add.add_argument("profile_id")
    add.add_argument("--engine", required=True)
    add.add_argument("--model", required=True)
    add.add_argument("--command", required=True)
    add.add_argument("--language", default="en")
    add.add_argument("--select", action="store_true")

    args = parser.parse_args(argv)
    config = load_config()

    if args.action in {None, "list"}:
        return _list()
    if args.action == "current":
        profile = active_profile(config)
        print(f"{profile.id}: {profile.engine} {profile.model} ({profile.language})")
        return 0
    if args.action == "select":
        profiles = load_profiles(config)
        if args.profile_id not in profiles:
            print(f"No such profile: {args.profile_id}", file=sys.stderr)
            return 1
        select_profile(args.profile_id)
        print(f"Selected {args.profile_id}")
        return 0
    if args.action == "add-external":
        write_profile(
            config,
            ModelProfile(
                id=args.profile_id,
                engine=args.engine,
                model=args.model,
                language=args.language,
                command=args.command,
            ),
            select=args.select,
        )
        print(f"Added {args.profile_id}")
        return 0
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
