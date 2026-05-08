from __future__ import annotations

import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from helpers import write_test_config
from risper.config import load_config
from risper.models import ModelProfile
from risper.sessions import create_session, read_events, update_metadata
from risper.toggle import _finish_session, main


class ToggleFinishTests(unittest.TestCase):
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
        self.config = load_config()

    def tearDown(self) -> None:
        for key, value in self.old_env.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value
        self.tempdir.cleanup()

    def test_finish_copies_transcript_without_attempting_paste(self) -> None:
        metadata = update_metadata(create_session(self.config), status="recorded")
        Path(str(metadata["audio_path"])).write_bytes(b"audio")

        with (
            patch(
                "risper.toggle.active_profile",
                return_value=ModelProfile("test", "test-engine", "test-model", "en", "test-command"),
            ),
            patch("risper.toggle.transcribe", return_value="hello"),
            patch("risper.toggle.copy_text", return_value=(True, "copied")),
            patch("risper.toggle.notify"),
            patch("risper.toggle.play"),
        ):
            code = _finish_session(self.config, metadata)

        self.assertEqual(code, 0)
        persisted = json.loads(Path(str(metadata["audio_path"])).parent.joinpath("metadata.json").read_text())
        self.assertEqual(persisted["status"], "complete")
        self.assertFalse(persisted["paste_attempted"])
        self.assertFalse(persisted["paste_helper_succeeded"])
        self.assertFalse(persisted["paste_succeeded"])
        self.assertEqual(persisted["paste_confirmation"], "not_attempted_automatic_paste_disabled")
        self.assertEqual(read_events(metadata)[-1]["event"], "paste.skipped")

    def test_main_cancels_active_transcription_instead_of_starting_recording(self) -> None:
        state = {"controller_pid": 123, "worker_pid": 456}

        with (
            patch("risper.toggle.current_transcription", return_value=state),
            patch("risper.toggle.cancel_transcription", return_value=True) as cancel,
            patch("risper.toggle.start_recording") as start,
            patch("risper.toggle.notify") as notify,
            patch("risper.toggle.play"),
        ):
            code = main()

        self.assertEqual(code, 0)
        cancel.assert_called_once_with(self.config, state)
        start.assert_not_called()
        notify.assert_called_once_with("🛑 Risper cancelled", "Transcription stopped.")


if __name__ == "__main__":
    unittest.main()
