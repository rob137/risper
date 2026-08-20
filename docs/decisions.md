# Decisions

## 2026-05-06 Initial Implementation

- Project name, package, commands, config paths, and data paths use `risper`.
- Recording uses `pw-record` because it is installed and fits the GNOME Wayland/PipeWire environment.
- The first implementation did not download a transcription model or install runtime packages. After the follow-up request to continue, whisper.cpp was installed user-locally and the `base.en` model was downloaded.
- Transcription is a local external command hook pointed at whisper.cpp. This keeps the core recoverable while leaving engine choice reversible.
- Model selection is profile-based in `~/.config/risper/models.toml`. This is deliberately lighter than a settings UI and makes a future engine addable via a wrapper command.
- Desktop integration is behind `desktop/`, and recording is behind `recording/`. Linux is implemented now; future portability work has a clear target.
- Paste is fail-soft. On this Wayland setup, no `wtype`, `ydotool`, `dotool`, or X11 `xdotool` path exists, so clipboard fallback is expected.
- The daemon is deliberately small. Its current useful job is startup recovery; the toggle command is independently usable for GNOME custom shortcuts.
- AppIndicator tray work is deferred because the current desktop environment lacks AppIndicator/Ayatana namespaces.
- Double Alt is deferred because implementing it correctly on Wayland requires input-event access or a lower-level key remapper. That should be explicitly approved before setup.

## 2026-07-06 Audit pass

- The rename to `risper` and the publish to `github.com/rob137/risper` both completed in May 2026; their one-shot task briefs (`docs/rename-to-risper.md`, `docs/publish.md`) are folded into this line and deleted.
- The standalone status monitor/overlay chain and its ignored config knob are removed. The status window was also removed during the Go cutover; `risper-status` is now the service-status alias. If a mic-level display comes back, resurrect it from git history rather than keeping unreferenced code warm.
- Retention stays `retention = "never"`: recordings are still never deleted automatically. Runaway forgotten-toggle sessions (multi-hour WAVs whose transcription was cancelled) get pruned by hand; automatic audio expiry is deferred until manual pruning actually hurts.

## 2026-07-16 Remove Parakeet support

- Parakeet support is removed: the NeMo wrapper, profile-add script, its test, and `docs/parakeet.md` are deleted. whisper.cpp is the only bundled engine. The profile system stays engine-agnostic, so a wrapper for any engine can be added back if and when there is a real reason. Rationale: this is a single-user tool on an AMD/CPU machine where Parakeet was much slower and heavier than whisper.cpp, so the extra engine was complication without payoff.

## 2026-07-16 Default model is small.en with -t 8

- The selected profile moves from `whispercpp-base-en` to `whispercpp-small-en`, and both profiles pass `-t 8` (whisper-cli defaults to 4 threads, half the physical cores). Benchmarks on saved sessions showed small.en costs about 3x the wall time (~3.4s vs ~1s on a short clip) but fixed meaning-level transcription errors on real dictation. base.en stays registered as the fast fallback. Numbers in `docs/performance.md`.
- A Vulkan build for the Radeon 780M iGPU was considered and parked: for short dictation clips the model-load and GPU-init overhead eats the compute saving, and it adds a platform-sensitive build to maintain.

## 2026-08-17 System audio is a second recorder source

This is the pre-Go capture design. The Go Phase 2 design below supersedes its
capture-time `--system` choice and its deletion of the source files.

- `risper-toggle --system` records the mic and the computer's own output at the same time, so a call can be transcribed with everyone's consent. The two captures are blended into the single `audio.wav` that transcription already reads, which leaves the session format, transcription contract, history, retranscribe, and retention untouched.
- Speaker attribution was considered and rejected. Recording the two sides to separate files and labelling the transcript would roughly double transcription time; an LLM reading the finished text can infer who was speaking well enough for notes. The two sources are therefore mixed, not kept apart.
- The mechanism is a `pw-record` property, `stream.capture.sink=true`, with no `--target` so it follows the default sink. Passing `--target` on its own does not work: `pw-record` silently records the default source instead, which reads as success because the mic picks up the speakers.
- Mixing uses `ffmpeg` with `amix`, which is why the stale README line claiming `ffmpeg` was unavailable mattered. `normalize=1` is deliberate: dictation here already peaks at 0 dBFS, so summing two hot sides clipped about four times as many samples as normalising did, and whisper.cpp transcribed the quieter mix identically.
- Following the default sink rather than pinning one was checked against a device change mid-recording: the capture relinked from the Bluetooth headset's mono monitor to the laptop speakers' stereo monitor and carried on, with no corruption from the channel count changing under a mono file header. No reconnect handling is needed.
- Sources are all signalled before any is waited on. Stopping them one at a time let the later source keep recording for as long as the earlier one took to exit.
- A source that captured nothing is dropped and noted in the session errors rather than failing the recording. A failed mix does fail the session and leaves both per-source files on disk to match the existing "failures leave recoverable files behind" rule.
- Long-call ergonomics are knowingly left alone. Transcription still runs inline and still lands on the clipboard, so an hour of audio blocks the toggle for roughly eight minutes at small.en's 0.16x realtime. Ruled out as out of scope for this change; revisit if it becomes annoying in practice.

## 2026-08-19 Go recording and transcription cycle

- Phase 2 moves the record, mix, transcribe, clipboard, notification, sound, and toggle cycle into Go. The durable session and state formats are shared so the migration leaves diagnosable data behind.
- Every Go recording starts both `pw-record` sources. The microphone goes to `audio.mic.wav`, the default sink monitor goes to `audio.system.wav`, and `ffmpeg` produces `audio.wav` with `amix=...:normalize=1`. All three files are retained. Keeping the tracks separate makes the default mixed transcription reversible and makes `audio_retention = "7d"` account for the additional capture rather than silently discarding it.
- The mixed `audio.wav` is the sole transcription input for the Go toggle. There is no capture-time or transcription-time source choice to store in the recording state. A source that captured nothing is already omitted from the mix, so a silent system track adds no transcript content or transcription cost.
- The Go functional test puts stubs for `pw-record`, `ffmpeg`, `whisper-cli`, `wl-copy`, `notify-send`, and `canberra-gtk-play` on a temporary `PATH`, then runs two real toggle cycles. This tests process groups, file hand-off, source selection, clipboard input, and event boundaries without touching Rob's live audio devices or sessions.

## 2026-08-19 Go command surface

- The remaining terminal command surface is available through one Go `risper` binary: service control, history, open, retranscribe, model profiles, diagnostics, benchmarks, and clipboard verification. Compatibility launchers preserve the established command names.
- Go retranscription always reads `audio.wav`. The per-source tracks remain on disk so a later tool can re-read a session differently, but the command has no source-selection flags.
- The Go path deliberately has no paste helper implementation. It normalizes legacy paste modes to `clipboard_only`, copies completed retranscriptions to the clipboard, and keeps the four historical paste metadata fields loadable and consistently false.

## 2026-08-19 Mixed audio is the transcription default

- The earlier mic-only default was meant to avoid putting song lyrics into notes when music was playing. That reasoning is superseded by how Rob actually prompts: he does not listen to music or other material while dictating, so any computer-output audio should be included when it exists.
- The mixed `audio.wav` is therefore transcribed by default in both the toggle and retranscribe paths. The source-selection flags are removed because they no longer express a real choice. The separate mic and system files remain on disk so this decision stays reversible through a later tool.

## 2026-08-06 Audio retention is enforced

- Manual pruning finally hurt, so the deferral recorded on 2026-07-06 ends here. 1562 sessions had accumulated 1.82 GB of `audio.wav` since 6 May while their transcripts and metadata came to 5.6 MB, and the `retention` key was parsed into `Config` and then read by nothing. A setting that describes a policy nobody enforces is worse than no setting.
- The key is now `audio_retention`, accepting `never` or a count with a unit (`7d`, `12h`, `2w`), and it means what it says: audio past the window is deleted, transcripts and metadata are kept forever. The daemon prunes at startup and hourly, `risper-history --prune-audio` does it on demand, and each pruned session records `audio_pruned_at` so `--retranscribe` can say why the wav is gone rather than just reporting it missing.
- The shipped default stays `never`. Deleting a new user's recordings without them asking is not a sensible out-of-the-box behaviour, and the field is now honest either way.

## 2026-08-19 Go daemon and Linux hotkey listener

- Phase 4 moves daemon startup recovery, hourly and startup audio-retention pruning, Double Alt input handling, and daemon logging into Go. The installed user service now runs `risper-daemon`.

## 2026-08-19 Go cutover

- Phase 5 removes the old source and test trees, packaging metadata, and mutation-runner wrapper. `go test ./...` is the test gate and `scripts/mutation-smoke.sh` remains the focused mutation check.
- `install-user.sh` builds one Go binary, installs the ten established compatibility launchers, installs the service and desktop files, and restarts the daemon in the same command. Source edits therefore become live only after a build, install, and restart.
- The standalone status window and automatic paste experiment are not part of the Go workflow. `risper-status` reports service status, while `risper-paste-test` copies a marker for manual clipboard verification.
- Double Alt reads Linux `/dev/input/event*` devices with per-device detector state, kernel event timestamps, stale-state recovery, and device re-registration after read failure. It logs discarded tap attempts as well as successful triggers, so a dead stretch is diagnosable rather than silent. A trigger runs the ordinary Go toggle, which transcribes the mixed capture by default.
- The portability direction is deliberately reversed from the 2026-05-06 entry: this is a one-user Ubuntu tool, so macOS and Windows starter adapters and `docs/portability.md` are not ported in the Go rewrite. The Go `platforms/` package is only the narrow Linux input boundary. Session-type detection remains in session metadata, implemented directly from the Linux session environment rather than carried through those unused adapters.
- Audio retention is now live in the daemon as well as the command surface. Rob's `audio_retention = "7d"` means the three Go capture files are pruned together while transcripts and metadata stay indefinitely.

## 2026-08-20 Voice triggers are opt-in, mic-only, and burst-based

- Voice triggers are disabled by default and remain a daemon feature rather than changing the ordinary `risper-toggle` command. The three words are configurable, must be distinct one-word values, and ship as uncommon, non-military placeholders (`quasar`, `marzipan`, `tangerine`) for Rob to choose rather than as a final product decision.
- Detection reads a second microphone-only `pw-record` stream. It never reads the mixed `audio.wav` or the system monitor, so speaker output cannot fire a command through a direct monitor path; the internal laptop microphone can still hear speakers acoustically. PCM is held only in memory for one short burst and piped to whisper.cpp; no pre-recording audio file or durable history is created, so the existing audio-retention contract remains honest.
- A relative loudness gate is the first filter. It compares each 100 ms frame with a rolling quiet level instead of an absolute dBFS cutoff: the BH-71 DSP gate and the laptop gain-path change make absolute levels unstable, while Rob's measurements show speech is consistently well above the local floor. Silence closes a burst and a single-word `base.en` recognition pass performs the actual match. Candidate audio is capped at 2.5 seconds; a gate that stays open longer marks the burst as a non-candidate and waits for quiet without launching repeated recognition passes, keeping sustained dictation from consuming recognition CPU continuously.
- `small.en` remains the dictation profile, but voice triggers default to the faster `whispercpp-base-en` profile. Continuous whisper polling was rejected because the measured CPU cost would occupy roughly seven cores; the burst path pays for recognition only after loud speech and keeps the expected latency to the silence window plus one short base-model pass. A dedicated loaded wake-word engine is not added because none is installed and introducing a second engine would violate the current one-bundled-engine direction.
- The start word invokes ordinary `risper-toggle`. Stop and send invoke the existing paste paths and pass a private control flag so the matched word is removed only from the end of that completed transcript before clipboard/paste. This keeps the control word out of the note while preserving ordinary uses elsewhere.
- The listener is Linux/PipeWire-specific and reports unavailable profiles or devices without preventing the daemon's existing recovery, retention, or Double Alt work. Enabling it, choosing the final words, and accepting the mic-listening trade-off remain Rob's decisions. `install-user.sh` is not run as part of this change.

## 2026-08-20 Voice trigger latency corrections

- `voice_trigger_profile` remains the source of the trigger model and whisper.cpp command template, but the listener derives a control profile from it: the dictation prompt is replaced by the three trigger words, and `-bs 1 -bo 1` is forced for greedy single-word decoding. The shared renderer supplies `-mc 0` when absent. This keeps trigger tuning out of the general dictation profile while preserving the option to select a different trigger model later.
- The measured base.en trigger pass is about 0.49–0.52s wall and 3.1–3.2s CPU for the tested 0.8s bursts with `-t 8`; default decoding varied from about 0.52s to 1.39s wall depending on content, and the full dictation prompt added about 20–25% in the second measurement. Whisper's 30s padding leaves a roughly 0.5s per-pass floor, so the default 400ms silence window makes roughly 0.9s end-to-end the honest target. A purpose-built ~0.2s wake-word engine remains undecided.
- `desktop.Play` remains a reaped, synchronous primitive for lifecycle-safe callers, while `desktop.PlayAsync` lets the transcription-start sound overlap the stop-to-paste pipeline. The toggle waits on that handle only at process exit, keeping temporary-directory cleanup deterministic without making Rob wait through the 2.18s service-login sample before transcription starts. Recording-start and successful completion sounds are launched without waiting so the daemon's voice-trigger in-flight guard is released before their chimes finish.

## 2026-08-20 Paste-and-send sound

- Paste-only completion keeps the active theme's `complete` event. Paste-and-send uses the same event as both inputs to the selected rising-pair recipe: a perfect-fifth `rubberband` copy delayed 170ms, mixed with the original, and limited to 0.95. This makes the distinction a one-note versus two-note count in the same sound family.
- The derived send sound is generated on first use under Risper's local data directory (`~/.local/share/risper/`) and never committed. The cache identity includes the resolved active-theme source path and file metadata, so changing themes causes a new local derivative and does not keep playing a Yaru-derived file. Missing theme resolution, `ffmpeg`, `rubberband`, or any generation error falls back to the single `complete` event.
