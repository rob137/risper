# Performance

Measurements on 2026-05-06, Ubuntu 24.04, AMD Ryzen 7 250, 14 GiB RAM, no NVIDIA GPU.

Command:

```bash
risper-benchmark 2026-05-06_09-12-38 \
  --profile whispercpp-base-en \
  --profile parakeet-tdt-0-6b-v3 \
  --repeat 2
```

Results for a 15s speech session:

```text
profile                  rep     wall    cpu%   rss_mb  chars
whispercpp-base-en         1    1.239   377.5    284.8    218
whispercpp-base-en         2    1.228   380.3    284.9    218
parakeet-tdt-0-6b-v3       1   27.529   140.9   5713.1    229
parakeet-tdt-0-6b-v3       2   19.319   158.6   5918.3    229
```

Results for a 12s speech/noise session:

```text
profile                  rep     wall    cpu%   rss_mb  chars
whispercpp-base-en         1    0.913   345.6    284.2     35
parakeet-tdt-0-6b-v3       1   18.607   149.5   5923.9    120
```

Interpretation:

- whisper.cpp is currently the practical default for fast dictation.
- Parakeet works on CPU, but process-per-dictation is expensive: roughly 19-28s wall time and 5.7-5.9 GiB peak RSS.
- The laptop is not unusable during the benchmark, but Parakeet is heavy enough that it should not be the default without a persistent worker or a smaller/faster runtime path.
- Parakeet quality may still be worth it for optional retranscription.

Likely next optimization:

- Keep a persistent transcription worker alive so NeMo and the Parakeet model load once.
- Or find a lighter Parakeet-compatible runtime, such as an ONNX/TensorRT/exported path if practical on this hardware.
