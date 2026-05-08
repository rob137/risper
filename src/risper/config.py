from __future__ import annotations

import os
import shutil
import tomllib
from dataclasses import dataclass
from pathlib import Path


APP_NAME = "risper"


def xdg_config_home() -> Path:
    return Path(os.environ.get("XDG_CONFIG_HOME", Path.home() / ".config"))


def xdg_data_home() -> Path:
    return Path(os.environ.get("XDG_DATA_HOME", Path.home() / ".local" / "share"))


def xdg_state_home() -> Path:
    return Path(os.environ.get("XDG_STATE_HOME", Path.home() / ".local" / "state"))


@dataclass(frozen=True)
class Config:
    config_path: Path
    models_path: Path
    data_dir: Path
    sessions_dir: Path
    state_dir: Path
    current_state_path: Path
    current_transcription_path: Path
    log_path: Path
    selected_model: str
    transcription_engine: str
    transcription_command: str
    model: str
    language: str
    paste_mode: str
    auto_paste_after_copy: bool
    show_overlay: bool
    play_sounds: bool
    double_alt_enabled: bool
    double_alt_window_ms: int
    retention: str


DEFAULT_CONFIG = """# Risper user config.
# Paths support ~ expansion.
sessions_dir = "~/.local/share/risper/sessions"
selected_model = "default"
transcription_engine = "external"
transcription_command = ""
model = "base.en"
language = "en"
paste_mode = "clipboard_only" # clipboard_only | auto | xdotool | wtype | ydotool | dotool
auto_paste_after_copy = false
# The daemon no longer starts a standalone status window.
show_overlay = false
play_sounds = true
double_alt_enabled = false
double_alt_window_ms = 350
retention = "never"
"""


def config_path() -> Path:
    return xdg_config_home() / APP_NAME / "config.toml"


def ensure_default_config() -> Path:
    path = config_path()
    path.parent.mkdir(parents=True, exist_ok=True)
    if not path.exists():
        path.write_text(DEFAULT_CONFIG, encoding="utf-8")
    return path


def models_path() -> Path:
    return xdg_config_home() / APP_NAME / "models.toml"


def update_config_value(key: str, value: str) -> None:
    path = ensure_default_config()
    lines = path.read_text(encoding="utf-8").splitlines()
    replacement = f'{key} = "{value}"'
    for index, line in enumerate(lines):
        if line.split("=", 1)[0].strip() == key:
            lines[index] = replacement
            break
    else:
        lines.append(replacement)
    path.write_text("\n".join(lines).rstrip() + "\n", encoding="utf-8")


def load_config() -> Config:
    path = ensure_default_config()
    raw = tomllib.loads(path.read_text(encoding="utf-8"))

    data_dir = xdg_data_home() / APP_NAME
    state_dir = xdg_state_home() / APP_NAME
    sessions_dir = Path(raw.get("sessions_dir", data_dir / "sessions")).expanduser()

    data_dir.mkdir(parents=True, exist_ok=True)
    state_dir.mkdir(parents=True, exist_ok=True)
    sessions_dir.mkdir(parents=True, exist_ok=True)

    paste_mode = str(raw.get("paste_mode", "clipboard_only"))
    if paste_mode not in {"auto", "clipboard_only", "xdotool", "wtype", "ydotool", "dotool"}:
        paste_mode = "clipboard_only"
    double_alt_window_ms = int(raw.get("double_alt_window_ms", 350))
    if double_alt_window_ms < 100:
        double_alt_window_ms = 100

    return Config(
        config_path=path,
        models_path=models_path(),
        data_dir=data_dir,
        sessions_dir=sessions_dir,
        state_dir=state_dir,
        current_state_path=state_dir / "current.json",
        current_transcription_path=state_dir / "current-transcription.json",
        log_path=state_dir / "risper.log",
        selected_model=str(raw.get("selected_model", "default")),
        transcription_engine=str(raw.get("transcription_engine", "external")),
        transcription_command=str(raw.get("transcription_command", "")),
        model=str(raw.get("model", "base.en")),
        language=str(raw.get("language", "en")),
        paste_mode=paste_mode,
        auto_paste_after_copy=bool(raw.get("auto_paste_after_copy", False)),
        show_overlay=bool(raw.get("show_overlay", False)),
        play_sounds=bool(raw.get("play_sounds", True)),
        double_alt_enabled=bool(raw.get("double_alt_enabled", False)),
        double_alt_window_ms=double_alt_window_ms,
        retention=str(raw.get("retention", "never")),
    )


def command_exists(name: str) -> bool:
    return shutil.which(name) is not None
