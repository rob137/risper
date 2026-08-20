# Voice triggers

Voice triggers are an optional Linux daemon feature. They are disabled by
default:

```toml
voice_triggers_enabled = false
voice_start_word = "quasar"
voice_stop_word = "marzipan"
voice_send_word = "tangerine"
voice_trigger_profile = "whispercpp-base-en"
voice_noise_gate_db = 10.0
voice_silence_ms = 400
```

The three words are examples, not a recommendation that Rob has to keep. They
are deliberately uncommon, not sentence-ending words, and avoid radio or
military phrasing. Change all three to words that are absent from normal
dictation, and keep them distinct. Risper normalizes them to one lowercase
word; invalid or duplicate values fall back to distinct defaults.

When enabled, the daemon opens a second, microphone-only `pw-record` stream.
It does not read the mixed recording or the system monitor, so speaker output
cannot fire a trigger through a direct monitor path. The internal laptop
microphone can still hear speakers acoustically; the boom headset is the
better isolation case. The stream is not put in a session folder and is not
written to a WAV file: short PCM bursts are held in memory, sent to the
configured whisper.cpp profile over stdin, and discarded. `audio_retention`
therefore does not need a special voice-audio rule.

The listener calibrates a rolling quiet level and only considers a burst when
it is above that level by `voice_noise_gate_db`. This is relative rather than
an absolute dBFS threshold because the BH-71 DSP gate and the laptop gain path
produce materially different absolute levels. A burst ends after
`voice_silence_ms` of quiet. Candidate audio is capped at 2.5 seconds; if the
gate stays open beyond that, the listener discards the burst and waits for
quiet instead of repeatedly recognizing sustained speech. The fast `base.en`
profile is the default because the selected `small.en` profile is for
dictation accuracy, not control latency.

The trigger path uses the selected profile for its model and whisper.cpp
command, but does not inherit the profile's dictation prompt or decode
settings. It replaces the prompt with only the three configured trigger words
(`Trigger words: quasar, marzipan, tangerine.` in the defaults) and forces
greedy single-word decoding with `-bs 1 -bo 1`. The common whisper.cpp command
renderer also supplies `-mc 0` when the profile does not already set it. These
overrides apply only to trigger recognition; ordinary dictation keeps its
profile prompt and settings.

Say the start word by itself to run ordinary `risper-toggle`. While recording,
the stop word runs `risper-toggle --paste --voice-stop`; the send word runs
`risper-toggle --paste --enter --voice-send`. The spoken stop/send word is
removed only when it is the final word of the completed transcript, so it is
not pasted or submitted. If the command fails or paste is unavailable, the
existing recoverable session and clipboard behavior still applies.

The practical latency is the end of the word, the configured silence window,
and one `base.en` recognition pass. On Rob's machine, with `-t 8`, a 0.8s
single-word burst took about 0.49–0.52s wall time and 3.1–3.2s CPU with the
trigger settings. The default decoder took about 0.52s on one slice but 1.39s
on another; the expensive case is content-dependent because whisper pads each
invocation to a 30s window and can search much longer on near-empty audio.
The old full dictation prompt added roughly 20–25% in the second measurement
and reached 1.47s wall / 10.5s CPU in the first. The trigger prompt and greedy
settings therefore make the existing architecture as predictable as it can be,
but there is still a roughly 0.5s per-invocation floor: feeding a shorter burst
does not remove whisper's 30s padding. With the default 400ms silence window,
roughly 0.9s end-to-end is the honest target. This is not a dedicated
always-loaded wake-word engine, so a ~0.2s target remains a separate future
decision rather than a promise here.

The `transcription_start` desktop sound starts when the stop-to-transcribe
path begins but no longer blocks recognition, clipboard copy, or paste. The
toggle waits for its helper only before the process exits, so the sound cannot
outlive a toggle-cycle test's temporary directory. The recording-start and
success sounds remain synchronous because they only affect process exit, not
the stop-to-paste pipeline.

Enabling the feature requires the configured voice profile to exist in
`models.toml`, `pw-record` to be available, and the daemon to be restarted.
The source checkout is not live until the normal review/install process; this
change does not run `install-user.sh`.

Deliberate exclusions:

- Voice triggers do not turn themselves on, change `audio_retention`, or add
  a persistent pre-recording history.
- The listener does not use the computer-output monitor, even though normal
  dictation transcription still uses the mixed session audio.
- Final trigger-word selection and whether to enable the feature remain
  Rob's choices.
