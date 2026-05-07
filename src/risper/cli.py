from __future__ import annotations

import argparse
import subprocess


SERVICE_NAME = "risper.service"


def _systemctl(*args: str) -> int:
    return subprocess.run(["systemctl", "--user", *args], check=False).returncode


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Manage the Risper user daemon.")
    parser.add_argument(
        "command",
        nargs="?",
        default="start",
        choices=("start", "kill", "stop", "restart", "status"),
        help="Daemon action. Omit to enable autostart and start Risper.",
    )
    args = parser.parse_args(argv)

    if args.command == "start":
        code = _systemctl("enable", "--now", SERVICE_NAME)
        if code == 0:
            print("Risper daemon enabled and running.")
        return code
    if args.command in {"kill", "stop"}:
        code = _systemctl("stop", SERVICE_NAME)
        if code == 0:
            print("Risper daemon stopped. Autostart remains enabled.")
        return code
    if args.command == "restart":
        code = _systemctl("restart", SERVICE_NAME)
        if code == 0:
            print("Risper daemon restarted.")
        return code
    return _systemctl("status", SERVICE_NAME, "--no-pager")


if __name__ == "__main__":
    raise SystemExit(main())
