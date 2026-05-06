from __future__ import annotations

import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from risper.session_actions import (
    copy_transcript,
    find_session_or_error,
    open_session,
    play_audio,
    transcript_path,
    transcript_preview,
)


class SessionActionTests(unittest.TestCase):
    def test_transcript_path_prefers_clean_transcript(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            raw = root / "raw.txt"
            clean = root / "clean.txt"
            raw.write_text("raw", encoding="utf-8")
            clean.write_text("clean", encoding="utf-8")

            path = transcript_path({"transcript_raw_path": str(raw), "transcript_clean_path": str(clean)})

            self.assertEqual(path, clean)

    def test_transcript_preview_falls_back_to_error(self) -> None:
        preview = transcript_preview({"errors": ["something broke loudly"]}, limit=9)

        self.assertEqual(preview, "something")

    def test_transcript_path_falls_back_to_raw_transcript(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            raw = Path(tmp) / "raw.txt"
            raw.write_text("raw", encoding="utf-8")

            self.assertEqual(transcript_path({"transcript_clean_path": "/missing", "transcript_raw_path": str(raw)}), raw)

    def test_transcript_path_returns_none_for_missing_paths(self) -> None:
        self.assertIsNone(transcript_path({"transcript_clean_path": "/missing", "transcript_raw_path": "/also-missing"}))

    def test_transcript_path_ignores_absent_metadata_keys(self) -> None:
        old_cwd = Path.cwd()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "None").write_text("should not be used", encoding="utf-8")
            os.chdir(root)
            try:
                self.assertIsNone(transcript_path({}))
            finally:
                os.chdir(old_cwd)

    def test_transcript_preview_compacts_whitespace_and_limits_text(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            clean = Path(tmp) / "clean.txt"
            clean.write_text(" hello\n\nfrom\tRisper ", encoding="utf-8")

            self.assertEqual(transcript_preview({"transcript_clean_path": str(clean)}, limit=12), "hello from R")

    def test_open_session_opens_session_directory(self) -> None:
        class FakePlatform:
            def open_path(self, path: Path) -> tuple[bool, str]:
                self.path = path
                return True, f"opened {path.name}"

        with tempfile.TemporaryDirectory() as tmp:
            audio = Path(tmp) / "audio.wav"
            fake = FakePlatform()

            with patch("risper.session_actions.current_platform", return_value=fake):
                ok, message = open_session({"audio_path": str(audio)})

            self.assertTrue(ok)
            self.assertEqual(message, f"opened {Path(tmp).name}")
            self.assertEqual(fake.path, Path(tmp))

    def test_play_audio_reports_missing_audio(self) -> None:
        ok, message = play_audio({"audio_path": "/missing/audio.wav"})

        self.assertFalse(ok)
        self.assertIn("Audio missing", message)

    def test_play_audio_opens_existing_audio(self) -> None:
        class FakePlatform:
            def open_path(self, path: Path) -> tuple[bool, str]:
                return True, f"played {path.name}"

        with tempfile.TemporaryDirectory() as tmp:
            audio = Path(tmp) / "audio.wav"
            audio.write_bytes(b"wav")

            with patch("risper.session_actions.current_platform", return_value=FakePlatform()):
                self.assertEqual(play_audio({"audio_path": str(audio)}), (True, "played audio.wav"))

    def test_copy_transcript_copies_clean_transcript(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            clean = Path(tmp) / "clean.txt"
            clean.write_text("copy me", encoding="utf-8")

            with patch("risper.session_actions.copy_text", return_value=(True, "copied")) as copy_text:
                self.assertEqual(copy_transcript({"transcript_clean_path": str(clean)}), (True, "copied"))

            copy_text.assert_called_once_with("copy me")

    def test_copy_transcript_reports_missing_transcript(self) -> None:
        self.assertEqual(copy_transcript({}), (False, "Session has no transcript."))

    def test_find_session_or_error_wraps_lookup_result(self) -> None:
        config = object()
        metadata = {"session_id": "abc"}
        with patch("risper.session_actions.find_session", return_value=metadata) as find_session:
            self.assertEqual(find_session_or_error(config, "abc"), (metadata, ""))

        find_session.assert_called_once_with(config, "abc")

    def test_find_session_or_error_reports_missing_session(self) -> None:
        config = object()
        with patch("risper.session_actions.find_session", return_value=None) as find_session:
            self.assertEqual(find_session_or_error(config, "missing"), (None, "No such session: missing"))

        find_session.assert_called_once_with(config, "missing")


if __name__ == "__main__":
    unittest.main()
