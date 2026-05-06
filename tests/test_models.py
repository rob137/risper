from __future__ import annotations

import os
import tempfile
import unittest
from pathlib import Path

from risper.config import load_config
from risper.models import ModelProfile, active_profile, load_profiles, select_profile, write_profile
from helpers import write_test_config


class ModelProfileTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.old_env = {
            key: os.environ.get(key)
            for key in ("XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME")
        }
        os.environ["XDG_CONFIG_HOME"] = str(self.root / "config")
        os.environ["XDG_DATA_HOME"] = str(self.root / "data")
        os.environ["XDG_STATE_HOME"] = str(self.root / "state")
        write_test_config(self.root)

    def tearDown(self) -> None:
        for key, value in self.old_env.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value
        self.tempdir.cleanup()

    def test_write_profile_and_select_active_profile(self) -> None:
        config = load_config()
        write_profile(
            config,
            ModelProfile(
                id="slow",
                engine="engine-a",
                model="model-a",
                language="en",
                command="/bin/echo slow",
            ),
        )
        write_profile(
            config,
            ModelProfile(
                id="fast",
                engine="engine-b",
                model="model-b",
                language="en",
                command="/bin/echo fast",
            ),
            select=True,
        )

        reloaded = load_config()
        self.assertEqual(reloaded.selected_model, "fast")
        self.assertEqual(active_profile(reloaded).id, "fast")
        self.assertEqual(active_profile(reloaded).engine, "engine-b")

    def test_active_profile_uses_selected_before_default_or_sort_order(self) -> None:
        config = load_config()
        write_profile(
            config,
            ModelProfile("aaa", "wrong-engine", "wrong-model", "en", "/bin/echo wrong"),
        )
        write_profile(
            config,
            ModelProfile("zzz", "right-engine", "right-model", "en", "/bin/echo right"),
            select=True,
        )

        self.assertEqual(active_profile(load_config()).id, "zzz")

    def test_profiles_without_commands_are_ignored(self) -> None:
        config = load_config()
        config.models_path.write_text(
            """
[models.empty]
engine = "ignored"
model = "ignored"
language = "en"
command = ""

[models.valid]
engine = "ok"
model = "ok"
language = "en"
command = "/bin/echo ok"
""".strip()
            + "\n",
            encoding="utf-8",
        )

        profiles = load_profiles(config)
        self.assertEqual(set(profiles), {"valid"})

    def test_select_profile_updates_config(self) -> None:
        select_profile("chosen")

        self.assertEqual(load_config().selected_model, "chosen")

    def test_active_profile_falls_back_to_default_profile(self) -> None:
        config = load_config()
        write_profile(config, ModelProfile("zzz", "engine-z", "model-z", "en", "/bin/echo z"))
        write_profile(config, ModelProfile("default", "engine-default", "model-default", "en", "/bin/echo default"))
        select_profile("missing")

        self.assertEqual(active_profile(load_config()).id, "default")

    def test_active_profile_falls_back_to_sorted_first_profile(self) -> None:
        config = load_config()
        write_profile(config, ModelProfile("bbb", "engine-b", "model-b", "en", "/bin/echo b"))
        write_profile(config, ModelProfile("aaa", "engine-a", "model-a", "en", "/bin/echo a"))
        select_profile("missing")

        self.assertEqual(active_profile(load_config()).id, "aaa")

    def test_legacy_transcription_command_becomes_default_profile(self) -> None:
        config = load_config()
        config.config_path.write_text(
            f"""
sessions_dir = "{self.root / "sessions"}"
selected_model = "default"
transcription_engine = "legacy-engine"
transcription_command = "/bin/echo legacy"
model = "legacy-model"
language = "cy"
paste_mode = "clipboard_only"
show_overlay = false
play_sounds = false
retention = "never"
""".lstrip(),
            encoding="utf-8",
        )
        config.models_path.write_text("# no models\n", encoding="utf-8")

        profile = active_profile(load_config())

        self.assertEqual(profile.id, "default")
        self.assertEqual(profile.engine, "legacy-engine")
        self.assertEqual(profile.model, "legacy-model")
        self.assertEqual(profile.language, "cy")
        self.assertEqual(profile.command, "/bin/echo legacy")

    def test_active_profile_raises_when_no_profiles_or_legacy_command(self) -> None:
        config = load_config()
        config.models_path.write_text("# no models\n", encoding="utf-8")

        with self.assertRaisesRegex(RuntimeError, "No transcription model configured"):
            active_profile(config)


if __name__ == "__main__":
    unittest.main()
