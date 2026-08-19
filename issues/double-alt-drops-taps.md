# Double Alt silently stops working

Double Alt goes dead for stretches, then comes back on its own. Nothing is logged when a tap is dropped, so from the outside it looks arbitrary. Easy to misread as risper caring about window focus, which it does not: the listener reads `/dev/input/event*` and sits below the window manager.

Two things in the current design could produce this.

The detector holds one set of currently-held keys, shared across every input device, and only accepts a tap when that set is empty (`hotkeys.py:23`, `hotkeys.py:39`). A key whose release is never seen leaves the set permanently non-empty and the hotkey permanently dead, until the daemon happens to restart the listener. Releases do go missing: `_drain` drops a device handle on any read error and never re-registers it (`linux_hotkey.py:94`). This machine also feeds nine to twenty-nine devices into that one set, including `ydotoold`, which exposes Left Alt of its own.

The tap gap is measured from `time.monotonic()` at the moment risper drains the event, not from the kernel timestamp, which is unpacked and discarded (`linux_hotkey.py:101`, `linux_hotkey.py:107`). Under CPU load, whisper.cpp on the same box, the reader thread can be scheduled late enough that two taps inside 350 ms measure as outside it.

Worth being able to tell these apart before fixing either. Today the log only records successes (`daemon.py:36`), so a failed attempt leaves no trace at all.
