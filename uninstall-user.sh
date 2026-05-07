#!/usr/bin/env bash
set -euo pipefail

systemctl --user disable --now risper.service 2>/dev/null || true
rm -f "${HOME}/.local/bin/risper"
rm -f "${HOME}/.local/bin/risper-toggle"
rm -f "${HOME}/.local/bin/risper-daemon"
rm -f "${HOME}/.local/bin/risper-open"
rm -f "${HOME}/.local/bin/risper-history"
rm -f "${HOME}/.local/bin/risper-monitor"
rm -f "${HOME}/.local/bin/risper-retranscribe"
rm -f "${HOME}/.local/bin/risper-models"
rm -f "${HOME}/.local/bin/risper-status"
rm -f "${HOME}/.local/bin/risper-benchmark"
rm -f "${HOME}/.local/bin/risper-diagnose"
rm -f "${HOME}/.config/systemd/user/risper.service"
rm -f "${HOME}/.local/share/applications/risper.desktop"
systemctl --user daemon-reload || true

echo "Removed Risper commands and service files."
echo "Data and config were intentionally kept:"
echo "  ${HOME}/.local/share/risper"
echo "  ${HOME}/.local/state/risper"
echo "  ${HOME}/.config/risper"
