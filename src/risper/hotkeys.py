from __future__ import annotations

from dataclasses import dataclass, field


ALT_KEYS = {56, 100}


@dataclass
class DoubleAltDetector:
    window_ms: int = 350
    _keys_down: set[int] = field(default_factory=set)
    _candidate_alt: int | None = None
    _polluted: bool = False
    _last_tap_ms: float | None = None

    def handle_key(self, key_code: int, pressed: bool, timestamp_ms: float) -> bool:
        if pressed:
            return self._handle_press(key_code)
        return self._handle_release(key_code, timestamp_ms)

    def _handle_press(self, key_code: int) -> bool:
        if key_code in ALT_KEYS and not self._keys_down:
            self._candidate_alt = key_code
            self._polluted = False
        else:
            self._polluted = True
        self._keys_down.add(key_code)
        return False

    def _handle_release(self, key_code: int, timestamp_ms: float) -> bool:
        self._keys_down.discard(key_code)
        if key_code not in ALT_KEYS or self._candidate_alt != key_code:
            if not self._keys_down:
                self._candidate_alt = None
                self._polluted = False
            return False

        pure_tap = not self._polluted and not self._keys_down
        self._candidate_alt = None
        self._polluted = False
        if not pure_tap:
            self._last_tap_ms = None
            return False

        previous = self._last_tap_ms
        self._last_tap_ms = timestamp_ms
        if previous is None:
            return False
        if 0 <= timestamp_ms - previous <= self.window_ms:
            self._last_tap_ms = None
            return True
        return False
