# Performance

Measurements on Ubuntu 24.04, AMD Ryzen 7 250, 14 GiB RAM, no NVIDIA GPU.

Command:

```bash
risper-benchmark 2026-05-06_09-12-38 \
  --profile whispercpp-base-en \
  --repeat 2
```

Results for a 15s speech session:

```text
profile                  rep     wall    cpu%   rss_mb  chars
whispercpp-base-en         1    1.239   377.5    284.8    218
whispercpp-base-en         2    1.228   380.3    284.9    218
```

Results for a 12s speech/noise session:

```text
profile                  rep     wall    cpu%   rss_mb  chars
whispercpp-base-en         1    0.913   345.6    284.2     35
```

Interpretation:

- whisper.cpp base.en is fast and light on this CPU: roughly 1s wall time and ~285 MiB peak RSS on short sessions.
- Any future engine should be benchmarked with the same command and compared against these figures before being made the default.
