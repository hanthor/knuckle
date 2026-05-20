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
    #!/usr/bin/env bash
    VERSION=$(git describe --tags --always 2>/dev/null || echo dev)
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o bin/knuckle ./cmd/knuckle

# Run tests
test:
    go test ./...

# Tool versions — bump here and in .github/workflows/ci.yml together
GOLANGCI_LINT_VERSION := "2.11.4"
GOLANGCI_LINT := ".tools/golangci-lint"

# Install pinned tool binaries (idempotent — run once after clone, or after version bump)
tools:
    just _install-golangci-lint
    go tool govulncheck -version
    @echo "tools ok"

# Full CI (tidy + fmt-check + vet + lint + vuln + test-race + cover-check + headless-e2e + build)
ci: _install-golangci-lint
    go mod tidy
    @git diff --exit-code go.mod go.sum || (echo "go.mod/go.sum dirty after tidy" && exit 1)
    just fmt-check
    go vet ./...
    {{GOLANGCI_LINT}} run ./...
    go tool govulncheck ./...
    go test -race ./...
    just cover-check
    just headless-test
    just build

# Check formatting (CI gate — fails if any file would change)
fmt-check:
    #!/usr/bin/env bash
    out=$(gofmt -l . 2>&1)
    if [[ -n "$out" ]]; then
        echo "gofmt would change these files:"; echo "$out"; exit 1
    fi

# Format all Go files in place
fmt:
    gofmt -w .

# Vulnerability scan — version pinned in go.mod via `go tool`
vuln:
    go tool govulncheck ./...

# Coverage report (text + cover.out for tooling)
cover:
    go test -count=1 -race -covermode=atomic -coverprofile=cover.out ./...
    @go tool cover -func=cover.out | tail -1

# Coverage HTML — open cover.html in browser to inspect uncovered lines
cover-html: cover
    go tool cover -html=cover.out -o cover.html
    @echo "open cover.html"

# Per-package coverage gate. Mirrors docs/CI-AND-TESTING.md targets.
# Exits non-zero if any package falls below its threshold.
# Uses statement-count coverage from `go test -cover`, not function-average.
cover-check:
    #!/usr/bin/env bash
    set -euo pipefail
    declare -A targets=(
        [model]=90  [validate]=85  [ignition]=85  [github]=85
        [bakery]=80 [probe]=80     [runner]=80    [install]=70
        [headless]=70 [wizard]=70  [iso]=70       [tui]=40
    )
    fail=0
    for pkg in "${!targets[@]}"; do
        pct=$(go test -count=1 -cover ./internal/${pkg}/... 2>/dev/null \
            | awk '/coverage:/ {gsub("%",""); print $(NF-2); exit}')
        pct=${pct%.*}
        if [[ -z "$pct" ]]; then
            echo "FAIL  internal/${pkg}   no coverage reported"
            fail=1
            continue
        fi
        if (( pct < ${targets[$pkg]} )); then
            echo "FAIL  internal/${pkg}  ${pct}%  (target ${targets[$pkg]}%)"
            fail=1
        else
            echo "ok    internal/${pkg}  ${pct}%  (target ${targets[$pkg]}%)"
        fi
    done
    exit $fail

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

# Full end-to-end: build ISO → boot in Ghostty → interactive install
e2e:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "=== E2E: Build ISO + launch interactive install VM ==="
    echo ""

    # Build ISO if not present
    ISO="output/knuckle-installer-stable.iso"
    if [[ ! -f "$ISO" ]]; then
        echo "Building ISO..."
        just iso stable
        echo ""
    fi

    just _kill-vm
    mkdir -p .vm
    rm -f .vm/target.qcow2
    qemu-img create -f qcow2 .vm/target.qcow2 20G >/dev/null

    OVMF="/home/linuxbrew/.linuxbrew/Cellar/qemu/11.0.0/share/qemu/edk2-x86_64-code.fd"
    if [[ ! -f "$OVMF" ]]; then
        echo "❌ OVMF not found at $OVMF"
        exit 1
    fi

    echo "Launching installer VM in Ghostty..."
    echo "  → ISO boots with GRUB, knuckle launches on tty1"
    echo "  → Target disk: .vm/target.qcow2 (20G)"
    echo "  → After install: just boot-target to verify"
    echo ""

    ghostty --gtk-single-instance=false -e bash -c "\
        cd $(pwd) && \
        {{QEMU}} \
            -m 4096 -smp 2 -enable-kvm \
            -drive if=pflash,format=raw,readonly=on,file=$OVMF \
            -cdrom $ISO \
            -drive if=virtio,file=.vm/target.qcow2,format=qcow2 \
            -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
            -nographic" &
    echo "VM window opened. When done: just boot-target"

# Build installer ISO (requires xorriso, mtools, cpio, systemd-boot-efi)
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
    OVMF=""
    for candidate in \
        /usr/share/OVMF/OVMF_CODE.fd \
        /usr/share/edk2/ovmf/OVMF_CODE.fd \
        /home/linuxbrew/.linuxbrew/Cellar/qemu/*/share/qemu/edk2-x86_64-code.fd; do
        # shellcheck disable=SC2086
        for f in $candidate; do
            [ -f "$f" ] && OVMF="$f" && break 2
        done
    done
    [ -n "$OVMF" ] || { echo "OVMF not found — install ovmf package"; exit 1; }
    echo "Booting ISO (UEFI, systemd-boot)..."
    echo "  → Ctrl-a x to quit QEMU"
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
    OVMF=""
    for candidate in \
        /usr/share/OVMF/OVMF_CODE.fd \
        /usr/share/edk2/ovmf/OVMF_CODE.fd \
        /home/linuxbrew/.linuxbrew/Cellar/qemu/*/share/qemu/edk2-x86_64-code.fd; do
        # shellcheck disable=SC2086
        for f in $candidate; do
            [ -f "$f" ] && OVMF="$f" && break 2
        done
    done
    [ -n "$OVMF" ] || { echo "OVMF not found — install ovmf package"; exit 1; }
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
_install-golangci-lint:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ -x "{{GOLANGCI_LINT}}" ]]; then
        ver=$("{{GOLANGCI_LINT}}" --version 2>&1 | grep -oP 'version \K[0-9.]+' || true)
        [[ "$ver" == "{{GOLANGCI_LINT_VERSION}}" ]] && exit 0
        echo "golangci-lint version mismatch (got $ver, want {{GOLANGCI_LINT_VERSION}}) — reinstalling"
    fi
    mkdir -p .tools
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
    ARCHIVE="golangci-lint-{{GOLANGCI_LINT_VERSION}}-${OS}-${ARCH}.tar.gz"
    URL="https://github.com/golangci/golangci-lint/releases/download/v{{GOLANGCI_LINT_VERSION}}/$ARCHIVE"
    echo "Downloading golangci-lint v{{GOLANGCI_LINT_VERSION}}..."
    curl -sSfL "$URL" | tar -xzf - -C .tools \
        --strip-components=1 \
        "golangci-lint-{{GOLANGCI_LINT_VERSION}}-${OS}-${ARCH}/golangci-lint"
    chmod +x "{{GOLANGCI_LINT}}"
    echo "golangci-lint v{{GOLANGCI_LINT_VERSION}} installed to {{GOLANGCI_LINT}}"

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
