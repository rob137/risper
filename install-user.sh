#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${HOME}/.local/bin"
SYSTEMD_DIR="${HOME}/.config/systemd/user"
APPLICATIONS_DIR="${HOME}/.local/share/applications"

mkdir -p "${BIN_DIR}" "${SYSTEMD_DIR}" "${APPLICATIONS_DIR}" "${HOME}/.config/risper"

make_wrapper() {
  local command="$1"
  local module="$2"
  cat > "${BIN_DIR}/${command}" <<EOF
#!/usr/bin/env bash
export PYTHONPATH="${ROOT}/src:\${PYTHONPATH:-}"
exec /usr/bin/python3 -m ${module} "\$@"
EOF
  chmod +x "${BIN_DIR}/${command}"
}

make_wrapper risper risper.cli
make_wrapper risper-toggle risper.toggle
make_wrapper risper-daemon risper.daemon
make_wrapper risper-open risper.open
make_wrapper risper-history risper.history
make_wrapper risper-monitor risper.monitor
make_wrapper risper-retranscribe risper.retranscribe
make_wrapper risper-models risper.model_cli
make_wrapper risper-status risper.status_window
make_wrapper risper-benchmark risper.benchmark
make_wrapper risper-diagnose risper.diagnose

cp "${ROOT}/systemd/risper.service" "${SYSTEMD_DIR}/risper.service"
sed -i "s|__ROOT__|${ROOT}|g" "${SYSTEMD_DIR}/risper.service"

cp "${ROOT}/desktop/risper.desktop" "${APPLICATIONS_DIR}/risper.desktop"
sed -i "s|__ROOT__|${ROOT}|g" "${APPLICATIONS_DIR}/risper.desktop"

systemctl --user daemon-reload || true
systemctl --user enable --now risper.service || true

echo "Installed Risper user commands into ${BIN_DIR}."
echo "Run: risper"
echo "Stop daemon temporarily: risper kill"
