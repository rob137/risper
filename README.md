# Risper

Risper is a local-first Ubuntu dictation utility built around durable session folders. Recording creates the folder and metadata before audio capture starts, and failures leave recoverable files behind.

The Go rewrite is complete. Go owns recording, mixing, local and opt-in OpenAI
transcription, clipboard copy, notifications, sounds, the command surface,
daemon recovery, audio retention, and the optional Linux Double Alt and
voice-trigger listeners.

## Commands

- `risper`: enables autostart and starts the user daemon. `risper kill` stops it temporarily.
- `risper toggle`: starts recording, stops it on the next run, or cancels an active transcription. Transcription includes the microphone and computer-output capture. `--paste` replays `paste_keys` into the focused window afterwards, and `--paste --enter` follows it with Return.
- `risper history`: lists recent sessions and opens, plays, copies, retranscribes, deletes, or prunes them.
- `risper open`: opens recordings, the last session, the last transcript, the last audio file, or the config.
- `risper retranscribe`: retranscribes a saved session from its mixed audio.
- `risper models`: lists, selects, and adds transcription model profiles.
- `risper benchmark`: measures transcription profile wall time, CPU use, and peak RSS.
- `risper diagnose`: prints environment or session diagnostics.
- `risper paste-test`: copies a marker to the clipboard so clipboard access can be checked manually.
- `risper daemon`: runs the daemon in the foreground.

The installed compatibility commands are thin wrappers over those subcommands:
`risper-toggle`, `risper-daemon`, `risper-open`, `risper-paste-test`,
`risper-history`, `risper-retranscribe`, `risper-models`, `risper-status`,
`risper-benchmark`, and `risper-diagnose`.

`risper-status` is the service-status alias. The desktop launcher opens the terminal history view; there is no separate status window in the default workflow.

## Install

```bash
cd ~/personal/risper
./install-user.sh
```

The installer builds the Go binary, installs it and the compatibility wrappers in `~/.local/bin`, installs the user service and desktop launcher, then reloads and restarts the daemon. Run it again after source changes: installed commands are compiled binaries, so edits in the checkout are not live until the next install.

It uses the existing Go toolchain and does not install dependencies or touch recordings, transcripts, or config.

For a checkout-only development run:

```bash
go run ./cmd/risper diagnose
go run ./cmd/risper toggle
go run ./cmd/risper daemon
```

## Verification

```bash
./scripts/test.sh
go test ./...
./scripts/mutation-smoke.sh
```

The mutation smoke copies the repository, breaks selected-model resolution in the copy, and expects the Go tests to fail. No general mutation runner is part of the project.

Performance measurements:

```bash
risper benchmark last --profile whispercpp-small-en
```

See `docs/performance.md` for benchmark context. The focused mutation smoke is
kept as `./scripts/mutation-smoke.sh`; it does not require a general mutation
runner.

## Configure

The first command run creates:

```text
~/.config/risper/config.toml
```

Important settings:

```toml
sessions_dir = "~/.local/share/risper/sessions"
selected_model = "whispercpp-small-en"
transcription_engine = "external"
transcription_command = ""
model = "small.en"
language = "en"
paste_mode = "clipboard_only"
play_sounds = true
double_alt_enabled = false
double_alt_window_ms = 350
paste_keys = "ctrl+v"
audio_retention = "never" # never | <count>h | <count>d | <count>w
voice_triggers_enabled = false
voice_start_word = "quasar"
voice_stop_word = "marzipan"
voice_send_word = "tangerine"
voice_trigger_profile = "whispercpp-base-en"
voice_noise_gate_db = 10.0
voice_silence_ms = 400
```

Model profiles live in:

```text
~/.config/risper/models.toml
```

Each profile describes an engine with metadata. Local command profiles can use
these placeholders:

```text
{audio} {raw} {raw_no_txt} {clean} {clean_no_txt} {model} {language} {prompt}
```

Installed whisper.cpp shape:

```toml
transcription_engine = "whisper.cpp"
transcription_command = "/home/robert-kirby/.local/share/risper/engines/whisper.cpp/build/bin/whisper-cli -m /home/robert-kirby/.local/share/risper/engines/whisper.cpp/models/ggml-small.en.bin -f {audio} -l {language} -t 8 -nt -otxt -of {raw_no_txt} -mc 0"
```

List or select profiles:

```bash
risper models list
risper models select whispercpp-small-en
```

Add another local backend profile:

```bash
risper models add-external my-engine \
  --engine some-engine \
  --model some-local-model \
  --language en \
  --command "/path/to/local-wrapper --model {model} --audio {audio}" \
  --select
```

### Optional OpenAI transcription

Risper remains local-first: whisper.cpp is still a first-class profile and the
default is not changed by this option. An OpenAI profile is an explicit choice
that sends the mixed `audio.wav` to a remote service, so do not use it for
confidential material unless that is acceptable for the recording. It also
incurs API charges and depends on network availability. The API key's account,
project, and organisation have to be checked by Rob; this repository does not
assume which OpenAI organisation owns a key.

Newly generated `models.toml` files contain a commented example. Existing
registries are never rewritten, so add the profile manually there; then select
it only when remote transcription is wanted:

```toml
[models.openai-gpt-transcribe]
engine = "openai"
model = "gpt-transcribe"
language = "en"
api_key_file = "~/.config/openai/key"
prompt = "A short comma-separated list of names and terms to recognize."
```

The key file must remain outside the registry, be readable only by Rob
(normally mode `0600`), and never be committed. The OpenAI engine reads it
locally; do not put the key itself in TOML, an environment dump, or a command
line. The endpoint accepts multipart audio, model, language, and prompt fields
and returns transcript text in its response; see the [OpenAI transcription API
reference](https://developers.openai.com/api/reference/resources/audio/subresources/transcriptions/methods/create).
The `gpt-transcribe` model's current published price is listed in the [OpenAI
model documentation](https://developers.openai.com/api/docs/models/gpt-transcribe)
and can change, so check it before deciding whether the latency is worth the
cost.

OpenAI's current data-control documentation says API inputs are not used to
train models unless the account opts in, and lists no abuse-monitoring or
application-state retention for `/v1/audio/transcriptions`. That is still a
remote disclosure boundary, and the company's actual project settings and
policy need verification. See [Data controls in the OpenAI
platform](https://developers.openai.com/api/docs/guides/your-data).

The profile prompt is useful for proper nouns and jargon, but keep it short and
review it as shared with the remote service. Voice triggers remain local-only:
`voice_trigger_profile` must point to a `whisper.cpp` profile.

To compare an opt-in remote profile, benchmark the same saved audio with both
profiles. A benchmark measures the end-to-end local process and network call,
not just model compute; repeat it over representative real speech before
changing the selected profile. See [docs/performance.md](docs/performance.md).

To install or refresh whisper.cpp locally:

```bash
cd ~/personal/risper
./scripts/install-whispercpp.sh small.en
```

## Data

Sessions are stored as:

```text
~/.local/share/risper/sessions/
  2026-05-06_12-00-00/
    audio.wav
    audio.mic.wav
    audio.system.wav
    transcript.raw.txt
    transcript.clean.txt
    metadata.json
    events.jsonl
    status.log
    error.log
    pw-record.log
```

Every Go recording captures microphone and computer output separately, then blends the sources into `audio.wav`. Transcription always reads that mixed file, while `audio.mic.wav` and `audio.system.wav` remain available for a later re-read. If one source is silent, it is dropped from the mix, so it adds no transcription content or cost. Audio remains until `audio_retention` prunes it.

`events.jsonl` is the structured debugging trail. It records workflow boundaries without storing full transcript text by default. Transcripts and metadata are retained indefinitely.

Inspect the latest session without dumping transcript contents:

```bash
risper diagnose last
```

## Shortcut

Bind this command in GNOME Settings, Keyboard, View and Customize Shortcuts, Custom Shortcuts:

```text
risper-toggle
```

Both sources are captured and included for every recording; the ordinary `risper-toggle` shortcut is all that is needed.

Double Alt is an optional Linux input-event listener in `risper-daemon`, disabled by default. It needs read access to `/dev/input/event*`; see `docs/double-alt.md`.

Double Alt runs `risper-toggle --paste`, which replays `paste_keys` into the focused window once the transcript is copied. Holding Shift across both taps runs `risper-toggle --paste --enter`, which follows the paste with Return. Both need `ydotoold`, and the target is whatever has focus when transcription finishes, not when the gesture was made.

Plain `risper-toggle` still only copies, so the GNOME custom shortcut remains the clipboard-only path.

Voice triggers are a separate optional Linux daemon listener. Set
`voice_triggers_enabled = true` only after choosing three uncommon,
distinct words; the defaults are placeholders for Rob to review. The listener
uses the microphone only, an adaptive relative loudness gate, and short
in-memory whisper.cpp bursts. It never listens to the mixed speaker monitor or
persists pre-recording audio; the internal laptop mic can still hear speakers
acoustically. See [docs/voice-triggers.md](docs/voice-triggers.md).

## Environment

Risper targets Rob's Ubuntu/GNOME/Wayland setup. The Linux integration uses `pw-record`, `ffmpeg`, `wl-copy`, `notify-send`, `canberra-gtk-play`, `gio`, and `ydotool` for the opt-in paste. A standalone tray/status window is deliberately outside the default workflow. Automatic paste is per-run through `--paste`, which the Double Alt gestures pass; completed transcripts remain on the clipboard whether or not the paste lands.

## Uninstall

```bash
cd ~/personal/risper
./uninstall-user.sh
```

Uninstall keeps config, state, recordings, and transcripts.

Codex gpt-5.6-sol, xhigh, prompted by Robert Kirby
