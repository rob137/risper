from __future__ import annotations

import argparse

from .config import load_config
from .platforms import current_platform
from .util import append_log


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Attempt system paste from the current clipboard.")
    parser.add_argument(
        "--mode",
        choices=("auto", "xdotool", "wtype", "ydotool", "dotool"),
        help="Paste helper to use. Defaults to configured paste_mode, or auto if that is clipboard_only.",
    )
    args = parser.parse_args(argv)

    config = load_config()
    mode = args.mode or config.paste_mode
    if mode == "clipboard_only":
        mode = "auto"
    ok, message = current_platform().attempt_paste(mode)
    append_log(config.log_path, f"paste_now mode={mode} ok={ok} message={message}")
    print(message)
    return 0 if ok else 1


if __name__ == "__main__":
    raise SystemExit(main())
