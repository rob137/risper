#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -x "${HOME}/.local/bin/risper" ]]; then
  command=("${HOME}/.local/bin/risper" history)
else
  command=(go run "${ROOT}/cmd/risper" history)
fi
if command -v gnome-terminal >/dev/null 2>&1; then
  exec gnome-terminal -- "${command[@]}"
fi
exec "${command[@]}"
