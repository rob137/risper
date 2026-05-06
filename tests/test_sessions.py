from __future__ import annotations

import json
import os
import tempfile
import unittest
from pathlib import Path

from risper.config import load_config
from risper.sessions import create_session, mark_incomplete_recordings_recovered, update_metadata
from helpers import write_test_config


class SessionTests(unittest.TestCase):
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

    def test_create_session_writes_recoverable_files_immediately(self) -> None:
        metadata = create_session(self.config)
        session_dir = Path(metadata["audio_path"]).parent

        self.assertTrue(session_dir.exists())
        self.assertTrue((session_dir / "metadata.json").exists())
        self.assertTrue((session_dir / "status.log").exists())
        self.assertTrue((session_dir / "error.log").exists())
        self.assertEqual(metadata["status"], "recording")

        persisted = json.loads((session_dir / "metadata.json").read_text(encoding="utf-8"))
        self.assertEqual(persisted["session_id"], metadata["session_id"])

    def test_update_metadata_is_persisted(self) -> None:
        metadata = create_session(self.config)
        update_metadata(metadata, status="complete", paste_succeeded=True)
        session_dir = Path(metadata["audio_path"]).parent

        persisted = json.loads((session_dir / "metadata.json").read_text(encoding="utf-8"))
        self.assertEqual(persisted["status"], "complete")
        self.assertTrue(persisted["paste_succeeded"])

    def test_incomplete_recording_is_marked_recovered(self) -> None:
        metadata = create_session(self.config)

        count = mark_incomplete_recordings_recovered(self.config)

        self.assertEqual(count, 1)
        session_dir = Path(metadata["audio_path"]).parent
        persisted = json.loads((session_dir / "metadata.json").read_text(encoding="utf-8"))
        self.assertEqual(persisted["status"], "recovered")
        self.assertIn("Recovered incomplete recording", persisted["errors"][0])


if __name__ == "__main__":
    unittest.main()
