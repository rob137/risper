#!/usr/bin/env python3
from __future__ import annotations

import argparse
import contextlib
import sys


INSTALL_HINT = """Parakeet NeMo dependencies are not installed.

This wrapper expects a local Python environment with NVIDIA NeMo ASR and PyTorch.
Risper did not install those automatically because they are large and
platform/GPU-sensitive.
"""


def transcribe(model_name: str, audio_path: str, language: str) -> str:
    with contextlib.redirect_stdout(sys.stderr):
        try:
            import nemo.collections.asr as nemo_asr
        except Exception as exc:
            raise RuntimeError(f"{INSTALL_HINT}\nImport error: {exc}") from exc

        model = nemo_asr.models.ASRModel.from_pretrained(model_name=model_name)
        outputs = model.transcribe([audio_path], batch_size=1)
    if not outputs:
        return ""
    first = outputs[0]
    if isinstance(first, str):
        return first
    if hasattr(first, "text"):
        return str(first.text)
    return str(first)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Risper wrapper for NVIDIA NeMo Parakeet ASR.")
    parser.add_argument("--model", default="nvidia/parakeet-tdt-0.6b-v3")
    parser.add_argument("--audio", required=True)
    parser.add_argument("--language", default="en")
    args = parser.parse_args(argv)

    try:
        text = transcribe(args.model, args.audio, args.language).strip()
    except Exception as exc:
        print(exc, file=sys.stderr)
        return 2

    if text:
        print(text)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
