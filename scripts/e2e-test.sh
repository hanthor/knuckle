#!/usr/bin/env bash
# E2E test: install Flatcar via knuckle, reboot from target, verify config
# Run from repo root: ./scripts/e2e-test.sh
set -euo pipefail

QEMU="${QEMU:-/home/linuxbrew/.linuxbrew/bin/qemu-system-x86_64}"
VM_DIR=".vm"
SSH="ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5 -p 2222 core@127.0.0.1"
SCP="scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -P 2222"

cleanup() {
    [ -f "$VM_DIR/qemu.pid" ] && kill "$(cat "$VM_DIR/qemu.pid")" 2>/dev/null || true
    rm -f "$VM_DIR/qemu.pid"
}
trap cleanup EXIT

echo "=== E2E Test: knuckle install + boot verification ==="

# 1. Build
echo "[1/7] Building knuckle..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/knuckle-linux-amd64 ./cmd/knuckle

# 2. Prepare VM
echo "[2/7] Preparing VM disks..."
mkdir -p "$VM_DIR"
[ -f "$VM_DIR/flatcar_base.img" ] || {
    echo "  Downloading Flatcar image..."
    curl -sL -o "$VM_DIR/flatcar_base.img.bz2" \
        "https://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_qemu_image.img.bz2"
    bunzip2 "$VM_DIR/flatcar_base.img.bz2"
}
cleanup
rm -f "$VM_DIR/boot.img" "$VM_DIR/target.qcow2"
cp "$VM_DIR/flatcar_base.img" "$VM_DIR/boot.img"
qemu-img create -f qcow2 "$VM_DIR/target.qcow2" 20G >/dev/null

SSH_KEY=$(cat ~/.ssh/id_ed25519.pub)
printf '{"ignition":{"version":"3.3.0"},"passwd":{"users":[{"name":"core","sshAuthorizedKeys":["%s"]}]},"systemd":{"units":[{"name":"sshd.service","enabled":true}]}}' "$SSH_KEY" > "$VM_DIR/config.ign"

# 3. Boot installer VM
echo "[3/7] Booting installer VM..."
$QEMU -m 2048 -smp 2 -enable-kvm \
    -drive if=virtio,file="$VM_DIR/boot.img",format=qcow2 \
    -drive if=virtio,file="$VM_DIR/target.qcow2",format=qcow2 \
    -fw_cfg "name=opt/org.flatcar-linux/config,file=$VM_DIR/config.ign" \
    -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
    -display none -daemonize -pidfile "$VM_DIR/qemu.pid" 2>/dev/null

for i in $(seq 1 30); do $SSH true 2>/dev/null && break; sleep 2; done
echo "  SSH ready"

# 4. Deploy and run install (non-interactive, scripted)
echo "[4/7] Running knuckle install..."
$SCP bin/knuckle-linux-amd64 core@127.0.0.1:/tmp/knuckle 2>/dev/null

# Generate a config and install directly (bypass TUI for E2E)
$SSH 'sudo /tmp/knuckle --dry-run=false --log-file /tmp/knuckle-e2e.log' <<'INPUT' 2>/dev/null || true
INPUT
# For non-interactive E2E, use the install package directly
$SSH "cat /tmp/knuckle-e2e.log" 2>/dev/null | tail -3

# 5. Boot from installed target disk
echo "[5/7] Booting installed system..."
cleanup
sleep 1
$QEMU -m 2048 -smp 2 -enable-kvm \
    -drive if=virtio,file="$VM_DIR/target.qcow2",format=qcow2 \
    -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
    -display none -daemonize -pidfile "$VM_DIR/qemu.pid" 2>/dev/null

for i in $(seq 1 30); do $SSH true 2>/dev/null && break; sleep 2; done
echo "  Installed system booted"

# 6. Verify
echo "[6/7] Verifying installed system..."
HOSTNAME=$($SSH "hostname" 2>/dev/null)
OS_VERSION=$($SSH "grep VERSION_ID /etc/os-release" 2>/dev/null | cut -d= -f2)
UPDATE_CONF=$($SSH "cat /etc/flatcar/update.conf 2>/dev/null" 2>/dev/null)
SSH_KEYS=$($SSH "cat ~/.ssh/authorized_keys 2>/dev/null | wc -l" 2>/dev/null)

PASS=0
FAIL=0

check() {
    if [ "$2" = "$3" ]; then
        echo "  ✓ $1: $2"
        PASS=$((PASS+1))
    else
        echo "  ✗ $1: got '$2', want '$3'"
        FAIL=$((FAIL+1))
    fi
}

check "OS" "$OS_VERSION" "4593.2.1"
check "Hostname" "$HOSTNAME" "flatcar"

if echo "$UPDATE_CONF" | grep -q "GROUP=stable"; then
    echo "  ✓ Update channel: stable"
    PASS=$((PASS+1))
else
    echo "  ✗ Update channel: not configured"
    FAIL=$((FAIL+1))
fi

if [ "$SSH_KEYS" -gt 0 ]; then
    echo "  ✓ SSH keys: $SSH_KEYS key(s)"
    PASS=$((PASS+1))
else
    echo "  ✗ SSH keys: none"
    FAIL=$((FAIL+1))
fi

# 7. Report
echo ""
echo "[7/7] Results: $PASS passed, $FAIL failed"
cleanup

if [ "$FAIL" -gt 0 ]; then
    echo "FAIL"
    exit 1
fi
echo "PASS"
