# Knuckle — Flatcar Container Linux TUI Installer
# https://github.com/castrojo/knuckle

QEMU := if path_exists("/home/linuxbrew/.linuxbrew/bin/qemu-system-x86_64") == "true" { "/home/linuxbrew/.linuxbrew/bin/qemu-system-x86_64" } else { "qemu-system-x86_64" }
SSH_OPTS := "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"

default:
    @just --list
    @echo ""
    @echo "Quickstart:"
    @echo "  just vm        — interactive TUI in a VM (dry-run)"
    @echo "  just vm ''     — interactive TUI, live install"
    @echo "  just e2e       — full end-to-end: build ISO → boot → install → verify"

# Build the binary
build:
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/knuckle ./cmd/knuckle

# Run tests
test:
    go test ./...

# Full CI (lint + test + build)
ci:
    go mod tidy
    golangci-lint run ./...
    go test -race ./...
    just build

# Quick headless dry-run test (no VM needed)
headless-test:
    #!/usr/bin/env bash
    set -euo pipefail
    just build
    cat > /tmp/knuckle-test-config.json <<'EOF'
    {"channel":"stable","hostname":"test-node","timezone":"UTC","network":{"mode":"dhcp"},"users":[{"username":"core","ssh_keys":["ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI test"]}],"disk":"/dev/vdb","update_strategy":"reboot","reboot":false}
    EOF
    bin/knuckle --config /tmp/knuckle-test-config.json --headless --dry-run
    echo "✅ PASS"

# Boot VM and launch knuckle TUI (interactive over SSH)
vm *FLAGS='--dry-run':
    #!/usr/bin/env bash
    set -euo pipefail
    just build
    just _ensure-base
    just _kill-vm

    rm -f .vm/boot.qcow2 .vm/target.qcow2
    qemu-img create -f qcow2 -b flatcar_base.img -F qcow2 .vm/boot.qcow2 >/dev/null
    qemu-img create -f qcow2 .vm/target.qcow2 20G >/dev/null
    just _write-ignition

    {{QEMU}} \
        -m 2048 -smp 2 -enable-kvm \
        -drive if=virtio,file=.vm/boot.qcow2,format=qcow2 \
        -drive if=virtio,file=.vm/target.qcow2,format=qcow2 \
        -fw_cfg name=opt/org.flatcar-linux/config,file=.vm/config.ign \
        -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
        -display none -daemonize -pidfile .vm/qemu.pid

    echo "Waiting for VM..."
    for i in $(seq 1 20); do
        ssh {{SSH_OPTS}} -o ConnectTimeout=2 -p 2222 core@127.0.0.1 true 2>/dev/null && break
        sleep 2
    done

    scp {{SSH_OPTS}} -P 2222 bin/knuckle core@127.0.0.1:/tmp/knuckle 2>/dev/null
    exec ssh -t {{SSH_OPTS}} -p 2222 core@127.0.0.1 "sudo /tmp/knuckle {{FLAGS}} --log-file /tmp/knuckle.log"

# Boot VM in background, get a shell (knuckle already deployed)
vm-ssh *FLAGS='--dry-run':
    #!/usr/bin/env bash
    set -euo pipefail
    just build
    just _ensure-base
    just _kill-vm

    rm -f .vm/boot.qcow2 .vm/target.qcow2
    qemu-img create -f qcow2 -b flatcar_base.img -F qcow2 .vm/boot.qcow2 >/dev/null
    qemu-img create -f qcow2 .vm/target.qcow2 20G >/dev/null
    just _write-ignition

    {{QEMU}} \
        -m 2048 -smp 2 -enable-kvm \
        -drive if=virtio,file=.vm/boot.qcow2,format=qcow2 \
        -drive if=virtio,file=.vm/target.qcow2,format=qcow2 \
        -fw_cfg name=opt/org.flatcar-linux/config,file=.vm/config.ign \
        -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
        -display none -daemonize -pidfile .vm/qemu.pid

    echo "Waiting for VM..."
    for i in $(seq 1 30); do
        ssh {{SSH_OPTS}} -o ConnectTimeout=2 -p 2222 core@127.0.0.1 true 2>/dev/null && break
        sleep 2
    done

    scp {{SSH_OPTS}} -P 2222 bin/knuckle core@127.0.0.1:/tmp/knuckle 2>/dev/null
    echo "VM ready. Binary at /tmp/knuckle. Run: sudo /tmp/knuckle {{FLAGS}}"
    exec ssh -t {{SSH_OPTS}} -p 2222 core@127.0.0.1

# SSH into running VM
ssh:
    ssh -t {{SSH_OPTS}} -p 2222 core@127.0.0.1

# Boot the installed target disk to verify installation worked
boot-target:
    #!/usr/bin/env bash
    set -euo pipefail
    just _kill-vm
    [ -f .vm/target.qcow2 ] || { echo "No target disk. Run 'just vm' first."; exit 1; }
    echo "Booting installed target disk..."
    echo "  → Ctrl-a x to quit QEMU"
    {{QEMU}} \
        -m 2048 -smp 2 -enable-kvm \
        -drive if=virtio,file=.vm/target.qcow2,format=qcow2 \
        -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
        -nographic

# Full end-to-end: build ISO → boot → headless install → reboot target → verify
e2e:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "=== E2E: ISO build → boot → install → verify ==="
    echo ""

    # 1. Build ISO
    echo "[1/5] Building ISO..."
    just iso stable
    echo ""

    # 2. Build binary for headless deploy
    echo "[2/5] Building binary..."
    just build
    echo ""

    # 3. Boot ISO, wait for system, deploy+run headless install
    echo "[3/5] Booting ISO + running headless install..."
    just _kill-vm
    mkdir -p .vm
    rm -f .vm/target.qcow2
    qemu-img create -f qcow2 .vm/target.qcow2 20G >/dev/null

    OVMF="/home/linuxbrew/.linuxbrew/Cellar/qemu/11.0.0/share/qemu/edk2-x86_64-code.fd"
    {{QEMU}} \
        -m 4096 -smp 2 -enable-kvm \
        -drive if=pflash,format=raw,readonly=on,file="$OVMF" \
        -cdrom output/knuckle-installer-stable.iso \
        -drive if=virtio,file=.vm/target.qcow2,format=qcow2 \
        -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
        -display none -daemonize -pidfile .vm/qemu.pid

    echo "  Waiting for ISO to boot..."
    for i in $(seq 1 60); do
        ssh {{SSH_OPTS}} -o ConnectTimeout=2 -p 2222 core@127.0.0.1 true 2>/dev/null && break
        sleep 2
    done
    ssh {{SSH_OPTS}} -o ConnectTimeout=5 -p 2222 core@127.0.0.1 true 2>/dev/null || {
        echo "FAIL: ISO did not boot (SSH timeout)"
        just _kill-vm
        exit 1
    }
    echo "  ISO booted, deploying knuckle..."

    # Deploy binary + config
    scp {{SSH_OPTS}} -P 2222 bin/knuckle core@127.0.0.1:/tmp/knuckle 2>/dev/null
    SSH_KEY=$(cat ~/.ssh/id_ed25519.pub)
    cat > .vm/e2e-config.json <<CFGEOF
    {"channel":"stable","hostname":"e2e-node","timezone":"UTC","network":{"mode":"dhcp"},"users":[{"username":"core","ssh_keys":["$SSH_KEY"]}],"disk":"/dev/vdb","update_strategy":"reboot","reboot":false,"dry_run":false}
    CFGEOF
    scp {{SSH_OPTS}} -P 2222 .vm/e2e-config.json core@127.0.0.1:/tmp/config.json 2>/dev/null

    echo "  Running headless install..."
    ssh {{SSH_OPTS}} -p 2222 core@127.0.0.1 "sudo /tmp/knuckle --config /tmp/config.json --headless --log-file /tmp/knuckle.log" || {
        echo "FAIL: headless install failed"
        ssh {{SSH_OPTS}} -p 2222 core@127.0.0.1 "tail -20 /tmp/knuckle.log" 2>/dev/null || true
        just _kill-vm
        exit 1
    }
    echo "  ✓ Install completed"
    echo ""

    # 4. Boot from installed target disk
    echo "[4/5] Booting installed target disk..."
    just _kill-vm
    sleep 1
    {{QEMU}} \
        -m 2048 -smp 2 -enable-kvm \
        -drive if=virtio,file=.vm/target.qcow2,format=qcow2 \
        -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
        -display none -daemonize -pidfile .vm/qemu.pid

    echo "  Waiting for installed system..."
    for i in $(seq 1 45); do
        ssh {{SSH_OPTS}} -o ConnectTimeout=2 -p 2222 core@127.0.0.1 true 2>/dev/null && break
        sleep 2
    done
    ssh {{SSH_OPTS}} -o ConnectTimeout=5 -p 2222 core@127.0.0.1 true 2>/dev/null || {
        echo "FAIL: installed system did not boot"
        just _kill-vm
        exit 1
    }
    echo "  Target booted"
    echo ""

    # 5. Verify
    echo "[5/5] Verifying installed system..."
    PASS=0; FAIL=0
    check() {
        local name="$1" got="$2" want="$3"
        if [ "$got" = "$want" ]; then echo "  ✓ $name: $got"; PASS=$((PASS+1))
        else echo "  ✗ $name: got '$got', want '$want'"; FAIL=$((FAIL+1)); fi
    }
    HOSTNAME=$(ssh {{SSH_OPTS}} -p 2222 core@127.0.0.1 "hostname" 2>/dev/null)
    KERNEL=$(ssh {{SSH_OPTS}} -p 2222 core@127.0.0.1 "uname -r" 2>/dev/null)
    OS=$(ssh {{SSH_OPTS}} -p 2222 core@127.0.0.1 "grep ^ID= /etc/os-release" 2>/dev/null)
    CHANNEL=$(ssh {{SSH_OPTS}} -p 2222 core@127.0.0.1 "grep GROUP /etc/flatcar/update.conf" 2>/dev/null)
    KEYS=$(ssh {{SSH_OPTS}} -p 2222 core@127.0.0.1 "wc -l < ~/.ssh/authorized_keys" 2>/dev/null)

    check "Hostname" "$HOSTNAME" "e2e-node"
    check "OS" "$OS" "ID=flatcar"
    check "Channel" "$CHANNEL" "GROUP=stable"
    [ "${KEYS:-0}" -gt 0 ] && { echo "  ✓ SSH keys: $KEYS"; PASS=$((PASS+1)); } || { echo "  ✗ SSH keys: none"; FAIL=$((FAIL+1)); }
    [ -n "${KERNEL:-}" ] && { echo "  ✓ Kernel: $KERNEL"; PASS=$((PASS+1)); } || { echo "  ✗ Kernel: not detected"; FAIL=$((FAIL+1)); }

    just _kill-vm
    echo ""
    echo "Results: $PASS passed, $FAIL failed"
    [ "$FAIL" -eq 0 ] && echo "✅ E2E PASS" || { echo "❌ E2E FAIL"; exit 1; }

# Build installer ISO (requires xorriso, mtools, cpio; GRUB optional)
iso *CHANNEL='stable':
    ./scripts/build-iso.sh {{CHANNEL}}

# Boot ISO in QEMU with UEFI (Ctrl-a x to quit)
boot-iso:
    #!/usr/bin/env bash
    set -euo pipefail
    ISO="output/knuckle-installer-stable.iso"
    [ -f "$ISO" ] || { echo "No ISO. Run: just iso"; exit 1; }
    just _kill-vm
    mkdir -p .vm
    [ -f .vm/target.qcow2 ] || qemu-img create -f qcow2 .vm/target.qcow2 20G >/dev/null
    OVMF="/home/linuxbrew/.linuxbrew/Cellar/qemu/11.0.0/share/qemu/edk2-x86_64-code.fd"
    [ -f "$OVMF" ] || { echo "OVMF not found at $OVMF"; exit 1; }
    echo "Booting ISO (UEFI)..."
    echo "  → Ctrl-a x to quit QEMU"
    echo "  → At EFI shell, type: fs0:\\startup.nsh"
    echo ""
    {{QEMU}} \
        -m 4096 -smp 2 -enable-kvm \
        -drive if=pflash,format=raw,readonly=on,file="$OVMF" \
        -cdrom "$ISO" \
        -drive if=virtio,file=.vm/target.qcow2,format=qcow2 \
        -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
        -nographic

# Boot ISO in background, wait for SSH
boot-iso-ssh:
    #!/usr/bin/env bash
    set -euo pipefail
    ISO="output/knuckle-installer-stable.iso"
    [ -f "$ISO" ] || { echo "No ISO. Run: just iso"; exit 1; }
    just _kill-vm
    mkdir -p .vm
    [ -f .vm/target.qcow2 ] || qemu-img create -f qcow2 .vm/target.qcow2 20G >/dev/null
    OVMF="/home/linuxbrew/.linuxbrew/Cellar/qemu/11.0.0/share/qemu/edk2-x86_64-code.fd"
    [ -f "$OVMF" ] || { echo "OVMF not found at $OVMF"; exit 1; }
    {{QEMU}} \
        -m 4096 -smp 2 -enable-kvm \
        -drive if=pflash,format=raw,readonly=on,file="$OVMF" \
        -cdrom "$ISO" \
        -drive if=virtio,file=.vm/target.qcow2,format=qcow2 \
        -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
        -display none -daemonize -pidfile .vm/qemu.pid
    echo "Waiting for ISO to boot..."
    for i in $(seq 1 40); do
        ssh {{SSH_OPTS}} -o ConnectTimeout=2 -p 2222 core@127.0.0.1 true 2>/dev/null && break
        sleep 3
    done
    echo "VM ready."
    exec ssh -t {{SSH_OPTS}} -p 2222 core@127.0.0.1

# Stop running VM
stop:
    just _kill-vm

# Clean everything
clean:
    #!/usr/bin/env bash
    just _kill-vm 2>/dev/null || true
    rm -rf bin/ .vm/

# --- Internal helpers (not listed) ---

[private]
_ensure-base:
    #!/usr/bin/env bash
    mkdir -p .vm
    if [ ! -f ".vm/flatcar_base.img" ]; then
        echo "Downloading Flatcar stable QEMU image (one-time)..."
        curl -L -o .vm/flatcar_base.img.bz2 \
            "https://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_qemu_image.img.bz2"
        bunzip2 .vm/flatcar_base.img.bz2
    fi

[private]
_kill-vm:
    #!/usr/bin/env bash
    if [ -f .vm/qemu.pid ]; then
        kill "$(cat .vm/qemu.pid)" 2>/dev/null || true
        rm -f .vm/qemu.pid
        sleep 1
    fi

[private]
_write-ignition:
    #!/usr/bin/env bash
    set -euo pipefail
    SSH_KEY=$(cat ~/.ssh/id_ed25519.pub)
    cat > .vm/config.ign <<EOF
    {
      "ignition": {"version": "3.4.0"},
      "passwd": {"users": [{"name": "core", "sshAuthorizedKeys": ["$SSH_KEY"]}]},
      "systemd": {"units": [
        {"name": "sshd.service", "enabled": true}
      ]}
    }
    EOF
