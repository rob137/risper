from __future__ import annotations

from pathlib import Path


def write_test_config(root: Path) -> None:
    config_dir = root / "config" / "risper"
    config_dir.mkdir(parents=True, exist_ok=True)
    (config_dir / "config.toml").write_text(
        f"""
sessions_dir = "{root / "sessions"}"
selected_model = "default"
transcription_engine = "external"
transcription_command = ""
model = "base.en"
language = "en"
paste_mode = "clipboard_only"
play_sounds = false
double_alt_enabled = false
double_alt_window_ms = 350
audio_retention = "never"
""".lstrip(),
        encoding="utf-8",
    )
