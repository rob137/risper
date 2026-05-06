#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

cp -a "${ROOT}" "${WORK}/risper"
cd "${WORK}/risper"

export PYTHONPATH="${WORK}/risper/src:${PYTHONPATH:-}"

echo "baseline: tests should pass"
/usr/bin/python3 -m unittest discover -s tests

echo "mutation: breaking selected model preference"
perl -0pi -e 's/if config\.selected_model in profiles:/if False and config.selected_model in profiles:/' src/risper/models.py

if /usr/bin/python3 -m unittest discover -s tests >/tmp/risper-mutation.log 2>&1; then
  cat /tmp/risper-mutation.log
  echo "mutation survived: tests did not catch selected-model breakage" >&2
  exit 1
fi

echo "mutation killed: tests failed as expected"
