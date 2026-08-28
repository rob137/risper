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

## Optional cloud comparison

An authored synthetic 10.05s clip was sent through seven OpenAI transcription
calls during the August 2026 investigation, including a final call through the
native Go engine. End-to-end calls took about
0.90-1.79s. `gpt-transcribe` kept the tested proper nouns stable; the tested
`gpt-4o-mini-transcribe` calls varied more. The two directly inspected
`gpt-transcribe` responses each reported 11 seconds of duration usage for the
clip. At the published price on 2026-08-28 (`$0.0045` per minute), that is about
`$0.000825` per call. These are directional observations, not a latency
guarantee: the clip was synthetic, the sample is small, and network/API
conditions vary.

The corresponding local synthetic run took about 40s, which is anomalous
against the real-speech baselines above and should not be used as a general
local-versus-cloud conclusion. Repeat the comparison with representative
speech, the same saved audio, and several runs. Include upload time, API
round-trip, model selection, and current published prices in the decision.
Cloud mode is opt-in and sends the recording away from the machine; local
whisper.cpp remains the confidentiality-preserving choice. See the [OpenAI
GPT-Transcribe model documentation](https://developers.openai.com/api/docs/models/gpt-transcribe)
and [API pricing](https://developers.openai.com/api/docs/pricing/) for current
values.

Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
