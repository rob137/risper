# Models

Risper separates the dictation appliance from transcription engines.

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
selected_model = "whispercpp-base-en"
```

Profile shape:

```toml
[models.whispercpp-base-en]
engine = "whisper.cpp"
model = "base.en"
language = "en"
command = "/path/to/whisper-cli -m /path/to/model.bin -f {audio} -l {language} -nt -otxt -of {raw_no_txt}"
```

Placeholders:

```text
{audio}
{raw}
{raw_no_txt}
{clean}
{clean_no_txt}
{model}
{language}
```

Backend contract:

- The command must be local.
- The command may print transcript text to stdout.
- Or it may write `{raw}` / `{clean}` itself.
- Risper stores the selected engine, model, and language in session metadata.
- The recorder, daemon, history, and paste layers should not need edits for a new backend.

Add a profile:

```bash
risper-models add-external my-local-engine \
  --engine my-engine \
  --model my-model-name \
  --language en \
  --command "/path/to/wrapper --model {model} --audio {audio}" \
  --select
```

For any newer model family, the preferred integration is a small local wrapper command that normalizes its CLI/API to this contract.
