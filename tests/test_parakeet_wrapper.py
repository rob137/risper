from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path


class ParakeetWrapperTests(unittest.TestCase):
    def test_missing_dependencies_fail_clearly(self) -> None:
        wrapper_path = Path(__file__).parents[1] / "scripts" / "parakeet-nemo-wrapper.py"
        spec = importlib.util.spec_from_file_location("parakeet_nemo_wrapper", wrapper_path)
        self.assertIsNotNone(spec)
        assert spec and spec.loader
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)

        with self.assertRaisesRegex(RuntimeError, "Parakeet NeMo dependencies are not installed"):
            module.transcribe("nvidia/parakeet-tdt-0.6b-v3", "/tmp/missing.wav", "en")


if __name__ == "__main__":
    unittest.main()
