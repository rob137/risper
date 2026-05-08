# Performance

Measurements on Ubuntu 24.04, AMD Ryzen 7 250, 14 GiB RAM, no NVIDIA GPU.

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

Result on 2026-05-08 after selecting Parakeet for live use:

```text
profile                  rep     wall    cpu%   rss_mb  chars
parakeet-tdt-0-6b-v3       1   27.873   127.6   5746.5     64
```

Interpretation:

- whisper.cpp is still the fast option.
- Parakeet works on CPU, but process-per-dictation is expensive: roughly 19-28s wall time and 5.7-5.9 GiB peak RSS.
- The laptop is not unusable during the benchmark, but Parakeet is heavy enough that live dictation has a long processing wait unless a persistent worker or smaller/faster runtime path is added.
- Parakeet is currently selected on this laptop for quality testing.

Likely next optimization:

- Keep a persistent transcription worker alive so NeMo and the Parakeet model load once.
- Or find a lighter Parakeet-compatible runtime, such as an ONNX/TensorRT/exported path if practical on this hardware.
