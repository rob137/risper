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

## 2026-08-06 Audio retention is enforced

- Manual pruning finally hurt, so the deferral recorded on 2026-07-06 ends here. 1562 sessions had accumulated 1.82 GB of `audio.wav` since 6 May while their transcripts and metadata came to 5.6 MB, and the `retention` key was parsed into `Config` and then read by nothing. A setting that describes a policy nobody enforces is worse than no setting.
- The key is now `audio_retention`, accepting `never` or a count with a unit (`7d`, `12h`, `2w`), and it means what it says: audio past the window is deleted, transcripts and metadata are kept forever. The daemon prunes at startup and hourly, `risper-history --prune-audio` does it on demand, and each pruned session records `audio_pruned_at` so `--retranscribe` can say why the wav is gone rather than just reporting it missing.
- The shipped default stays `never`. Deleting a new user's recordings without them asking is not a sensible out-of-the-box behaviour, and the field is now honest either way.
