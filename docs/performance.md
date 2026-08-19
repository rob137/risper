# Performance

Measurements on Ubuntu 24.04, AMD Ryzen 7 250 (8 cores / 16 threads), 46 GiB RAM, no NVIDIA GPU.

Command:

```bash
risper benchmark <session-id> \
  --profile whispercpp-base-en --profile whispercpp-small-en \
  --repeat 2
```

Results (2026-07-16, both profiles running with `-t 8`):

```text
session          profile                  wall    cpu%   rss_mb
7.3s speech      whispercpp-base-en      ~1.0s   ~690     ~304
7.3s speech      whispercpp-small-en     ~3.4s   ~721     ~771
22.2s speech     whispercpp-base-en      ~1.3s   ~731     ~305
22.2s speech     whispercpp-small-en     ~4.1s   ~737     ~772
52.8s speech     whispercpp-base-en      ~2.7s   ~772     ~339
52.8s speech     whispercpp-small-en     ~8.6s   ~766     ~808
```

Interpretation:

- `-t 8` matters: whisper-cli defaults to 4 threads, which left half the physical cores idle (~380% CPU in earlier runs). With `-t 8` both models saturate around 700-770%.
- small.en is roughly 3x slower and ~2.5x the RSS of base.en, but on a 53s real dictation it fixed three meaning-level errors base.en made ("coal face" vs "cold face", an entirely garbled clause, "slightly" vs "slowly"). small.en is the selected default for that reason.
- base.en stays registered as the fast fallback profile.
- Any future engine should be benchmarked with the same command and compared against these figures before being made the default.
