#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

cp -a "${ROOT}" "${WORK}/risper"
cd "${WORK}/risper"

echo "baseline: tests should pass"
go test ./...

echo "mutation: breaking selected model preference"
perl -0pi -e 's/if profile, ok := profiles\[cfg\.SelectedModel\]; ok \{/if profile, ok := profiles[cfg.SelectedModel]; ok \&\& false {/' models/models.go

if go test ./... >"${WORK}/mutation.log" 2>&1; then
  cat "${WORK}/mutation.log"
  echo "mutation survived: tests did not catch selected-model breakage" >&2
  exit 1
fi

echo "mutation killed: tests failed as expected"
