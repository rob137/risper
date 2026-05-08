from __future__ import annotations

import json
import os
import tempfile
import unittest
from pathlib import Path

from helpers import write_test_config
from risper.config import load_config
from risper.sessions import create_session, read_events
from risper.transcription_state import cancel_transcription, current_transcription, start_transcription_state


class TranscriptionStateTests(unittest.TestCase):
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

    def test_cancel_marks_session_cancelled_and_clears_state(self) -> None:
        metadata = create_session(self.config)
        start_transcription_state(self.config, metadata, "test-profile")
        state = current_transcription(self.config)

        self.assertIsNotNone(state)
        self.assertTrue(cancel_transcription(self.config, state or {}))

        persisted = json.loads(Path(str(metadata["audio_path"])).parent.joinpath("metadata.json").read_text())
        self.assertEqual(persisted["status"], "cancelled")
        self.assertFalse(self.config.current_transcription_path.exists())
        self.assertEqual(read_events(metadata)[-1]["event"], "transcription.cancel_requested")


if __name__ == "__main__":
    unittest.main()
