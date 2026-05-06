#!/usr/bin/env bash
set -euo pipefail

ENGINE_DIR="${HOME}/.local/share/risper/engines"
WHISPER_DIR="${ENGINE_DIR}/whisper.cpp"
MODEL="${1:-base.en}"
REPO_URL="https://github.com/ggerganov/whisper.cpp.git"

mkdir -p "${ENGINE_DIR}" "${HOME}/.config/risper"

if [ ! -d "${WHISPER_DIR}/.git" ]; then
  git clone --depth 1 "${REPO_URL}" "${WHISPER_DIR}"
else
  git -C "${WHISPER_DIR}" pull --ff-only
fi

cmake -S "${WHISPER_DIR}" -B "${WHISPER_DIR}/build" -DCMAKE_BUILD_TYPE=Release
cmake --build "${WHISPER_DIR}/build" --config Release -j"$(nproc)"

"${WHISPER_DIR}/models/download-ggml-model.sh" "${MODEL}"

CONFIG="${HOME}/.config/risper/config.toml"
if [ ! -f "${CONFIG}" ]; then
  PYTHONPATH="$(cd "$(dirname "${BASH_SOURCE[0]}")/../src" && pwd)" /usr/bin/python3 -c 'from risper.config import load_config; load_config()'
fi

PROFILE_ID="whispercpp-${MODEL//./-}"
COMMAND="${WHISPER_DIR}/build/bin/whisper-cli -m ${WHISPER_DIR}/models/ggml-${MODEL}.bin -f {audio} -l {language} -nt -otxt -of {raw_no_txt}"

PYTHONPATH="$(cd "$(dirname "${BASH_SOURCE[0]}")/../src" && pwd)" \
  /usr/bin/python3 -m risper.model_cli add-external "${PROFILE_ID}" \
  --engine "whisper.cpp" \
  --model "${MODEL}" \
  --language "en" \
  --command "${COMMAND}" \
  --select

PYTHONPATH="$(cd "$(dirname "${BASH_SOURCE[0]}")/../src" && pwd)" /usr/bin/python3 - "$CONFIG" "$COMMAND" "$MODEL" <<'PY'
from pathlib import Path
import json
import sys

path = Path(sys.argv[1])
command = sys.argv[2]
model = sys.argv[3]
text = path.read_text(encoding="utf-8") if path.exists() else ""
lines = text.splitlines()
updates = {
    "transcription_engine": "whisper.cpp",
    "transcription_command": command,
    "model": model,
}
seen = set()
out = []
for line in lines:
    key = line.split("=", 1)[0].strip() if "=" in line else ""
    if key in updates:
        out.append(f"{key} = {json.dumps(updates[key])}")
        seen.add(key)
    else:
        out.append(line)
for key, value in updates.items():
    if key not in seen:
        out.append(f"{key} = {json.dumps(value)}")
path.write_text("\n".join(out).rstrip() + "\n", encoding="utf-8")
PY

echo "Installed whisper.cpp locally and configured Risper:"
  echo "  ${WHISPER_DIR}/build/bin/whisper-cli"
  echo "  ${WHISPER_DIR}/models/ggml-${MODEL}.bin"
echo "  profile ${PROFILE_ID}"
  echo "  ${CONFIG}"
