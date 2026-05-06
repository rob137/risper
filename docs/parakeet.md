# Parakeet

Parakeet here means NVIDIA NeMo Parakeet ASR models. The current profile template targets:

```text
nvidia/parakeet-tdt-0.6b-v3
```

Risper includes:

- `scripts/parakeet-nemo-wrapper.py`
- `scripts/add-parakeet-profile.sh`

Add the profile:

```bash
cd ~/personal/risper
./scripts/add-parakeet-profile.sh
risper-models list
```

Installed local environment:

```text
~/.local/share/risper/engines/parakeet-nemo/venv
```

Current status on this laptop:

- NeMo ASR imports successfully.
- `nvidia/parakeet-tdt-0.6b-v3` is cached by Hugging Face.
- The Parakeet profile is selected.
- Inference runs on CPU because there is no NVIDIA GPU.
- Startup/model load is noticeably slower than whisper.cpp, but transcription works.
- Benchmarks show roughly 19-28s wall time and 5.7-5.9 GiB peak RSS per process on short saved sessions. See `docs/performance.md`.

Select Parakeet:

```bash
risper-models select parakeet-tdt-0-6b-v3
```

Switch back to whisper.cpp:

```bash
risper-models select whispercpp-base-en
```

The wrapper contract is the same as every Risper backend: print transcript text to stdout, and Risper writes the session transcript files.

NeMo/PyTorch installation is large and platform-sensitive. On this AMD laptop it installed a CUDA-flavoured Torch wheel set even though CUDA is not available; it is isolated to the Parakeet engine directory.
