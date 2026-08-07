from __future__ import annotations

import json
import os
import tempfile
import time
import unittest
from pathlib import Path
from unittest.mock import patch

from risper.config import Config, load_config
from risper.sessions import (
    append_event,
    all_sessions,
    create_session,
    events_path,
    find_session,
    load_session,
    mark_incomplete_recordings_recovered,
    prune_expired_audio,
    read_events,
    update_metadata,
)
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
        self.assertTrue((session_dir / "events.jsonl").exists())
        self.assertTrue((session_dir / "status.log").exists())
        self.assertTrue((session_dir / "error.log").exists())
        self.assertEqual(metadata["status"], "recording")

        persisted = json.loads((session_dir / "metadata.json").read_text(encoding="utf-8"))
        self.assertEqual(persisted["session_id"], metadata["session_id"])
        events = read_events(metadata)
        self.assertEqual(events[0]["event"], "session.created")
        self.assertEqual(events_path(metadata), session_dir / "events.jsonl")

    def test_append_event_writes_json_lines_without_transcript_payloads(self) -> None:
        metadata = create_session(self.config)

        append_event(metadata, "paste.result", ok=False, message="no installed paste helper", transcript_chars=42)

        events = read_events(metadata)
        self.assertEqual(events[-1]["event"], "paste.result")
        self.assertFalse(events[-1]["ok"])
        self.assertEqual(events[-1]["transcript_chars"], 42)
        self.assertNotIn("transcript", events[-1])

    def test_create_session_metadata_matches_config_contract(self) -> None:
        metadata = create_session(self.config)
        session_dir = Path(metadata["audio_path"]).parent

        self.assertEqual(metadata["session_id"], session_dir.name)
        self.assertEqual(metadata["audio_path"], str(session_dir / "audio.wav"))
        self.assertEqual(metadata["transcript_raw_path"], str(session_dir / "transcript.raw.txt"))
        self.assertEqual(metadata["transcript_clean_path"], str(session_dir / "transcript.clean.txt"))
        self.assertEqual(metadata["transcription_engine"], self.config.transcription_engine)
        self.assertEqual(metadata["model"], self.config.model)
        self.assertEqual(metadata["language"], self.config.language)
        self.assertFalse(metadata["paste_attempted"])
        self.assertFalse(metadata["paste_helper_succeeded"])
        self.assertFalse(metadata["paste_succeeded"])
        self.assertEqual(metadata["paste_confirmation"], "not_attempted")
        self.assertEqual(metadata["target_app"], None)
        self.assertEqual(metadata["errors"], [])
        self.assertEqual((session_dir / "error.log").read_text(encoding="utf-8"), "")

    def test_create_session_suffixes_colliding_session_ids(self) -> None:
        with patch("risper.sessions.session_id_from_now", return_value="same-id"):
            first = create_session(self.config)
            second = create_session(self.config)
            third = create_session(self.config)

        self.assertEqual(first["session_id"], "same-id")
        self.assertEqual(second["session_id"], "same-id-2")
        self.assertEqual(third["session_id"], "same-id-3")

    def test_update_metadata_is_persisted(self) -> None:
        metadata = create_session(self.config)
        update_metadata(metadata, status="complete", paste_succeeded=True)
        session_dir = Path(metadata["audio_path"]).parent

        persisted = json.loads((session_dir / "metadata.json").read_text(encoding="utf-8"))
        self.assertEqual(persisted["status"], "complete")
        self.assertTrue(persisted["paste_succeeded"])

    def test_load_session_handles_missing_and_invalid_metadata(self) -> None:
        missing = self.root / "missing-session"
        invalid = self.root / "invalid-session"
        invalid.mkdir()
        (invalid / "metadata.json").write_text("{not json", encoding="utf-8")

        self.assertIsNone(load_session(missing))
        self.assertIsNone(load_session(invalid))

    def test_all_sessions_are_newest_first_and_ignore_bad_entries(self) -> None:
        first = create_session(self.config)
        second = create_session(self.config)
        first_dir = Path(first["audio_path"]).parent
        second_dir = Path(second["audio_path"]).parent
        update_metadata(first, started_at="2026-01-01T00:00:00+00:00")
        update_metadata(second, started_at="2026-01-02T00:00:00+00:00")
        bad_dir = self.config.sessions_dir / "bad"
        bad_dir.mkdir()
        (bad_dir / "metadata.json").write_text("{bad json", encoding="utf-8")

        sessions = all_sessions(self.config)

        self.assertEqual([item["session_id"] for item in sessions], [second_dir.name, first_dir.name])

    def test_find_session_supports_last_and_exact_id(self) -> None:
        first = create_session(self.config)
        second = create_session(self.config)
        update_metadata(first, started_at="2026-01-01T00:00:00+00:00")
        update_metadata(second, started_at="2026-01-02T00:00:00+00:00")

        self.assertEqual(find_session(self.config, "last")["session_id"], second["session_id"])
        self.assertEqual(find_session(self.config, first["session_id"])["session_id"], first["session_id"])
        self.assertIsNone(find_session(self.config, "missing"))

    def _write_audio(self, metadata: dict, age_seconds: float) -> Path:
        audio = Path(metadata["audio_path"])
        audio.write_bytes(b"RIFF")
        stamp = time.time() - age_seconds
        os.utime(audio, (stamp, stamp))
        return audio

    def test_prune_keeps_everything_when_retention_is_never(self) -> None:
        metadata = create_session(self.config)
        audio = self._write_audio(metadata, age_seconds=400 * 86400)

        self.assertEqual(prune_expired_audio(self.config), 0)
        self.assertTrue(audio.exists())

    def test_prune_deletes_expired_audio_and_keeps_transcripts(self) -> None:
        config = self._config_with_retention("7d")
        old = create_session(config)
        recent = create_session(config)
        old_audio = self._write_audio(old, age_seconds=8 * 86400)
        recent_audio = self._write_audio(recent, age_seconds=6 * 86400)
        transcript = Path(old["transcript_clean_path"])
        transcript.write_text("kept", encoding="utf-8")

        self.assertEqual(prune_expired_audio(config), 1)

        self.assertFalse(old_audio.exists())
        self.assertTrue(recent_audio.exists())
        self.assertEqual(transcript.read_text(encoding="utf-8"), "kept")
        persisted = json.loads(Path(old["audio_path"]).parent.joinpath("metadata.json").read_text(encoding="utf-8"))
        self.assertIn("audio_pruned_at", persisted)
        self.assertEqual(read_events(old)[-1]["event"], "audio.pruned")

    def test_prune_is_idempotent_and_ignores_sessions_without_audio(self) -> None:
        config = self._config_with_retention("1h")
        metadata = create_session(config)
        self._write_audio(metadata, age_seconds=2 * 3600)

        self.assertEqual(prune_expired_audio(config), 1)
        self.assertEqual(prune_expired_audio(config), 0)

    def _config_with_retention(self, value: str) -> Config:
        path = Path(self.config.config_path)
        path.write_text(
            path.read_text(encoding="utf-8").replace(
                'audio_retention = "never"', f'audio_retention = "{value}"'
            ),
            encoding="utf-8",
        )
        return load_config()

    def test_incomplete_recording_is_marked_recovered(self) -> None:
        metadata = create_session(self.config)

        count = mark_incomplete_recordings_recovered(self.config)

        self.assertEqual(count, 1)
        session_dir = Path(metadata["audio_path"]).parent
        persisted = json.loads((session_dir / "metadata.json").read_text(encoding="utf-8"))
        self.assertEqual(persisted["status"], "recovered")
        self.assertEqual(
            persisted["errors"][0],
            "Recovered incomplete recording after startup; audio may be partial.",
        )
        self.assertIsNotNone(persisted["ended_at"])

    def test_recovery_preserves_existing_errors_and_skips_finished_sessions(self) -> None:
        recording = create_session(self.config)
        complete = create_session(self.config)
        update_metadata(recording, errors=["previous error"])
        update_metadata(complete, status="complete")

        count = mark_incomplete_recordings_recovered(self.config)

        self.assertEqual(count, 1)
        recovered = json.loads(Path(recording["audio_path"]).parent.joinpath("metadata.json").read_text(encoding="utf-8"))
        finished = json.loads(Path(complete["audio_path"]).parent.joinpath("metadata.json").read_text(encoding="utf-8"))
        self.assertEqual(recovered["errors"][0], "previous error")
        self.assertEqual(
            recovered["errors"][1],
            "Recovered incomplete recording after startup; audio may be partial.",
        )
        self.assertEqual(finished["status"], "complete")
        self.assertEqual(finished["errors"], [])

    def test_recovery_handles_legacy_recording_without_errors_list(self) -> None:
        metadata = create_session(self.config)
        metadata.pop("errors")
        update_metadata(metadata)

        self.assertEqual(mark_incomplete_recordings_recovered(self.config), 1)

        recovered = json.loads(Path(metadata["audio_path"]).parent.joinpath("metadata.json").read_text(encoding="utf-8"))
        self.assertEqual(
            recovered["errors"],
            ["Recovered incomplete recording after startup; audio may be partial."],
        )


if __name__ == "__main__":
    unittest.main()
