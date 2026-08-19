#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${HOME}/.local/bin"
SYSTEMD_DIR="${HOME}/.config/systemd/user"
APPLICATIONS_DIR="${HOME}/.local/share/applications"

if ! command -v go >/dev/null 2>&1; then
	echo "go is required to install Risper" >&2
	exit 1
fi

BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "${BUILD_DIR}"' EXIT

go build -o "${BUILD_DIR}/risper" "${ROOT}/cmd/risper"

mkdir -p "${BIN_DIR}" "${SYSTEMD_DIR}" "${APPLICATIONS_DIR}" "${HOME}/.config/risper"

install -m 0755 "${BUILD_DIR}/risper" "${BIN_DIR}/risper"

make_wrapper() {
	local command="$1"
	local subcommand="$2"
	cat > "${BIN_DIR}/${command}" <<EOF
#!/usr/bin/env bash
set -euo pipefail
exec "\$(dirname "\${BASH_SOURCE[0]}")/risper" ${subcommand} "\$@"
EOF
	chmod +x "${BIN_DIR}/${command}"
}

make_wrapper risper-toggle toggle
make_wrapper risper-daemon daemon
make_wrapper risper-open open
make_wrapper risper-paste-test paste-test
make_wrapper risper-history history
make_wrapper risper-retranscribe retranscribe
make_wrapper risper-models models
make_wrapper risper-status status
make_wrapper risper-benchmark benchmark
make_wrapper risper-diagnose diagnose
rm -f "${BIN_DIR}/risper-monitor" "${BIN_DIR}/risper-toggle-"*

cp "${ROOT}/systemd/risper.service" "${SYSTEMD_DIR}/risper.service"
sed -i "s|__ROOT__|${ROOT}|g" "${SYSTEMD_DIR}/risper.service"

cp "${ROOT}/desktop/risper.desktop" "${APPLICATIONS_DIR}/risper.desktop"
sed -i "s|__ROOT__|${ROOT}|g" "${APPLICATIONS_DIR}/risper.desktop"

systemctl --user daemon-reload
systemctl --user enable risper.service
systemctl --user restart risper.service

echo "Built and installed Risper user commands into ${BIN_DIR}."
echo "Restarted risper.service with the new binary."
echo "Run: risper"
echo "Stop daemon temporarily: risper kill"
