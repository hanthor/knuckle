# Knuckle Test Plan

## Product Vision (Clarified)

Knuckle is a **TUI form that generates an Ignition config** and then **hands off to `flatcar-install`** with a polished progress UI. Think: Ubuntu Server installer TUI (subiquity) but for Flatcar Container Linux.

### What Knuckle Does
1. **Form wizard** — collects: channel, network, disk, user/SSH, sysexts, update strategy
2. **Generates Ignition** — produces valid Ignition JSON (via Butane compilation)
3. **Confirmation dialog** — "point of no return" screen showing exactly what will happen
4. **Hands off to `flatcar-install`** — executes the real installer with the generated config
5. **Progress UI** — shows download/write progress with a polished charm.sh progress bar + spinner
6. **Reboot prompt** — on success, offers to reboot the system

### What Knuckle Does NOT Do
- Write its own disk partitioning logic
- Implement its own download/image-write code
- Replace `flatcar-install` — it wraps it

### UX Model (Ubuntu Server / Subiquity Pattern)
```
┌─────────────────────────────────────────────┐
│  🔧 Knuckle — Flatcar Installer             │  ← Fixed header
├─────────────────────────────────────────────┤
│                                             │
│  [Step content: forms, tables, progress]    │  ← Body (scrollable)
│                                             │
├─────────────────────────────────────────────┤
│  ↑↓ navigate • enter confirm • esc back    │  ← Fixed footer/help
└─────────────────────────────────────────────┘
```

---

## charm.sh Component Strategy

Based on exploration of bubbletea examples and huh:

| Step | Component | Pattern |
|---|---|---|
| Welcome, Network, User | `huh.Form` + `huh.Group` | Multi-field forms with validation (burger example) |
| Storage (disk picker) | `bubbles/table` or raw Bubble Tea | Selectable table rows with disk metadata |
| Sysext (multi-select) | `huh.MultiSelect` | Checkbox list with descriptions |
| Update Strategy | `huh.Select` | Radio-button style single select |
| Review | Custom View | Styled summary with Butane preview toggle |
| Confirmation | `huh.Confirm` | "YES to proceed" with danger styling |
| Install Progress | `bubbles/progress` + `bubbles/spinner` | **package-manager pattern** (see below) |
| Done | Custom View | Success message + reboot prompt |

### Install Progress Pattern (from bubbletea/examples/package-manager)

The key insight: use `tea.Printf` to scroll completed steps above the progress bar, with a spinner + progress bar for the current step:

```
✓ Generating Ignition config
✓ Compiling Butane → Ignition JSON
✓ Downloading Flatcar 4152.1.0 (stable)
⣾ Writing to /dev/disk/by-id/ata-VBOX_HARDDISK_VB12345  ━━━━━━━━━━━░░░░░░ 67%
```

This uses:
- `tea.Cmd` returning `installedPkgMsg` when each phase completes
- `progress.Model` for the animated bar
- `spinner.Model` for the current-phase indicator
- `tea.Printf` to print completed items above the TUI (scroll buffer)

### huh.Spinner for Post-Form Processing

After form completion, before install starts:
```go
spinner.New().
    Title("Compiling Ignition config...").
    Action(func(ctx context.Context) error {
        return compilButane(cfg)
    }).
    Run()
```

---

## Test Layers

### Layer 1: Unit Tests (go test, no network, no QEMU)

| Package | What to Test | Coverage Target |
|---|---|---|
| `internal/model` | Type construction, step ordering | ≥90% |
| `internal/validate` | All validators: hostname, CIDR, gateway, SSH key, disk path, timezone | ≥95% |
| `internal/ignition` | Template rendering: DHCP, static, timezone, sysexts, password, combo cases | ≥90% |
| `internal/probe` | JSON parsing from lsblk/ip fixtures | ≥85% |
| `internal/bakery` | Catalog parsing, channel version parsing, tag parsing | ≥85% |
| `internal/runner` | SpyRunner recording, DryRunner behavior | ≥80% |
| `internal/install` | Arg building, ignition file write, cleanup | ≥80% |
| `internal/wizard` | Step transitions, validation gating, consistency checks | ≥85% |
| `internal/tui` | Key handling, field propagation, view rendering | ≥70% |

**Key missing tests to add:**
- `TestGenerateButaneTimezoneAndStatic` — confirmed bug #56
- `TestValidateNetworkRequiresInterface` — bug #67
- `TestHashPasswordErrorHandling` — bug #57
- `TestInstallArgsWithVersionPin`
- `TestWizardSkipsStepsWithIgnitionURL`

### Layer 2: Integration Tests (go test, requires network)

Tagged with `//go:build integration`

| Test | What It Validates |
|---|---|
| `TestBakeryFetchRealCatalog` | GitHub Releases API returns parseable sysext entries |
| `TestChannelFetchRealVersions` | Flatcar release server returns valid version.txt |
| `TestGitHubKeyFetchReal` | Known user (e.g. "torvalds") returns SSH keys |
| `TestButaneCompilationRoundtrip` | Generated Butane YAML → butane CLI → valid Ignition JSON |

### Layer 3: QEMU Live Tests (ghost lab, headless)

These tests boot a real Flatcar VM and exercise knuckle end-to-end.

**Host:** ghost (192.168.1.102), 32 cores, 64GB RAM, headless
**NOT the NUC** — NUC is reserved for dakota.

#### Test Environment Setup

```bash
# On ghost: prepare test infrastructure
ssh jorge@192.168.1.102

# Download Flatcar QEMU image (one-time)
mkdir -p /var/tmp/knuckle-test
cd /var/tmp/knuckle-test
curl -L -o flatcar_base.img.bz2 \
    "https://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_qemu_image.img.bz2"
bunzip2 flatcar_base.img.bz2

# Create a target disk for installation tests
qemu-img create -f qcow2 /var/tmp/knuckle-test/target.qcow2 50G
```

#### QEMU Launch Pattern (Headless — ghost has no monitor)

```bash
QEMU=/usr/bin/qemu-system-x86_64

# Fresh boot disk each test (Ignition only fires on first boot)
cp /var/tmp/knuckle-test/flatcar_base.img /var/tmp/knuckle-test/boot.img

$QEMU \
    -machine q35 -m 4096 -smp 4 -enable-kvm -cpu host \
    -drive if=virtio,file=/var/tmp/knuckle-test/boot.img,format=qcow2 \
    -drive if=virtio,file=/var/tmp/knuckle-test/target.qcow2,format=qcow2 \
    -fw_cfg name=opt/org.flatcar-linux/config,file=/var/tmp/knuckle-test/config.ign \
    -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
    -display none -daemonize \
    -serial file:/var/tmp/knuckle-test/serial.log \
    -pidfile /var/tmp/knuckle-test/qemu.pid \
    -name knuckle-test
```

#### Test Scenarios

| ID | Scenario | Steps | Validation |
|---|---|---|---|
| **E2E-01** | Dry-run full wizard | Deploy knuckle, run `--dry-run`, exercise all steps | Exit 0, no disk writes, Ignition JSON valid |
| **E2E-02** | Generated Ignition boots | Generate Ignition for DHCP+SSH key, boot fresh VM with it | VM boots, SSH works with configured key |
| **E2E-03** | Static network Ignition | Generate static IP config, boot VM | VM gets configured IP, DNS resolves |
| **E2E-04** | Timezone in Ignition | Generate config with timezone, boot | `/etc/localtime` is correct symlink |
| **E2E-05** | Sysext in Ignition | Generate config with docker sysext, boot | `systemd-sysext status` shows extension |
| **E2E-06** | Real install (dry-run verify) | Run knuckle without `--dry-run` against target disk | `flatcar-install` is invoked with correct args |
| **E2E-07** | Install + reboot | Full install to target disk, reboot from target | Target disk boots Flatcar with configured users |
| **E2E-08** | Multi-user config | Generate with 2 users, different SSH keys | Both users can SSH in |
| **E2E-09** | Password-only auth | Generate with password, no SSH keys | `PasswordAuthentication yes` in sshd_config |
| **E2E-10** | Update strategy persists | Generate with `off` strategy, boot | `/etc/flatcar/update.conf` has `REBOOT_STRATEGY=off` |

#### Test Automation Script Pattern

```bash
#!/usr/bin/env bash
# test-e2e-ignition-boots.sh — E2E-02
set -euo pipefail

TEST_DIR=/var/tmp/knuckle-test
KNUCKLE_BIN=/var/tmp/knuckle-test/knuckle-linux-amd64

# 1. Build knuckle
cd ~/src/knuckle  # or wherever the repo is on ghost
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$KNUCKLE_BIN" ./cmd/knuckle

# 2. Generate Ignition JSON (use the library directly via a test helper)
cat > "$TEST_DIR/test-config.json" << 'EOF'
{
  "channel": "stable",
  "hostname": "knuckle-test",
  "network": {"mode": "dhcp"},
  "users": [{"username": "core", "ssh_keys": ["ssh-ed25519 AAAA... test@host"]}]
}
EOF
# Run a Go test helper that generates Ignition from this config
go test -run TestGenerateIgnitionForE2E -v ./internal/ignition/ -config "$TEST_DIR/test-config.json" -output "$TEST_DIR/test.ign"

# 3. Boot VM with generated Ignition
cp "$TEST_DIR/flatcar_base.img" "$TEST_DIR/boot-e2e02.img"
qemu-system-x86_64 \
    -machine q35 -m 2048 -smp 2 -enable-kvm -cpu host \
    -drive if=virtio,file="$TEST_DIR/boot-e2e02.img" \
    -fw_cfg name=opt/org.flatcar-linux/config,file="$TEST_DIR/test.ign" \
    -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
    -display none -daemonize \
    -serial file:"$TEST_DIR/serial-e2e02.log" \
    -pidfile "$TEST_DIR/qemu-e2e02.pid" \
    -name knuckle-e2e02

# 4. Wait for SSH
for i in $(seq 1 60); do
    if ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
        -o ConnectTimeout=2 -p 2222 core@127.0.0.1 true 2>/dev/null; then
        break
    fi
    sleep 2
done

# 5. Validate
ssh -p 2222 core@127.0.0.1 "hostname" | grep -q "knuckle-test" || { echo "FAIL: hostname"; exit 1; }
ssh -p 2222 core@127.0.0.1 "cat /etc/flatcar/update.conf" | grep -q "REBOOT_STRATEGY" || { echo "FAIL: update.conf"; exit 1; }

echo "PASS: E2E-02 — Generated Ignition boots and configures correctly"

# 6. Cleanup
kill "$(cat $TEST_DIR/qemu-e2e02.pid)" 2>/dev/null || true
rm -f "$TEST_DIR/boot-e2e02.img" "$TEST_DIR/qemu-e2e02.pid"
```

---

## Test Priority Order

### Phase 1: Fix Critical Bugs (blocks all testing)
1. Fix #56 (Butane template structure — timezone + static combo)
2. Fix #67 (network validation requires interface)
3. Fix #62 (empty GROUP= in update.conf)
4. Add missing unit tests for the above

### Phase 2: huh.Form Migration + Async Install
1. Add `charmbracelet/huh` dependency
2. Migrate form steps to huh (Welcome, Network, User)
3. Implement async install with progress pattern (#60)
4. Add spinner for Butane compilation phase

### Phase 3: QEMU Integration Tests on Ghost
1. Set up test infrastructure on ghost (`/var/tmp/knuckle-test/`)
2. Implement E2E-01 (dry-run) and E2E-02 (Ignition boots)
3. Add E2E-03 through E2E-05 (network, timezone, sysext)
4. Add E2E-07 (full install + reboot from target disk)

### Phase 4: Polish
1. Implement package-manager style progress bar for install
2. Add reboot prompt on completion
3. Final UX pass: colors, borders, help text
4. Accessibility mode (`--accessible` flag for screen readers)

---

## Ghost Lab Rules (for this project)

- **Use ghost (192.168.1.102) only** — NUC is reserved for dakota
- **Headless QEMU only** — ghost has no monitor, use `-display none`
- **Disk images in `/var/tmp/`** — NOT `/tmp` (16GB tmpfs fills instantly)
- **50GB target disks** — match real-world bare-metal
- **SSH on port 2222** — user NAT forwarding
- **Kill VMs after tests** — don't leave orphan QEMU processes
- **Serial logs** — always capture to `/var/tmp/knuckle-test/serial-*.log` for debugging

---

## Acceptance Criteria for v1.0 Ship

- [ ] All unit tests pass (`go test -race ./...`)
- [ ] `just ci` passes (tidy + lint + test + build)
- [ ] E2E-01 through E2E-05 pass on ghost
- [ ] E2E-07 passes (full install + reboot)
- [ ] huh.Form integrated for all form steps
- [ ] Progress bar animates during `flatcar-install` execution
- [ ] Confirmation dialog clearly shows destructive action
- [ ] `--dry-run` mode works end-to-end without touching disks
- [ ] Generated Ignition passes `butane --strict` validation
- [ ] No x/crypto dependency (use mkpasswd via runner)
- [ ] README accurately reflects current capabilities
