#!/usr/bin/env bash
set -euo pipefail

ENGINE_DIR="${HOME}/.local/share/risper/engines"
WHISPER_DIR="${ENGINE_DIR}/whisper.cpp"
MODEL="${1:-base.en}"
REPO_URL="https://github.com/ggerganov/whisper.cpp.git"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

mkdir -p "${ENGINE_DIR}" "${HOME}/.config/risper"

if [ ! -d "${WHISPER_DIR}/.git" ]; then
  git clone --depth 1 "${REPO_URL}" "${WHISPER_DIR}"
else
  git -C "${WHISPER_DIR}" pull --ff-only
fi

cmake -S "${WHISPER_DIR}" -B "${WHISPER_DIR}/build" -DCMAKE_BUILD_TYPE=Release
cmake --build "${WHISPER_DIR}/build" --config Release -j"$(nproc)"

"${WHISPER_DIR}/models/download-ggml-model.sh" "${MODEL}"

PROFILE_ID="whispercpp-${MODEL//./-}"
COMMAND="${WHISPER_DIR}/build/bin/whisper-cli -m ${WHISPER_DIR}/models/ggml-${MODEL}.bin -f {audio} -l {language} -nt -otxt -of {raw_no_txt}"

cd "${ROOT}"
go run ./cmd/risper models add-external "${PROFILE_ID}" \
  --engine "whisper.cpp" \
  --model "${MODEL}" \
  --language "en" \
  --command "${COMMAND}" \
  --select

echo "Installed whisper.cpp locally and configured Risper:"
  echo "  ${WHISPER_DIR}/build/bin/whisper-cli"
  echo "  ${WHISPER_DIR}/models/ggml-${MODEL}.bin"
  echo "  profile ${PROFILE_ID}"
  echo "  ${HOME}/.config/risper/models.toml"
