#!/usr/bin/env bash
# DEPRECATED: this script is superseded by `just vm-e2e` (arch-aware, covers 3 passes).
# Left here for historical reference. Hardcoded amd64 throughout.
# E2E test: install Flatcar via knuckle headless mode, reboot from target, verify
# Run: just e2e (or ./scripts/e2e-test.sh)
# Requires: qemu-system-x86_64, KVM, ~2min to run
set -euo pipefail

QEMU="${QEMU:-/home/linuxbrew/.linuxbrew/bin/qemu-system-x86_64}"
VM_DIR=".vm"
SSH="ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5 -p 2222 core@127.0.0.1"
SCP="scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -P 2222"
TIMEOUT="${E2E_TIMEOUT:-120}"

cleanup() {
    [ -f "$VM_DIR/qemu.pid" ] && kill "$(cat "$VM_DIR/qemu.pid")" 2>/dev/null || true
    rm -f "$VM_DIR/qemu.pid"
}
trap cleanup EXIT

echo "=== E2E Test: knuckle headless install + boot verification ==="

# 1. Build
echo "[1/8] Building knuckle..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/knuckle-linux-amd64 ./cmd/knuckle

# 2. Prepare VM disks
echo "[2/8] Preparing VM disks..."
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

# 3. Generate Ignition for installer VM (SSH access only)
SSH_KEY=$(cat ~/.ssh/id_ed25519.pub)
printf '{"ignition":{"version":"3.3.0"},"passwd":{"users":[{"name":"core","sshAuthorizedKeys":["%s"]}]},"systemd":{"units":[{"name":"sshd.service","enabled":true}]}}' "$SSH_KEY" > "$VM_DIR/config.ign"

# 4. Boot installer VM
echo "[3/8] Booting installer VM..."
$QEMU -m 2048 -smp 2 -enable-kvm \
    -drive if=virtio,file="$VM_DIR/boot.img",format=qcow2 \
    -drive if=virtio,file="$VM_DIR/target.qcow2",format=qcow2 \
    -fw_cfg "name=opt/org.flatcar-linux/config,file=$VM_DIR/config.ign" \
    -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
    -display none -daemonize -pidfile "$VM_DIR/qemu.pid" 2>/dev/null

echo "  Waiting for SSH..."
for i in $(seq 1 30); do $SSH true 2>/dev/null && break; sleep 2; done
$SSH true 2>/dev/null || { echo "FAIL: SSH timeout"; exit 1; }
echo "  SSH ready"

# 5. Deploy binary + headless config
echo "[4/8] Deploying knuckle + config..."
$SCP bin/knuckle-linux-amd64 core@127.0.0.1:/tmp/knuckle 2>/dev/null
$SSH "chmod +x /tmp/knuckle" 2>/dev/null

# Create headless install config — targets /dev/vdb (the second virtio disk)
cat > "$VM_DIR/install-config.json" <<EOF
{
  "channel": "stable",
  "hostname": "e2e-test-node",
  "timezone": "UTC",
  "network": {"mode": "dhcp"},
  "users": [{"username": "core", "ssh_keys": ["$SSH_KEY"]}],
  "disk": "/dev/vdb",
  "update_strategy": "reboot",
  "reboot": false,
  "dry_run": false
}
EOF

$SCP "$VM_DIR/install-config.json" core@127.0.0.1:/tmp/install-config.json 2>/dev/null

# 6. Run headless install
echo "[5/8] Running knuckle --headless --config..."
INSTALL_OUTPUT=$($SSH "sudo /tmp/knuckle --config /tmp/install-config.json --headless --log-file /tmp/knuckle-e2e.log" 2>&1) || {
    echo "  INSTALL FAILED:"
    echo "$INSTALL_OUTPUT"
    echo ""
    echo "  Log tail:"
    $SSH "tail -20 /tmp/knuckle-e2e.log" 2>/dev/null || true
    exit 1
}
echo "$INSTALL_OUTPUT" | sed 's/^/  /'
echo "  ✓ Install completed"

# 7. Boot from installed target disk
echo "[6/8] Booting installed system..."
cleanup
sleep 1
$QEMU -m 2048 -smp 2 -enable-kvm \
    -drive if=virtio,file="$VM_DIR/target.qcow2",format=qcow2 \
    -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
    -display none -daemonize -pidfile "$VM_DIR/qemu.pid" 2>/dev/null

echo "  Waiting for installed system SSH..."
for i in $(seq 1 45); do $SSH true 2>/dev/null && break; sleep 2; done
$SSH true 2>/dev/null || { echo "FAIL: installed system SSH timeout"; exit 1; }
echo "  Installed system booted"

# 8. Verify installed system
echo "[7/8] Verifying installed system..."
PASS=0
FAIL=0

check() {
    local name="$1" got="$2" want="$3"
    if [ "$got" = "$want" ]; then
        echo "  ✓ $name: $got"
        PASS=$((PASS+1))
    else
        echo "  ✗ $name: got '$got', want '$want'"
        FAIL=$((FAIL+1))
    fi
}

check_contains() {
    local name="$1" haystack="$2" needle="$3"
    if echo "$haystack" | grep -q "$needle"; then
        echo "  ✓ $name: contains '$needle'"
        PASS=$((PASS+1))
    else
        echo "  ✗ $name: missing '$needle' in '$haystack'"
        FAIL=$((FAIL+1))
    fi
}

HOSTNAME=$($SSH "hostname" 2>/dev/null || echo "")
OS_RELEASE=$($SSH "cat /etc/os-release" 2>/dev/null || echo "")
UPDATE_CONF=$($SSH "cat /etc/flatcar/update.conf 2>/dev/null" 2>/dev/null || echo "")
SSH_KEYS=$($SSH "cat ~/.ssh/authorized_keys 2>/dev/null | wc -l" 2>/dev/null || echo "0")
KERNEL=$($SSH "uname -r" 2>/dev/null || echo "")

check "Hostname" "$HOSTNAME" "e2e-test-node"
check_contains "OS" "$OS_RELEASE" "flatcar"
check_contains "Update channel" "$UPDATE_CONF" "GROUP=stable"

if [ "$SSH_KEYS" -gt 0 ]; then
    echo "  ✓ SSH keys: $SSH_KEYS key(s)"
    PASS=$((PASS+1))
else
    echo "  ✗ SSH keys: none found"
    FAIL=$((FAIL+1))
fi

if [ -n "$KERNEL" ]; then
    echo "  ✓ Kernel: $KERNEL"
    PASS=$((PASS+1))
else
    echo "  ✗ Kernel: not detected"
    FAIL=$((FAIL+1))
fi

# Report
echo ""
echo "[8/8] Results: $PASS passed, $FAIL failed"
cleanup

if [ "$FAIL" -gt 0 ]; then
    echo ""
    echo "FAIL"
    exit 1
fi
echo ""
echo "PASS — Full E2E headless install verified ✅"
