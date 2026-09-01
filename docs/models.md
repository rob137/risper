# Models

Risper separates the dictation appliance from transcription engines. Local
transcription remains a first-class option because confidentiality is a reason
to use Risper.

The core flow only knows:

- audio path
- raw transcript path
- clean transcript path
- selected model profile

Model profiles live in:

```text
~/.config/risper/models.toml
```

Config selects one:

```toml
selected_model = "whispercpp-small-en"
```

Profile shape:

```toml
[models.whispercpp-base-en]
engine = "whisper.cpp"
model = "base.en"
language = "en"
command = "/path/to/whisper-cli -m /path/to/model.bin -f {audio} -l {language} -nt -otxt -of {raw_no_txt} -mc 0"
```

The default registry also includes a commented, opt-in OpenAI profile:

```toml
[models.openai-gpt-transcribe]
engine = "openai"
model = "gpt-transcribe"
language = "en"
api_key_file = "~/.config/openai/key"
billing_price_per_minute = 0.0045
billing_currency = "USD"
fallback_profile = "whispercpp-small-en"
fallback_timeout_seconds = 15
prompt = "A short comma-separated list of names and terms to recognize."
```

An `openai` profile may omit `command`; the engine reads `api_key_file` and
calls `/v1/audio/transcriptions`. If `api_key_file` is omitted, it defaults to
`~/.config/openai/key`. Keep that file outside `models.toml`, mode `0600`, and
never commit it. Verify which OpenAI project and organisation the key belongs
to before use. Audio and the profile prompt leave the machine, and requests
are chargeable; cloud transcription is deliberately never selected by the
generated defaults.

The transcription endpoint does not return a model's price per minute. Risper
therefore reads `billing_price_per_minute` and `billing_currency` from the
profile, stores a snapshot on each newly completed OpenAI session, and labels
the popup totals as estimates. The known `gpt-transcribe` model defaults to the
published USD 0.0045/minute price checked on 2026-09-01 so existing registries
need no migration. Set both fields explicitly to override that assumption.
Historical currencies remain separate; Risper does not invent an exchange
rate.

An OpenAI profile may name a `whisper.cpp` `fallback_profile` and set the
positive `fallback_timeout_seconds` allowed for the cloud attempt. The default
budget is 15 seconds. Without an explicit fallback, selection prefers the
canonical `whispercpp-small-en` profile, another configured `small.en` profile,
the configured voice-trigger profile, then the first whisper.cpp profile in
sorted order. Explicit missing or non-local fallbacks are configuration errors.
Fallback covers key, transport, HTTP, timeout, and response failures; it does
not rerun missing input audio or a local transcript-write failure.

Placeholders:

```text
{audio}
{raw}
{raw_no_txt}
{clean}
{clean_no_txt}
{model}
{language}
{prompt}
```

Backend contract:

- A command profile is executed locally. Its backend may still be a remote
  service if the user explicitly chooses that profile.
- The command may print transcript text to stdout.
- Or it may write `{raw}` / `{clean}` itself.
- Risper stores the engine, model, and language that actually produced the
  transcript in session metadata.
- The recorder, daemon, history, and paste layers should not need edits for a new backend.

The `prompt` field is intended for short vocabulary guidance such as proper
nouns and jargon. The OpenAI transcription API supports prompt guidance for
the applicable models; see the [transcription API reference](https://developers.openai.com/api/reference/resources/audio/subresources/transcriptions/methods/create).
Do not put secrets in prompts. Voice triggers are separate and require a local
`whisper.cpp` profile.

Add a profile:

```bash
risper models add-external my-local-engine \
  --engine my-engine \
  --model my-model-name \
  --language en \
  --command "/path/to/wrapper --model {model} --audio {audio}" \
  --select
```

For any newer local model family, the preferred integration is a small local
wrapper command that normalizes its CLI to this contract. For OpenAI, use the
built-in `openai` engine so key handling and response parsing do not depend on
curl, jq, or shell interpolation.

Current model pricing and limits are maintained in the [OpenAI GPT-Transcribe
model documentation](https://developers.openai.com/api/docs/models/gpt-transcribe)
and the [OpenAI API pricing page](https://developers.openai.com/api/docs/pricing/);
check those pages before using cloud transcription regularly.

Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
