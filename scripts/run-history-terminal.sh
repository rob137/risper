#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PYTHONPATH="${ROOT}/src:${PYTHONPATH:-}"
if command -v gnome-terminal >/dev/null 2>&1; then
  exec gnome-terminal -- /usr/bin/python3 -m risper.history
fi
exec /usr/bin/python3 -m risper.history
