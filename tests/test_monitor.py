from __future__ import annotations

import os
import tempfile
import unittest
from pathlib import Path

from helpers import write_test_config
from risper.config import load_config
from risper.monitor import StatusModel, _detail_for_status, _display_detail
from risper.sessions import create_session, update_metadata
from risper.util import atomic_write_json


class MonitorModelTests(unittest.TestCase):
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

    def test_current_recording_is_visible(self) -> None:
        metadata = create_session(self.config)
        session_dir = Path(str(metadata["audio_path"])).parent
        atomic_write_json(
            self.config.current_state_path,
            {
                "session_dir": str(session_dir),
                "metadata_path": str(session_dir / "metadata.json"),
                "audio_path": metadata["audio_path"],
                "recorder_pid": os.getpid(),
            },
        )

        snapshot = StatusModel().snapshot(self.config, now=10.0)

        self.assertTrue(snapshot.visible)
        self.assertEqual(snapshot.status, "recording")
        self.assertEqual(snapshot.session_id, metadata["session_id"])

    def test_active_transcription_is_visible_without_current_recording(self) -> None:
        metadata = create_session(self.config)
        update_metadata(metadata, status="transcribing")

        snapshot = StatusModel().snapshot(self.config, now=10.0)

        self.assertTrue(snapshot.visible)
        self.assertEqual(snapshot.status, "transcribing")
        self.assertIn("transcribing", snapshot.title)

    def test_terminal_status_is_held_only_when_observed_after_active_state(self) -> None:
        metadata = create_session(self.config)
        model = StatusModel(terminal_hold_seconds=2.0)

        self.assertEqual(model.snapshot(self.config, now=10.0).status, "recording")
        update_metadata(metadata, status="paste_failed", errors=["paste helper failed"])

        visible = model.snapshot(self.config, now=11.0)
        hidden = model.snapshot(self.config, now=14.0)

        self.assertTrue(visible.visible)
        self.assertEqual(visible.detail, "paste helper failed")
        self.assertFalse(hidden.visible)

    def test_existing_terminal_session_is_not_shown_on_monitor_startup(self) -> None:
        metadata = create_session(self.config)
        update_metadata(metadata, status="complete")

        snapshot = StatusModel(terminal_hold_seconds=2.0).snapshot(self.config, now=10.0)

        self.assertFalse(snapshot.visible)
        self.assertEqual(snapshot.status, "complete")

    def test_display_detail_uses_spinner_for_busy_states(self) -> None:
        metadata = create_session(self.config)
        update_metadata(metadata, status="transcribing")
        snapshot = StatusModel().snapshot(self.config, now=10.0)

        self.assertIn("working on transcript", _display_detail(snapshot, mic_monitor=None, frame_index=1))
        self.assertEqual(_detail_for_status("idle"), "ready")


if __name__ == "__main__":
    unittest.main()
