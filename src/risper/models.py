from __future__ import annotations

import json
import tomllib
from dataclasses import dataclass
from pathlib import Path

from .config import Config, update_config_value


@dataclass(frozen=True)
class ModelProfile:
    id: str
    engine: str
    model: str
    language: str
    command: str
    prompt: str = ""


DEFAULT_MODELS = """# Risper model profiles.
#
# Add profiles like:
#
# [models.whispercpp-base-en]
# engine = "whisper.cpp"
# model = "base.en"
# language = "en"
# command = "/path/to/whisper-cli -m /path/to/model.bin -f {audio} -l {language} --prompt \\"{prompt}\\" -nt -otxt -of {raw_no_txt}"
#
# An optional `prompt` biases decoding toward the words it lists (proper nouns,
# names, jargon). It is rendered into the command's {prompt} placeholder. Keep it
# a short comma list, not a paragraph. Only whisper.cpp uses it.
"""


def ensure_models_file(config: Config) -> Path:
    config.models_path.parent.mkdir(parents=True, exist_ok=True)
    if not config.models_path.exists():
        config.models_path.write_text(DEFAULT_MODELS, encoding="utf-8")
    return config.models_path


def load_profiles(config: Config) -> dict[str, ModelProfile]:
    path = ensure_models_file(config)
    raw = tomllib.loads(path.read_text(encoding="utf-8"))
    profiles: dict[str, ModelProfile] = {}
    for profile_id, data in raw.get("models", {}).items():
        if not isinstance(data, dict):
            continue
        command = str(data.get("command", "")).strip()
        if not command:
            continue
        profiles[profile_id] = ModelProfile(
            id=profile_id,
            engine=str(data.get("engine", "external")),
            model=str(data.get("model", profile_id)),
            language=str(data.get("language", config.language)),
            command=command,
            prompt=str(data.get("prompt", "")),
        )
    if not profiles and config.transcription_command.strip():
        profiles.setdefault(
            "default",
            ModelProfile(
                id="default",
                engine=config.transcription_engine,
                model=config.model,
                language=config.language,
                command=config.transcription_command,
            ),
        )
    return profiles


def active_profile(config: Config) -> ModelProfile:
    profiles = load_profiles(config)
    if not profiles:
        raise RuntimeError(
            "No transcription model configured. Add a profile to "
            f"{config.models_path} or set transcription_command in {config.config_path}."
        )
    if config.selected_model in profiles:
        return profiles[config.selected_model]
    if "default" in profiles:
        return profiles["default"]
    first_id = sorted(profiles)[0]
    return profiles[first_id]


def write_profile(config: Config, profile: ModelProfile, select: bool = False) -> None:
    profiles = load_profiles(config)
    profiles[profile.id] = profile
    lines = [DEFAULT_MODELS.rstrip(), ""]
    for item in sorted(profiles.values(), key=lambda profile: profile.id):
        lines.append(f"[models.{item.id}]")
        lines.append(f"engine = {json.dumps(item.engine)}")
        lines.append(f"model = {json.dumps(item.model)}")
        lines.append(f"language = {json.dumps(item.language)}")
        lines.append(f"command = {json.dumps(item.command)}")
        if item.prompt:
            lines.append(f"prompt = {json.dumps(item.prompt)}")
        lines.append("")
    config.models_path.write_text("\n".join(lines).rstrip() + "\n", encoding="utf-8")
    if select:
        update_config_value("selected_model", profile.id)


def select_profile(profile_id: str) -> None:
    update_config_value("selected_model", profile_id)
