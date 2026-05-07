from __future__ import annotations

import contextlib
import io
import os
import tempfile
import unittest
from pathlib import Path

from helpers import write_test_config
from risper.config import load_config
from risper.diagnose import main
from risper.sessions import append_event, create_session, update_metadata


class DiagnoseTests(unittest.TestCase):
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

    def test_session_diagnosis_summarizes_latest_session_without_transcript_text(self) -> None:
        metadata = create_session(self.config)
        update_metadata(metadata, status="paste_failed", paste_attempted=True, paste_succeeded=False)
        Path(str(metadata["transcript_clean_path"])).write_text("private dictated text", encoding="utf-8")
        append_event(metadata, "paste.result", ok=False, message="no installed paste helper")
        output = io.StringIO()

        with contextlib.redirect_stdout(output):
            code = main(["last"])

        text = output.getvalue()
        self.assertEqual(code, 0)
        self.assertIn("Risper session diagnosis", text)
        self.assertIn("paste_failed", text)
        self.assertIn("paste.result", text)
        self.assertIn("no installed paste helper", text)
        self.assertNotIn("private dictated text", text)


if __name__ == "__main__":
    unittest.main()
