#!/usr/bin/env bash
set -euo pipefail

if ! command -v sudo >/dev/null 2>&1; then
  echo "sudo is required to install ydotool and configure /dev/uinput." >&2
  exit 1
fi

echo "Installing ydotool packages..."
sudo apt-get install -y ydotool ydotoold

echo "Configuring /dev/uinput access for the input group..."
sudo tee /etc/udev/rules.d/99-risper-uinput.rules >/dev/null <<'EOF'
KERNEL=="uinput", GROUP="input", MODE="0660", OPTIONS+="static_node=uinput"
EOF

sudo modprobe uinput || true
sudo udevadm control --reload-rules
sudo udevadm trigger --subsystem-match=misc --sysname-match=uinput || true

if [[ -e /dev/uinput ]]; then
  sudo chgrp input /dev/uinput
  sudo chmod 0660 /dev/uinput
fi

if ! id -nG "${USER}" | tr ' ' '\n' | grep -qx input; then
  echo "Adding ${USER} to input group..."
  sudo usermod -aG input "${USER}"
  echo "You must log out and back in before ydotoold can run as your user."
fi

mkdir -p "${HOME}/.config/systemd/user"
cat > "${HOME}/.config/systemd/user/ydotoold.service" <<'EOF'
[Unit]
Description=ydotool daemon for Risper paste automation

[Service]
Type=simple
ExecStart=/usr/bin/ydotoold
Restart=on-failure
RestartSec=1

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable --now ydotoold.service

echo
echo "ydotoold status:"
systemctl --user status ydotoold.service --no-pager || true
echo
echo "Next verification step:"
echo "  risper-paste-test"
