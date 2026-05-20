# Knuckle — Flatcar Container Linux TUI Installer
# https://github.com/castrojo/knuckle

QEMU := if path_exists("/home/linuxbrew/.linuxbrew/bin/qemu-system-x86_64") == "true" { "/home/linuxbrew/.linuxbrew/bin/qemu-system-x86_64" } else { "qemu-system-x86_64" }
SSH_OPTS := "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"

default:
    @just --list

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

# Run E2E test: install to disk + boot + verify
e2e:
    ./scripts/e2e-test.sh

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
