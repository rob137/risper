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

    def test_run_profile_uses_backend_raw_file_before_stdout(self) -> None:
        audio = self.root / "audio.wav"
        script = self.root / "transcribe.py"
        audio.write_bytes(b"fake audio")
        script.write_text(
            "import sys\n"
            "from pathlib import Path\n"
            "Path(sys.argv[1]).write_text('raw transcript from file\\n', encoding='utf-8')\n"
            "print('stdout transcript')\n",
            encoding="utf-8",
        )
        profile = ModelProfile("bench", "test", "test-model", "en", f"/usr/bin/python3 {script} {{raw}}")

        result = _run_profile(profile, audio)

        self.assertEqual(result["returncode"], 0)
        self.assertEqual(result["transcript_chars"], len("raw transcript from file"))
        self.assertEqual(result["transcript_preview"], "raw transcript from file")

    def test_run_profile_uses_backend_clean_file_when_raw_missing(self) -> None:
        audio = self.root / "audio.wav"
        script = self.root / "transcribe.py"
        audio.write_bytes(b"fake audio")
        script.write_text(
            "import sys\n"
            "from pathlib import Path\n"
            "Path(sys.argv[1]).write_text('clean transcript from file\\n', encoding='utf-8')\n",
            encoding="utf-8",
        )
        profile = ModelProfile("bench", "test", "test-model", "en", f"/usr/bin/python3 {script} {{clean}}")

        result = _run_profile(profile, audio)

        self.assertEqual(result["returncode"], 0)
        self.assertEqual(result["transcript_preview"], "clean transcript from file")

    def test_run_profile_reports_failed_command_without_raising(self) -> None:
        audio = self.root / "audio.wav"
        script = self.root / "fail.py"
        audio.write_bytes(b"fake audio")
        script.write_text(
            "import sys\n"
            "sys.stderr.write('line 1\\nline 2\\nline 3\\nline 4\\nline 5\\nline 6\\n')\n"
            "raise SystemExit(9)\n",
            encoding="utf-8",
        )
        profile = ModelProfile("bench", "test-engine", "test-model", "cy", f"/usr/bin/python3 {script}")

        result = _run_profile(profile, audio)

        self.assertEqual(result["profile"], "bench")
        self.assertEqual(result["engine"], "test-engine")
        self.assertEqual(result["model"], "test-model")
        self.assertEqual(result["returncode"], 9)
        self.assertEqual(result["stderr_tail"], ["line 2", "line 3", "line 4", "line 5", "line 6"])


if __name__ == "__main__":
    unittest.main()
