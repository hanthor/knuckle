# QA Lab Setup

`just qa-pr <PR>` runs a full Flatcar VM install + boot + domain assertions
for any PR. It works on a laptop, on a dedicated test machine, or remotely.

---

## What it needs

- Linux host with KVM (`/dev/kvm` accessible)
- QEMU (`qemu-system-x86_64`)
- A Flatcar QEMU base image (~477 MB)
- `just`, `gh` CLI, standard Go toolchain (for the build step)

---

## Laptop / local setup (simplest)

```bash
# 1. Download the Flatcar QEMU base image (~477 MB, one-time)
mkdir -p /var/tmp/knuckle-test
curl -L https://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_qemu_image.img.bz2 \
  | bunzip2 > /var/tmp/knuckle-test/flatcar_base.img

# 2. Run a PR test (defaults to localhost)
just qa-pr 170

# Artifacts land in .qa/runs/pr-170-TIMESTAMP/
```

That's it. No additional configuration needed.

---

## Remote machine (Jorge's setup — ghost at 192.168.1.102)

The base image and QEMU run on ghost. The build and unit tests still run
locally; only the VM portion runs remotely.

```bash
# Set once in your shell profile or .env:
export QA_HOST=jorge@192.168.1.102
export QA_FLATCAR_BASE=/var/tmp/knuckle-test/flatcar_base.img

# Run exactly the same way:
just qa-pr 170
```

The script SSH-tunnels into the remote host for all VM operations.
`QA_FLATCAR_BASE` must be a path on `QA_HOST`, not on your local machine.

---

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `QA_HOST` | `localhost` | Machine where QEMU runs (`user@host` or `localhost`) |
| `QA_FLATCAR_BASE` | `/var/tmp/knuckle-test/flatcar_base.img` | Path to Flatcar QEMU image **on QA_HOST** |
| `FILE_ISSUES` | `0` | Set to `1` to auto-file GitHub issues on failure |

---

## What the test does

For any PR, `just qa-pr <N>` runs in order:

1. **Build** the binary from the PR's head commit
2. **`just ci`** — unit tests, lint, coverage gate (on local machine)
3. **Boot Flatcar installer VM** on QA_HOST (fresh qcow2 overlay)
4. **Tool check** — sfdisk version, wipefs version, --relocate present
5. **Headless --dry-run** — config generation + Ignition compile, no disk writes
6. **Real headless install** — flatcar-install writes to /dev/vdb
7. **Boot the installed system** — kills installer VM, boots target disk
8. **Domain assertions** — quoted evidence from inside the booted installed Flatcar

Steps 3-8 run on `QA_HOST` via SSH. The assertion script (`assert.sh`) is
generated locally and SCPed to QA_HOST, then into the VM — no heredoc escaping.

---

## Artifacts

Each run saves everything to `.qa/runs/pr-N-TIMESTAMP/`:

```
.qa/runs/pr-170-20260522-193000/
├── report.md              # the full test report (publish to PR with gh pr comment)
├── build.log              # go build output
├── ci.log                 # just ci output
├── knuckle-install.log    # knuckle slog output from inside the VM
└── ghost/                 # all QEMU artifacts fetched from QA_HOST:
    ├── serial-installer.log
    ├── serial-installed.log
    └── ...
```

Failed runs also write `issue-body.md` ready to file:

```bash
gh issue create --repo projectbluefin/knuckle \
  --title "qa: PR #170 — <summary>" \
  --body-file .qa/runs/pr-170-.../issue-body.md
```

Or set `FILE_ISSUES=1` to file automatically.

---

## Minimum host requirements

| Resource | Minimum | Notes |
|---|---|---|
| RAM | 4 GB free | 2 GB for installer VM + 2 GB for installed VM |
| Disk | 5 GB free | ~500 MB install + logs |
| CPU | KVM capable | Software emulation (TCG) works but takes ~5× longer |
| Ports | 2300–2315 free | Script auto-allocates; adjust if you have conflicts |

On a laptop with 8 GB RAM and SSD, a full Tier 3 run takes ~8 minutes.
On ghost (32 cores, NVMe), ~3 minutes.

---

## Adding a new QA host

Set two env vars and run:

```bash
# On the new host: download the base image once
ssh user@newhost "
  mkdir -p /var/tmp/knuckle-test
  curl -L https://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_qemu_image.img.bz2 \
    | bunzip2 > /var/tmp/knuckle-test/flatcar_base.img
"

# On your dev machine:
export QA_HOST=user@newhost
just qa-pr 170
```

The only requirement on the remote host is QEMU + KVM + SSH access.
