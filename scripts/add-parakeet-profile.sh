#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PYTHONPATH="${ROOT}/src:${PYTHONPATH:-}"

PROFILE_ID="${1:-parakeet-tdt-0-6b-v3}"
MODEL="${2:-nvidia/parakeet-tdt-0.6b-v3}"

exec /usr/bin/python3 -m risper.model_cli add-external "${PROFILE_ID}" \
  --engine "parakeet-nemo" \
  --model "${MODEL}" \
  --language "en" \
  --command "${ROOT}/scripts/parakeet-nemo-wrapper.py --model {model} --audio {audio} --language {language}"
