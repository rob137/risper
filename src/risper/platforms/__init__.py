from __future__ import annotations

import platform

from .base import DesktopPlatform
from .linux import LinuxDesktopPlatform
from .macos import MacOSDesktopPlatform
from .windows import WindowsDesktopPlatform


def current_platform() -> DesktopPlatform:
    system = platform.system().lower()
    if system == "darwin":
        return MacOSDesktopPlatform()
    if system == "windows":
        return WindowsDesktopPlatform()
    return LinuxDesktopPlatform()
