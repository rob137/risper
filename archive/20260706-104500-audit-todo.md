# TODO — risper

_Claude audit pass, 2026-07-06 10:37. Executed 2026-07-06 10:45._

Hygiene backlog from a whole-repo look-over. Not scope creep: nothing here adds a
feature, each item closes a gap the current code already has. Evidence gathered
against the live daemon (PID 115514, up since 2026-06-16).

## Risk / correctness (2026-07-06 10:37)

- [x] **Commit the live-but-uncommitted Wayland fix.** (done 2026-07-06 10:41)
  `src/risper/platforms/linux.py` had been dirty since 2026-06-16 (Wayland-first
  `session_type` default for the systemd environment) — and because the wrappers
  run straight out of the repo `src/`, that diff WAS the running daemon's
  behaviour, one careless `git checkout` from a silent revert. Committed with
  three tests pinning the unset/empty/lowercase cases.
- [x] **The double-alt listener leaks threads across restarts.** (done
  2026-07-06 10:44) Live daemon had 374 threads after 560 listener restarts
  since Jun 16: `stop()` closed the handles but a thread blocked in `read()` on
  a quiet device (lid switch, sleep button) never wakes on close, and nothing
  joined. Replaced the thread-per-device design with a single selector-driven
  thread over non-blocking fds, joined on stop — regression test opens a FIFO
  device and asserts the thread actually dies. Restart churn debounced too: a
  device change must hold for a full tick before restarting (dock/undock used
  to fire several restarts within a second).
- [x] **Decide retention.** (decided 2026-07-06 10:45) `retention = "never"`
  stays: recordings are never deleted automatically, and that remains the
  design. Runaway forgotten-toggle sessions (multi-hour WAVs whose transcription
  was cancelled, no transcript) get pruned by hand; the three live offenders
  (452M + 179M + 123M, ~750M of 1.7G) verified as accidents via their
  `events.jsonl` and deleted. Auto-expiry deferred until manual pruning hurts.
  Recorded in `docs/decisions.md`.

## Hygiene (2026-07-06 10:37)

- [x] **Delete dead modules or say why they stay.** (done 2026-07-06 10:44)
  Deleted: `monitor.py`, `overlay.py`, `audiolevel.py`, their two test files,
  and the `show_overlay` config knob the daemon explicitly ignored.
  `status_window.py` (`risper-status`) is the one status UI. Rationale in
  `docs/decisions.md`; resurrect from git history if a mic-level display
  returns.
- [x] **Retire the fossil docs.** (done 2026-07-06 10:44)
  `docs/rename-to-risper.md` and `docs/publish.md` deleted, each folded into a
  line in `docs/decisions.md`.
- [x] **Refresh README "Current Environment Findings".** (done 2026-07-06
  10:44) Now a dated snapshot (verified 2026-07-06); `ydotool` and `wtype`
  moved to the available list.
- [x] **Sweep stale `__pycache__`.** (done 2026-07-06 10:44) Removed
  everywhere; remnants of deleted modules no longer pollute greps.

## Open

(nothing — file future items here with a DTG stamp)
