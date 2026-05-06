from __future__ import annotations

import os
import tempfile
import unittest
from pathlib import Path

from helpers import write_test_config
from risper.benchmark import _run_profile
from risper.config import load_config
from risper.models import ModelProfile


class BenchmarkTests(unittest.TestCase):
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

    def test_run_profile_reports_timing_and_transcript(self) -> None:
        audio = self.root / "audio.wav"
        script = self.root / "transcribe.py"
        audio.write_bytes(b"fake audio")
        script.write_text("print('bench transcript')\n", encoding="utf-8")
        profile = ModelProfile("bench", "test", "test-model", "en", f"/usr/bin/python3 {script}")

        result = _run_profile(profile, audio)

        self.assertEqual(result["returncode"], 0)
        self.assertEqual(result["transcript_preview"], "bench transcript")
        self.assertGreaterEqual(result["elapsed_seconds"], 0)
        self.assertIn("cpu_percent", result)
        self.assertIn("max_rss_mb", result)


if __name__ == "__main__":
    unittest.main()
