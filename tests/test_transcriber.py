from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path

from risper.config import load_config
from risper.models import ModelProfile, write_profile
from risper.transcriber import TranscriptionUnavailable, transcribe
from helpers import write_test_config


class TranscriberTests(unittest.TestCase):
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
        self.audio = self.root / "audio.wav"
        self.raw = self.root / "transcript.raw.txt"
        self.clean = self.root / "transcript.clean.txt"
        self.audio.write_bytes(b"not real audio for unit tests")

    def tearDown(self) -> None:
        for key, value in self.old_env.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value
        self.tempdir.cleanup()

    def _script(self, body: str) -> Path:
        script = self.root / "helper.py"
        script.write_text(body, encoding="utf-8")
        return script

    def test_stdout_transcript_is_written_to_raw_and_clean(self) -> None:
        script = self._script("print('hello from stdout')\n")
        profile = ModelProfile(
            id="stdout",
            engine="test",
            model="test-model",
            language="en",
            command=f"/usr/bin/python3 {script}",
        )

        text = transcribe(self.config, self.audio, self.raw, self.clean, profile)

        self.assertEqual(text, "hello from stdout")
        self.assertEqual(self.raw.read_text(encoding="utf-8"), "hello from stdout\n")
        self.assertEqual(self.clean.read_text(encoding="utf-8"), "hello from stdout\n")

    def test_raw_file_written_by_backend_is_preserved_and_copied_to_clean(self) -> None:
        script = self._script(
            "import sys\n"
            "from pathlib import Path\n"
            "Path(sys.argv[1]).write_text('raw backend text\\n', encoding='utf-8')\n"
        )
        profile = ModelProfile(
            id="raw-writer",
            engine="test",
            model="test-model",
            language="en",
            command=f"/usr/bin/python3 {script} {{raw}}",
        )

        text = transcribe(self.config, self.audio, self.raw, self.clean, profile)

        self.assertEqual(text, "raw backend text")
        self.assertEqual(self.raw.read_text(encoding="utf-8"), "raw backend text\n")
        self.assertEqual(self.clean.read_text(encoding="utf-8"), "raw backend text\n")

    def test_clean_file_written_by_backend_is_preserved_and_copied_to_raw(self) -> None:
        script = self._script(
            "import sys\n"
            "from pathlib import Path\n"
            "Path(sys.argv[1]).write_text('clean backend text\\n', encoding='utf-8')\n"
        )
        profile = ModelProfile(
            id="clean-writer",
            engine="test",
            model="test-model",
            language="en",
            command=f"/usr/bin/python3 {script} {{clean}}",
        )

        text = transcribe(self.config, self.audio, self.raw, self.clean, profile)

        self.assertEqual(text, "clean backend text")
        self.assertEqual(self.clean.read_text(encoding="utf-8"), "clean backend text\n")
        self.assertEqual(self.raw.read_text(encoding="utf-8"), "clean backend text\n")

    def test_stdout_replaces_existing_raw_transcript(self) -> None:
        self.raw.write_text("old transcript\n", encoding="utf-8")
        self.clean.write_text("old transcript\n", encoding="utf-8")
        script = self._script("print('new transcript')\n")
        profile = ModelProfile(
            id="stdout",
            engine="test",
            model="test-model",
            language="en",
            command=f"/usr/bin/python3 {script}",
        )

        text = transcribe(self.config, self.audio, self.raw, self.clean, profile)

        self.assertEqual(text, "new transcript")
        self.assertEqual(self.raw.read_text(encoding="utf-8"), "new transcript\n")
        self.assertEqual(self.clean.read_text(encoding="utf-8"), "new transcript\n")

    def test_command_receives_all_placeholders(self) -> None:
        script = self._script(
            "import sys\n"
            "from pathlib import Path\n"
            "Path(sys.argv[2]).write_text('\\n'.join(sys.argv[1:]) + '\\n', encoding='utf-8')\n"
            "print('placeholder transcript')\n"
        )
        args_path = self.root / "args.txt"
        profile = ModelProfile(
            id="placeholders",
            engine="test",
            model="model-name",
            language="cy",
            command=(
                f"/usr/bin/python3 {script} "
                f"{{audio}} {args_path} {{raw}} {{raw_no_txt}} {{clean}} {{clean_no_txt}} {{model}} {{language}}"
            ),
        )

        text = transcribe(self.config, self.audio, self.raw, self.clean, profile)

        args = args_path.read_text(encoding="utf-8").splitlines()
        self.assertEqual(text, "placeholder transcript")
        self.assertEqual(args[0], str(self.audio))
        self.assertEqual(args[2], str(self.raw))
        self.assertEqual(args[3], str(self.raw.with_suffix("")))
        self.assertEqual(args[4], str(self.clean))
        self.assertEqual(args[5], str(self.clean.with_suffix("")))
        self.assertEqual(args[6], "model-name")
        self.assertEqual(args[7], "cy")

    def test_uses_active_profile_when_profile_is_not_supplied(self) -> None:
        script = self._script("print('selected profile transcript')\n")
        write_profile(
            self.config,
            ModelProfile(
                id="selected",
                engine="test",
                model="test-model",
                language="en",
                command=f"/usr/bin/python3 {script}",
            ),
            select=True,
        )

        text = transcribe(load_config(), self.audio, self.raw, self.clean)

        self.assertEqual(text, "selected profile transcript")

    def test_expands_tilde_for_single_executable_command(self) -> None:
        home = self.root / "home"
        bin_dir = home / "bin"
        bin_dir.mkdir(parents=True)
        script = bin_dir / "risper-test-transcriber"
        script.write_text("#!/usr/bin/python3\nprint('home command transcript')\n", encoding="utf-8")
        script.chmod(0o755)
        old_home = os.environ.get("HOME")
        os.environ["HOME"] = str(home)
        profile = ModelProfile(
            id="home-command",
            engine="test",
            model="test-model",
            language="en",
            command="~/bin/risper-test-transcriber",
        )

        try:
            text = transcribe(self.config, self.audio, self.raw, self.clean, profile)
        finally:
            if old_home is None:
                os.environ.pop("HOME", None)
            else:
                os.environ["HOME"] = old_home

        self.assertEqual(text, "home command transcript")

    def test_whitespace_only_stdout_without_files_raises(self) -> None:
        script = self._script("print('   ')\n")
        profile = ModelProfile("blank", "test", "test-model", "en", f"/usr/bin/python3 {script}")

        with self.assertRaises(TranscriptionUnavailable):
            transcribe(self.config, self.audio, self.raw, self.clean, profile)

    def test_command_failure_is_not_hidden(self) -> None:
        script = self._script("import sys\nsys.stderr.write('backend exploded\\n')\nraise SystemExit(7)\n")
        profile = ModelProfile("failing", "test", "test-model", "en", f"/usr/bin/python3 {script}")

        with self.assertRaises(subprocess.CalledProcessError) as raised:
            transcribe(self.config, self.audio, self.raw, self.clean, profile)

        self.assertEqual(raised.exception.returncode, 7)
        self.assertEqual(raised.exception.stderr, "backend exploded\n")

    def test_empty_backend_output_raises(self) -> None:
        script = self._script("")
        profile = ModelProfile("empty", "test", "test-model", "en", f"/usr/bin/python3 {script}")

        with self.assertRaises(TranscriptionUnavailable):
            transcribe(self.config, self.audio, self.raw, self.clean, profile)


if __name__ == "__main__":
    unittest.main()
