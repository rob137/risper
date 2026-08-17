from __future__ import annotations

import json
import os
import signal
import subprocess
import tempfile
import unittest
from contextlib import ExitStack, contextmanager
from pathlib import Path
from unittest.mock import patch

from helpers import write_test_config
from risper.config import load_config
from risper.recorder import current_recording, start_recording, stop_recording
from risper.recorders import MIC, SYSTEM, PipeWireRecorderBackend, mix_sources
from risper.sessions import read_events


class FakeProcess:
    def __init__(self, pid: int) -> None:
        self.pid = pid


class FakeBackend:
    name = "fake-record"
    supported_sources = (MIC, SYSTEM)

    def __init__(self) -> None:
        self.started: list[tuple[str, Path]] = []
        self.stopped: list[int] = []
        self.stop_calls: list[list[int]] = []
        self.alive: set[int] = set()
        self.fail_on: str | None = None
        self.writes: dict[str, bytes] = {}

    def available(self) -> bool:
        return True

    def log_name(self, source: str) -> str:
        return "pw-record.log" if source == MIC else f"pw-record.{source}.log"

    def start(self, source: str, audio_path: Path, stderr_path: Path) -> FakeProcess:
        if source == self.fail_on:
            raise OSError("could not spawn")
        self.started.append((source, audio_path))
        audio_path.write_bytes(self.writes.get(source, b"x" * 4000))
        pid = 5000 + len(self.started)
        self.alive.add(pid)
        return FakeProcess(pid)

    def stop_all(self, pids) -> None:
        pids = list(pids)
        self.stop_calls.append(pids)
        for pid in pids:
            self.stopped.append(pid)
            self.alive.discard(pid)

    def stop(self, pid: int) -> None:
        self.stop_all([pid])


class RecorderTestCase(unittest.TestCase):
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
        self.backend = FakeBackend()

    def tearDown(self) -> None:
        for key, value in self.old_env.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value
        self.tempdir.cleanup()

    @contextmanager
    def patched(self, mixer: bool = True):
        with ExitStack() as stack:
            stack.enter_context(patch("risper.recorder.default_recorder_backend", return_value=self.backend))
            stack.enter_context(patch("risper.recorder.mixer_available", return_value=mixer))
            stack.enter_context(
                patch("risper.recorder.pid_alive", side_effect=lambda pid: pid in self.backend.alive)
            )
            yield

    def start(self, sources, mixer: bool = True):
        with self.patched(mixer):
            return start_recording(self.config, sources)

    def stop(self, state, mix=None):
        with self.patched():
            if mix is None:
                return stop_recording(self.config, state)
            with patch("risper.recorder.mix_sources", side_effect=mix):
                return stop_recording(self.config, state)

    def metadata_for(self, state) -> dict:
        return json.loads(Path(str(state["metadata_path"])).read_text())


class StartRecordingTests(RecorderTestCase):
    def test_mic_only_writes_audio_wav_directly(self) -> None:
        state = self.start((MIC,))

        self.assertEqual(state["sources"], [MIC])
        self.assertEqual(list(state["part_paths"]), [MIC])
        self.assertEqual(state["part_paths"][MIC], state["audio_path"])
        self.assertEqual([source for source, _ in self.backend.started], [MIC])
        self.assertEqual(self.metadata_for(state)["audio_sources"], [MIC])

    def test_two_sources_capture_into_separate_parts(self) -> None:
        state = self.start((MIC, SYSTEM))

        audio_path = Path(str(state["audio_path"]))
        self.assertEqual(state["sources"], [MIC, SYSTEM])
        self.assertEqual(
            state["part_paths"],
            {
                MIC: str(audio_path.with_suffix(".mic.wav")),
                SYSTEM: str(audio_path.with_suffix(".system.wav")),
            },
        )
        self.assertNotIn(str(audio_path), state["part_paths"].values())
        self.assertEqual(len(state["recorder_pids"]), 2)

    def test_two_sources_refuse_to_start_without_a_mixer(self) -> None:
        with self.assertRaises(RuntimeError) as caught:
            self.start((MIC, SYSTEM), mixer=False)

        self.assertIn("ffmpeg", str(caught.exception))
        self.assertEqual(self.backend.started, [])

    def test_unsupported_source_is_rejected_before_a_session_is_created(self) -> None:
        with self.assertRaises(RuntimeError) as caught:
            self.start((MIC, "radio"))

        self.assertIn("radio", str(caught.exception))
        self.assertEqual(list(self.config.sessions_dir.iterdir()), [])

    def test_a_failed_second_source_stops_the_first(self) -> None:
        self.backend.fail_on = SYSTEM

        with self.assertRaises(OSError):
            self.start((MIC, SYSTEM))

        self.assertEqual(len(self.backend.stopped), 1)
        self.assertFalse(self.config.current_state_path.exists())


class CurrentRecordingTests(RecorderTestCase):
    def test_recording_survives_one_source_dying(self) -> None:
        state = self.start((MIC, SYSTEM))
        self.backend.alive.discard(state["recorder_pids"][SYSTEM])

        with self.patched():
            self.assertIsNotNone(current_recording(self.config))

    def test_recording_ends_when_every_source_is_gone(self) -> None:
        state = self.start((MIC, SYSTEM))
        self.backend.alive.clear()

        with self.patched():
            self.assertIsNone(current_recording(self.config))
        self.assertFalse(self.config.current_state_path.exists())
        self.assertIn("audio.mic.wav", str(state["part_paths"][MIC]))


class StopRecordingTests(RecorderTestCase):
    def test_every_source_is_stopped_in_one_call(self) -> None:
        state = self.start((MIC, SYSTEM))

        def fake_mix(parts, output):
            output.write_bytes(b"mixed")
            return list(parts)

        self.stop(state, mix=fake_mix)

        self.assertEqual(len(self.backend.stop_calls), 1)
        self.assertEqual(sorted(self.backend.stop_calls[0]), sorted(state["recorder_pids"].values()))

    def test_mic_only_stop_does_not_mix(self) -> None:
        state = self.start((MIC,))

        with patch("risper.recorder.mix_sources") as mix:
            metadata = self.stop(state)

        mix.assert_not_called()
        self.assertEqual(metadata["status"], "recorded")

    def test_two_sources_are_mixed_into_audio_wav_and_parts_removed(self) -> None:
        state = self.start((MIC, SYSTEM))
        audio_path = Path(str(state["audio_path"]))

        def fake_mix(parts, output):
            output.write_bytes(b"mixed")
            return list(parts)

        metadata = self.stop(state, mix=fake_mix)

        self.assertEqual(metadata["status"], "recorded")
        self.assertEqual(audio_path.read_bytes(), b"mixed")
        self.assertFalse(audio_path.with_suffix(".mic.wav").exists())
        self.assertFalse(audio_path.with_suffix(".system.wav").exists())
        mixed = [event for event in read_events(metadata) if event["event"] == "recorder.mixed"]
        self.assertEqual(mixed[-1]["used_sources"], [MIC, SYSTEM])
        self.assertEqual(mixed[-1]["dropped_sources"], [])

    def test_a_silent_source_is_dropped_and_recorded_as_an_error(self) -> None:
        state = self.start((MIC, SYSTEM))
        audio_path = Path(str(state["audio_path"]))

        def fake_mix(parts, output):
            output.write_bytes(b"mic only")
            return [parts[0]]

        metadata = self.stop(state, mix=fake_mix)

        self.assertEqual(metadata["status"], "recorded")
        self.assertEqual(audio_path.read_bytes(), b"mic only")
        self.assertTrue(any("system" in error for error in metadata["errors"]))
        mixed = [event for event in read_events(metadata) if event["event"] == "recorder.mixed"]
        self.assertEqual(mixed[-1]["dropped_sources"], [SYSTEM])

    def test_a_failed_mix_fails_the_session_and_keeps_the_parts(self) -> None:
        state = self.start((MIC, SYSTEM))
        audio_path = Path(str(state["audio_path"]))

        def fake_mix(parts, output):
            raise RuntimeError("ffmpeg exploded")

        metadata = self.stop(state, mix=fake_mix)

        self.assertEqual(metadata["status"], "failed")
        self.assertTrue(audio_path.with_suffix(".mic.wav").exists())
        self.assertTrue(audio_path.with_suffix(".system.wav").exists())
        self.assertTrue(any("ffmpeg exploded" in error for error in metadata["errors"]))


class PipeWireCommandTests(unittest.TestCase):
    def _command_for(self, source: str) -> list[str]:
        backend = PipeWireRecorderBackend()
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            with patch("risper.recorders.subprocess.Popen") as popen:
                backend.start(source, root / "audio.wav", root / "log")
            return list(popen.call_args.args[0])

    def test_mic_capture_does_not_ask_for_the_sink_monitor(self) -> None:
        command = self._command_for(MIC)

        self.assertEqual(command[0], "pw-record")
        self.assertNotIn("-P", command)
        self.assertIn("16000", command)

    def test_system_capture_binds_to_the_default_sink_monitor(self) -> None:
        command = self._command_for(SYSTEM)

        self.assertIn("-P", command)
        self.assertIn("{ stream.capture.sink=true }", command)
        self.assertNotIn("--target", command)

    def test_unknown_source_is_refused(self) -> None:
        with self.assertRaises(ValueError):
            PipeWireRecorderBackend().start("radio", Path("audio.wav"), Path("log"))

    def test_log_names_keep_mic_on_the_original_path(self) -> None:
        backend = PipeWireRecorderBackend()

        self.assertEqual(backend.log_name(MIC), "pw-record.log")
        self.assertEqual(backend.log_name(SYSTEM), "pw-record.system.log")


class PipeWireStopTests(unittest.TestCase):
    def test_all_sources_are_signalled_before_any_is_waited_on(self) -> None:
        backend = PipeWireRecorderBackend()
        calls: list[str] = []

        with (
            patch("risper.recorders.pid_alive", return_value=True),
            patch("risper.recorders.os.killpg", side_effect=lambda pid, sig: calls.append(f"signal{pid}")),
            patch("risper.recorders.wait_until", side_effect=lambda *a, **k: calls.append("wait")),
        ):
            backend.stop_all([11, 22])

        self.assertEqual(calls[:2], ["signal11", "signal22"])

    def test_a_process_that_ignores_sigint_is_terminated(self) -> None:
        backend = PipeWireRecorderBackend()
        signals: list[int] = []

        with (
            patch("risper.recorders.pid_alive", return_value=True),
            patch("risper.recorders.os.killpg", side_effect=lambda pid, sig: signals.append(sig)),
            patch("risper.recorders.wait_until"),
        ):
            backend.stop_all([11])

        self.assertEqual(signals, [signal.SIGINT, signal.SIGTERM])

    def test_a_dead_process_is_left_alone(self) -> None:
        backend = PipeWireRecorderBackend()

        with (
            patch("risper.recorders.pid_alive", return_value=False),
            patch("risper.recorders.os.killpg") as killpg,
        ):
            backend.stop_all([11])

        killpg.assert_not_called()


class MixSourcesTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def _part(self, name: str, payload: bytes) -> Path:
        path = self.root / name
        path.write_bytes(payload)
        return path

    def test_a_header_only_part_is_dropped(self) -> None:
        good = self._part("a.wav", b"x" * 4000)
        empty = self._part("b.wav", b"x" * 44)
        output = self.root / "audio.wav"

        with patch("risper.recorders.subprocess.run") as run:
            used = mix_sources([good, empty], output)

        run.assert_not_called()
        self.assertEqual(used, [good])
        self.assertEqual(output.read_bytes(), b"x" * 4000)

    def test_no_usable_part_raises(self) -> None:
        empty = self._part("a.wav", b"x" * 44)

        with self.assertRaises(RuntimeError):
            mix_sources([empty], self.root / "audio.wav")

    def test_two_usable_parts_are_mixed_with_level_normalising(self) -> None:
        first = self._part("a.wav", b"x" * 4000)
        second = self._part("b.wav", b"y" * 4000)
        output = self.root / "audio.wav"

        with patch("risper.recorders.subprocess.run") as run:
            used = mix_sources([first, second], output)

        command = list(run.call_args.args[0])
        self.assertEqual(used, [first, second])
        self.assertIn("amix=inputs=2:duration=longest:normalize=1", command)
        self.assertEqual(command.count("-i"), 2)
        self.assertEqual(command[-1], str(output))


class MixSourcesIntegrationTests(unittest.TestCase):
    """Exercises the real ffmpeg mix, because the filter string is easy to get wrong."""

    def setUp(self) -> None:
        if subprocess.run(["which", "ffmpeg"], capture_output=True).returncode != 0:
            self.skipTest("ffmpeg is not installed")
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def _tone(self, name: str, frequency: int, duration: float) -> Path:
        path = self.root / name
        subprocess.run(
            [
                "ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
                "-f", "lavfi", "-i", f"sine=frequency={frequency}:duration={duration}",
                "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", str(path),
            ],
            check=True,
        )
        return path

    def test_mixed_output_is_mono_16k_and_as_long_as_the_longer_part(self) -> None:
        short = self._tone("short.wav", 440, 1.0)
        long = self._tone("long.wav", 880, 3.0)
        output = self.root / "audio.wav"

        mix_sources([short, long], output)

        probe = subprocess.run(
            [
                "ffprobe", "-v", "error", "-show_entries",
                "stream=channels,sample_rate,duration",
                "-of", "default=nw=1:nk=1", str(output),
            ],
            check=True,
            capture_output=True,
            text=True,
        ).stdout.split()
        self.assertEqual(probe[0], "16000")
        self.assertEqual(probe[1], "1")
        self.assertAlmostEqual(float(probe[2]), 3.0, delta=0.1)


if __name__ == "__main__":
    unittest.main()
