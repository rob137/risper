# Decisions

## 2026-05-06 Initial Implementation

- Project name, package, commands, config paths, and data paths use `risper`.
- Recording uses `pw-record` because it is installed and fits the GNOME Wayland/PipeWire environment.
- The first implementation did not download a transcription model or install Python packages. After the follow-up request to continue, whisper.cpp was installed user-locally and the `base.en` model was downloaded.
- Transcription is a local external command hook pointed at whisper.cpp. This keeps the core recoverable while leaving engine choice reversible.
- Model selection is profile-based in `~/.config/risper/models.toml`. This is deliberately lighter than a settings UI and makes a future engine addable via a wrapper command.
- Desktop integration is behind `platforms/`, and recording is behind `recorders.py`. Linux is implemented now; macOS/Windows have starter adapters so future portability work has a clear target.
- Paste is fail-soft. On this Wayland setup, no `wtype`, `ydotool`, `dotool`, or X11 `xdotool` path exists, so clipboard fallback is expected.
- The daemon is deliberately small. Its current useful job is startup recovery; the toggle command is independently usable for GNOME custom shortcuts.
- AppIndicator tray work is deferred because the current Python environment lacks AppIndicator/Ayatana namespaces.
- Double Alt is deferred because implementing it correctly on Wayland requires input-event access or a lower-level key remapper. That should be explicitly approved before setup.

## 2026-07-06 Audit pass

- The rename to `risper` and the publish to `github.com/rob137/risper` both completed in May 2026; their one-shot task briefs (`docs/rename-to-risper.md`, `docs/publish.md`) are folded into this line and deleted.
- The standalone status monitor/overlay chain (`monitor.py`, `overlay.py`, `audiolevel.py`, the `show_overlay` config knob) is removed. It was dead: nothing in `src/` imported it and the daemon explicitly ignored the knob. `status_window.py` (`risper-status`) is the one status UI. If a mic-level display comes back, resurrect from git history rather than keeping unreferenced code warm.
- Retention stays `retention = "never"`: recordings are still never deleted automatically. Runaway forgotten-toggle sessions (multi-hour WAVs whose transcription was cancelled) get pruned by hand; automatic audio expiry is deferred until manual pruning actually hurts.

## 2026-07-16 Remove Parakeet support

- Parakeet support is removed: the NeMo wrapper, profile-add script, its test, and `docs/parakeet.md` are deleted. whisper.cpp is the only bundled engine. The profile system stays engine-agnostic, so a wrapper for any engine can be added back if and when there is a real reason. Rationale: this is a single-user tool on an AMD/CPU machine where Parakeet was much slower and heavier than whisper.cpp, so the extra engine was complication without payoff.

## 2026-07-16 Default model is small.en with -t 8

- The selected profile moves from `whispercpp-base-en` to `whispercpp-small-en`, and both profiles pass `-t 8` (whisper-cli defaults to 4 threads, half the physical cores). Benchmarks on saved sessions showed small.en costs about 3x the wall time (~3.4s vs ~1s on a short clip) but fixed meaning-level transcription errors on real dictation. base.en stays registered as the fast fallback. Numbers in `docs/performance.md`.
- A Vulkan build for the Radeon 780M iGPU was considered and parked: for short dictation clips the model-load and GPU-init overhead eats the compute saving, and it adds a platform-sensitive build to maintain.

## 2026-08-17 System audio is a second recorder source

This is the Python-era capture design. The Go Phase 2 design below supersedes
its capture-time `--system` choice and its deletion of the source files; the
Python compatibility path remains unchanged.

- `risper-toggle --system` records the mic and the computer's own output at the same time, so a call can be transcribed with everyone's consent. The two captures are blended into the single `audio.wav` that transcription already reads, which leaves the session format, transcription contract, history, retranscribe, and retention untouched.
- Speaker attribution was considered and rejected. Recording the two sides to separate files and labelling the transcript would roughly double transcription time; an LLM reading the finished text can infer who was speaking well enough for notes. The two sources are therefore mixed, not kept apart.
- The mechanism is a `pw-record` property, `stream.capture.sink=true`, with no `--target` so it follows the default sink. Passing `--target` on its own does not work: `pw-record` silently records the default source instead, which reads as success because the mic picks up the speakers.
- Mixing uses `ffmpeg` with `amix`, which is why the stale README line claiming `ffmpeg` was unavailable mattered. `normalize=1` is deliberate: dictation here already peaks at 0 dBFS, so summing two hot sides clipped about four times as many samples as normalising did, and whisper.cpp transcribed the quieter mix identically.
- Following the default sink rather than pinning one was checked against a device change mid-recording: the capture relinked from the Bluetooth headset's mono monitor to the laptop speakers' stereo monitor and carried on, with no corruption from the channel count changing under a mono file header. No reconnect handling is needed.
- Sources are all signalled before any is waited on. Stopping them one at a time let the later source keep recording for as long as the earlier one took to exit.
- A source that captured nothing is dropped and noted in the session errors rather than failing the recording. A failed mix does fail the session and leaves both per-source files on disk to match the existing "failures leave recoverable files behind" rule.
- Long-call ergonomics are knowingly left alone. Transcription still runs inline and still lands on the clipboard, so an hour of audio blocks the toggle for roughly eight minutes at small.en's 0.16x realtime. Ruled out as out of scope for this change; revisit if it becomes annoying in practice.

## 2026-08-19 Go recording and transcription cycle

- Phase 2 moves the record, mix, transcribe, clipboard, notification, sound, and toggle cycle into Go. The Python modules remain intact and runnable while the Go command is validated; the durable session and state formats are shared so either side leaves diagnosable data behind.
- Every Go recording starts both `pw-record` sources. The microphone goes to `audio.mic.wav`, the default sink monitor goes to `audio.system.wav`, and `ffmpeg` produces `audio.wav` with `amix=...:normalize=1`. All three files are retained. Keeping the tracks separate is what makes a later mixed transcription possible and makes `audio_retention = "7d"` account for the additional capture rather than silently discarding it.
- `--system` no longer chooses what gets captured. It selects the mixed `audio.wav` for transcription; without it, the Go toggle transcribes `audio.mic.wav`. If the call shortcut is used to start recording, that mixed-transcription request is stored in the recording state so an ordinary shortcut can still stop it. This preserves the useful shortcut habit without making the capture decision at the first keypress.
- The Go functional test puts stubs for `pw-record`, `ffmpeg`, `whisper-cli`, `wl-copy`, `notify-send`, and `canberra-gtk-play` on a temporary `PATH`, then runs two real toggle cycles. This tests process groups, file hand-off, source selection, clipboard input, and event boundaries without touching Rob's live audio devices or sessions.

## 2026-08-19 Go command surface

- The remaining terminal command surface is available through one Go `risper` binary: service control, history, open, retranscribe, model profiles, diagnostics, and benchmarks. The existing Python entry points remain during the migration; the installation script only switches the top-level `risper` service command to Go.
- Go retranscription chooses `audio.mic.wav` by default for sessions with per-source tracks and accepts `--mixed` or `--system` to recover a call from `audio.wav`. Older sessions without source paths continue to use `audio.wav`.
- The Go path deliberately has no paste helper implementation. It normalizes legacy paste modes to `clipboard_only`, copies completed retranscriptions to the clipboard, and keeps the four historical paste metadata fields loadable and consistently false.

## 2026-08-06 Audio retention is enforced

- Manual pruning finally hurt, so the deferral recorded on 2026-07-06 ends here. 1562 sessions had accumulated 1.82 GB of `audio.wav` since 6 May while their transcripts and metadata came to 5.6 MB, and the `retention` key was parsed into `Config` and then read by nothing. A setting that describes a policy nobody enforces is worse than no setting.
- The key is now `audio_retention`, accepting `never` or a count with a unit (`7d`, `12h`, `2w`), and it means what it says: audio past the window is deleted, transcripts and metadata are kept forever. The daemon prunes at startup and hourly, `risper-history --prune-audio` does it on demand, and each pruned session records `audio_pruned_at` so `--retranscribe` can say why the wav is gone rather than just reporting it missing.
- The shipped default stays `never`. Deleting a new user's recordings without them asking is not a sensible out-of-the-box behaviour, and the field is now honest either way.
