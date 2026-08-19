#!/usr/bin/env bash
set -euo pipefail

systemctl --user disable --now risper.service 2>/dev/null || true
for command in risper risper-toggle risper-daemon risper-open risper-paste-test risper-history risper-retranscribe risper-models risper-status risper-benchmark risper-diagnose; do
	rm -f "${HOME}/.local/bin/${command}"
done
rm -f "${HOME}/.local/bin/risper-monitor" "${HOME}/.local/bin/risper-toggle-"*
rm -f "${HOME}/.config/systemd/user/risper.service"
rm -f "${HOME}/.local/share/applications/risper.desktop"
systemctl --user daemon-reload || true

echo "Removed Risper commands and service files."
echo "Data and config were intentionally kept:"
echo "  ${HOME}/.local/share/risper"
echo "  ${HOME}/.local/state/risper"
echo "  ${HOME}/.config/risper"
