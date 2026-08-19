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

make_go_command() {
	local command="$1"
	local package="$2"
	if ! command -v go >/dev/null 2>&1; then
		echo "go is required to install ${command}" >&2
		exit 1
	fi
	go build -o "${BIN_DIR}/${command}" "${ROOT}/${package}"
	chmod +x "${BIN_DIR}/${command}"
}

make_go_command risper cmd/risper
make_wrapper risper-toggle-python risper.toggle
make_go_command risper-toggle cmd/risper-toggle
make_wrapper risper-daemon risper.daemon
make_wrapper risper-open risper.open
make_wrapper risper-paste-test risper.paste_test
make_wrapper risper-history risper.history
make_wrapper risper-retranscribe risper.retranscribe
make_wrapper risper-models risper.model_cli
make_wrapper risper-status risper.status_window
make_wrapper risper-benchmark risper.benchmark
make_wrapper risper-diagnose risper.diagnose
rm -f "${BIN_DIR}/risper-monitor"

cp "${ROOT}/systemd/risper.service" "${SYSTEMD_DIR}/risper.service"
sed -i "s|__ROOT__|${ROOT}|g" "${SYSTEMD_DIR}/risper.service"

cp "${ROOT}/desktop/risper.desktop" "${APPLICATIONS_DIR}/risper.desktop"
sed -i "s|__ROOT__|${ROOT}|g" "${APPLICATIONS_DIR}/risper.desktop"

systemctl --user daemon-reload || true
systemctl --user enable --now risper.service || true

echo "Installed Risper user commands into ${BIN_DIR}."
echo "Run: risper"
echo "Stop daemon temporarily: risper kill"
