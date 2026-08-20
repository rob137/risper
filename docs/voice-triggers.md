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

Say the start word by itself to run ordinary `risper-toggle`. While recording,
the stop word runs `risper-toggle --paste --voice-stop`; the send word runs
`risper-toggle --paste --enter --voice-send`. The spoken stop/send word is
removed only when it is the final word of the completed transcript, so it is
not pasted or submitted. If the command fails or paste is unavailable, the
existing recoverable session and clipboard behavior still applies.

The practical latency is the end of the word, the configured silence window,
and one short `base.en` recognition pass. This avoids continuously polling
whisper over a rolling window. It is not a dedicated always-loaded wake-word
engine, so “instant” remains a measured target rather than a promise.

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
