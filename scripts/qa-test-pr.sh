#!/usr/bin/env bash
# scripts/qa-test-pr.sh — Knuckle PR ghost testlab harness
#
# Usage:
#   ./scripts/qa-test-pr.sh <PR_NUMBER>
#
# Runs on the dev machine. Builds the PR binary locally, SCPs it to ghost,
# boots a Flatcar VM there, runs tier-appropriate tests, and prints a
# markdown lab report to stdout.
#
# Exit codes:
#   0 — all tests passed
#   1 — tests failed
#   2 — PR is too complex; skipped (human vm-e2e required)

set -euo pipefail

PR=${1:?usage: qa-test-pr.sh <PR_NUMBER>}

GHOST_HOST=192.168.1.102
GHOST_USER=jorge
GHOST="$GHOST_USER@$GHOST_HOST"
GHOST_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
FLATCAR_BASE="/var/tmp/knuckle-test/flatcar_base.img"
WORK_REMOTE="/var/tmp/knuckle-qa-pr-${PR}"
REPORT_FILE="/tmp/knuckle-qa-pr-${PR}-report.md"
START=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

_ghost() { ssh $GHOST_OPTS "$GHOST" "$@"; }
_ghost_scp() { scp $GHOST_OPTS "$@"; }

log() { echo "  [qa] $*" >&2; }

# ── 1. Fetch PR metadata ──────────────────────────────────────────────────────
log "Fetching PR #${PR} metadata..."

TITLE=$(gh pr view "$PR" --repo projectbluefin/knuckle --json title --jq '.title' 2>/dev/null)
BRANCH=$(gh pr view "$PR" --repo projectbluefin/knuckle --json headRefName --jq '.headRefName' 2>/dev/null)
LABELS=$(gh pr view "$PR" --repo projectbluefin/knuckle --json labels \
  --jq '[.labels[].name] | join(", ")' 2>/dev/null || echo "")
SIZE=$(gh pr view "$PR" --repo projectbluefin/knuckle --json labels \
  --jq '[.labels[] | select(.name | startswith("size:")) | .name] | first // ""' 2>/dev/null || echo "")

# ── 2. Complexity gate ────────────────────────────────────────────────────────
DOMAIN_COUNT=$(echo "$LABELS" | tr ',' '\n' | grep -c "domain:" || true)

if [[ "$SIZE" == "size:XL" ]] || [[ "$DOMAIN_COUNT" -gt 4 ]]; then
  log "SKIP: PR #${PR} too complex (size=${SIZE}, domains=${DOMAIN_COUNT})"
  cat <<MSG
## 🧪 Ghost Testlab — PR #${PR} SKIPPED

**Reason:** PR is too complex for automated ghost testing (size: ${SIZE}, domains touched: ${DOMAIN_COUNT}).

**Required:** Human `just vm-e2e` verification on ghost before merge.
MSG
  exit 2
fi

# ── 3. Determine test tier ────────────────────────────────────────────────────
TIER=0
echo "$LABELS" | grep -qE "domain:probe|domain:tui|domain:security"     && TIER=1
echo "$LABELS" | grep -qE "domain:install|domain:headless|domain:ignition" && TIER=2
echo "$LABELS" | grep -q "domain:iso"                                       && TIER=3

log "PR #${PR}: \"${TITLE}\" | labels: ${LABELS} | tier: ${TIER}"

# ── 4. Build from PR head ─────────────────────────────────────────────────────
log "Fetching PR head..."
PREV_BRANCH=$(git branch --show-current 2>/dev/null || echo "main")

git fetch upstream "pull/${PR}/head:pr${PR}-qa" -q 2>/dev/null
HEAD_SHA=$(git rev-parse "pr${PR}-qa" | head -c 12)

git stash -q 2>/dev/null || true
git checkout "pr${PR}-qa" -q

log "Building from ${HEAD_SHA}..."
BUILD_OUT=$(just build 2>&1) && BUILD_RC=0 || BUILD_RC=$?

git checkout "$PREV_BRANCH" -q 2>/dev/null || true
git stash pop -q 2>/dev/null || true

if [[ $BUILD_RC -ne 0 ]]; then
  log "BUILD FAILED"
  cat > "$REPORT_FILE" <<EOF
## 🧪 Ghost Testlab Report — PR #${PR}
**Branch:** \`${BRANCH}\`  **Date:** ${START}

### ⛔ Build Failed
\`\`\`
${BUILD_OUT}
\`\`\`
**Verdict: ❌ BUILD FAIL**
EOF
  cat "$REPORT_FILE"
  exit 1
fi

BINARY_SHA=$(sha256sum bin/knuckle | cut -c1-12)
log "Binary built: sha256=${BINARY_SHA}"

# ── 5. Tier 0 — CI gate on dev machine ───────────────────────────────────────
log "Running Tier 0 (just ci)..."
git checkout "pr${PR}-qa" -q
UNIT_OUT=$(just ci 2>&1) && UNIT_RC=0 || UNIT_RC=$?
git checkout "$PREV_BRANCH" -q 2>/dev/null || true

UNIT_SUMMARY=$(echo "$UNIT_OUT" | grep -E "^ok |^FAIL|✅|PASS|error:" | tail -20)

# ── 6. Start report ───────────────────────────────────────────────────────────
FLATCAR_VER=$(_ghost "cat /etc/os-release 2>/dev/null | grep -m1 VERSION_ID= | cut -d= -f2" 2>/dev/null || echo "unknown")

cat > "$REPORT_FILE" <<EOF
## 🧪 Ghost Testlab Report — PR #${PR}

**Branch:** \`${BRANCH}\`  **Commit:** \`${HEAD_SHA}\`
**PR:** ${TITLE}
**Flatcar:** ${FLATCAR_VER}  **Date:** ${START}
**Labels:** ${LABELS}  **Tier:** ${TIER}

### Tier 0 — CI gate (dev machine)

\`\`\`
${UNIT_SUMMARY}
\`\`\`

EOF

if [[ $UNIT_RC -ne 0 ]]; then
  echo "**⛔ TIER 0 FAILED — ghost tests skipped**" >> "$REPORT_FILE"
  echo "" >> "$REPORT_FILE"
  echo "<details><summary>Full CI output</summary>" >> "$REPORT_FILE"
  echo "" >> "$REPORT_FILE"
  echo '```' >> "$REPORT_FILE"
  echo "$UNIT_OUT" | tail -40 >> "$REPORT_FILE"
  echo '```' >> "$REPORT_FILE"
  echo "</details>" >> "$REPORT_FILE"
  echo "" >> "$REPORT_FILE"
  echo "**Verdict: ❌ TIER 0 FAIL**" >> "$REPORT_FILE"
  cat "$REPORT_FILE"
  exit 1
fi

echo "**✅ TIER 0 PASS**" >> "$REPORT_FILE"

[[ $TIER -eq 0 ]] && {
  echo "" >> "$REPORT_FILE"
  echo "**Verdict: ✅ PASS** (unit tests only — no VM tests needed for this label set)" >> "$REPORT_FILE"
  cat "$REPORT_FILE"
  exit 0
}

# ── 7. Ghost VM setup ─────────────────────────────────────────────────────────
log "Setting up ghost VM (tier ${TIER})..."

# SCP binary
_ghost "mkdir -p ${WORK_REMOTE}"
_ghost_scp $GHOST_OPTS bin/knuckle "$GHOST:${WORK_REMOTE}/knuckle"
log "Binary uploaded to ghost"

# Find free port
PORT=$(_ghost "
  for p in \$(seq 2300 2315); do
    ss -tln 2>/dev/null | grep -q \":\${p} \" || { echo \$p; exit 0; }
  done
  echo NOFREEPORT
")

if [[ "$PORT" == "NOFREEPORT" ]]; then
  log "No free port in 2300-2315 on ghost"
  echo "**⛔ No free port on ghost — kill stale VMs and retry**" >> "$REPORT_FILE"
  cat "$REPORT_FILE"
  exit 1
fi
log "Allocated port ${PORT} on ghost"

# Boot installer VM
VM_RC=0
VM_BOOT=$(_ghost "
  set -euo pipefail
  WORK=${WORK_REMOTE}
  PORT=${PORT}
  SO='-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR'

  # Fresh overlays
  rm -f \$WORK/boot.qcow2 \$WORK/target.qcow2
  qemu-img create -f qcow2 -b ${FLATCAR_BASE} -F qcow2 \$WORK/boot.qcow2 >/dev/null
  qemu-img create -f qcow2 \$WORK/target.qcow2 20G >/dev/null

  # Ignition with ghost SSH key
  SSH_KEY=\$(cat ~/.ssh/id_ed25519.pub)
  printf '{\"ignition\":{\"version\":\"3.4.0\"},\"passwd\":{\"users\":[{\"name\":\"core\",\"sshAuthorizedKeys\":[\"%s\"]}]},\"systemd\":{\"units\":[{\"name\":\"sshd.service\",\"enabled\":true}]}}\n' \"\$SSH_KEY\" > \$WORK/config.ign

  qemu-system-x86_64 \
    -m 2048 -smp 2 -enable-kvm -cpu host \
    -drive if=virtio,file=\$WORK/boot.qcow2,format=qcow2 \
    -drive if=virtio,file=\$WORK/target.qcow2,format=qcow2 \
    -fw_cfg name=opt/org.flatcar-linux/config,file=\$WORK/config.ign \
    -net nic,model=virtio -net user,hostfwd=tcp::\${PORT}-:22 \
    -display none -daemonize -pidfile \$WORK/qemu.pid \
    -serial file:\$WORK/serial.log >/dev/null 2>&1

  ok=0
  for i in \$(seq 1 20); do
    ssh \$SO -o ConnectTimeout=2 -p \$PORT core@127.0.0.1 true 2>/dev/null && ok=1 && break
    sleep 2
  done

  if [ \$ok -ne 1 ]; then
    echo 'VM BOOT TIMEOUT — last serial lines:'
    tail -10 \$WORK/serial.log 2>/dev/null || true
    kill \$(cat \$WORK/qemu.pid 2>/dev/null) 2>/dev/null || true
    exit 1
  fi
  echo VM_READY
" 2>&1) || VM_RC=$?

if [[ $VM_RC -ne 0 ]] || ! echo "$VM_BOOT" | grep -q "VM_READY"; then
  log "VM boot failed"
  echo "" >> "$REPORT_FILE"
  echo "### Tier 1 — Ghost VM boot" >> "$REPORT_FILE"
  echo '```' >> "$REPORT_FILE"
  echo "$VM_BOOT" >> "$REPORT_FILE"
  echo '```' >> "$REPORT_FILE"
  echo "**⛔ VM BOOT FAILED**" >> "$REPORT_FILE"
  echo "" >> "$REPORT_FILE"
  echo "**Verdict: ❌ FAIL**" >> "$REPORT_FILE"
  cat "$REPORT_FILE"
  exit 1
fi
log "VM booted on ghost:${PORT}"

# SCP binary into VM
_ghost "
  scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR \
    -P ${PORT} ${WORK_REMOTE}/knuckle core@127.0.0.1:/tmp/knuckle
  ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR \
    -p ${PORT} core@127.0.0.1 'chmod +x /tmp/knuckle'
"

# ── 8. Tier 1 — tool check + dry-run ─────────────────────────────────────────
log "Tier 1: tool check + dry-run..."

TIER1_OUT=$(_ghost "
  set -euo pipefail
  SO='-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR'
  VM='ssh \$SO -p ${PORT} core@127.0.0.1'
  echo '=== flatcar version ==='
  \$VM 'cat /etc/os-release | grep -E \"VERSION_ID|PRETTY_NAME\"'
  echo '=== util-linux versions ==='
  \$VM 'sfdisk --version && wipefs --version'
  echo '=== sfdisk --relocate support ==='
  \$VM 'sfdisk --help 2>&1 | grep -o -- \"--relocate\" && echo \"present\" || echo \"MISSING\"'
  echo '=== headless --dry-run ==='
  HOST_KEY=\$(cat ~/.ssh/id_ed25519.pub)
  CFG='{\"channel\":\"stable\",\"hostname\":\"qa-pr-${PR}\",\"timezone\":\"UTC\",\"network\":{\"mode\":\"dhcp\"},\"users\":[{\"username\":\"core\",\"ssh_keys\":[\"'\$HOST_KEY'\"]}],\"disk\":\"/dev/vdb\",\"update_strategy\":\"off\",\"reboot\":false}'
  echo \"\$CFG\" | \$VM 'cat > /tmp/qa.json'
  \$VM 'sudo /tmp/knuckle --headless --dry-run --config /tmp/qa.json --log-file /tmp/knuckle-qa.log 2>&1' && echo DRY_RUN_PASS || echo DRY_RUN_FAIL
  \$VM 'sudo cat /tmp/knuckle-qa.log 2>/dev/null' || true
" 2>&1) || true

DRY_PASS=$(echo "$TIER1_OUT" | grep -c "DRY_RUN_PASS" || true)

{
  echo ""
  echo "### Tier 1 — Ghost installer VM (port ${PORT})"
  echo ""
  echo '```'
  echo "$TIER1_OUT"
  echo '```'
  echo ""
  [[ $DRY_PASS -gt 0 ]] && echo "**✅ TIER 1 PASS**" || echo "**❌ TIER 1 FAIL**"
} >> "$REPORT_FILE"

if [[ $DRY_PASS -eq 0 ]]; then
  _ghost "kill \$(cat ${WORK_REMOTE}/qemu.pid 2>/dev/null) 2>/dev/null || true" 2>/dev/null || true
  echo "" >> "$REPORT_FILE"
  echo "**Verdict: ❌ FAIL**" >> "$REPORT_FILE"
  cat "$REPORT_FILE"
  exit 1
fi

[[ $TIER -lt 2 ]] && {
  _ghost "kill \$(cat ${WORK_REMOTE}/qemu.pid 2>/dev/null) 2>/dev/null || true" 2>/dev/null || true
  echo "" >> "$REPORT_FILE"
  echo "**Verdict: ✅ PASS**" >> "$REPORT_FILE"
  cat "$REPORT_FILE"
  exit 0
}

# ── 9. Tier 2 — real headless install ────────────────────────────────────────
log "Tier 2: real headless install..."

TIER2_OUT=$(_ghost "
  set -euo pipefail
  SO='-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR'
  VM='ssh \$SO -p ${PORT} core@127.0.0.1'
  echo '=== headless install (real flatcar-install) ==='
  \$VM 'sudo /tmp/knuckle --headless --config /tmp/qa.json --log-file /tmp/knuckle-install.log 2>&1' && echo INSTALL_PASS || echo INSTALL_FAIL
  echo '=== knuckle.log (last 30 lines) ==='
  \$VM 'sudo cat /tmp/knuckle-install.log 2>/dev/null | tail -30' || true
" 2>&1) || true

INSTALL_PASS=$(echo "$TIER2_OUT" | grep -c "INSTALL_PASS" || true)

{
  echo ""
  echo "### Tier 2 — Ghost headless install"
  echo ""
  echo '```'
  echo "$TIER2_OUT"
  echo '```'
  echo ""
  [[ $INSTALL_PASS -gt 0 ]] && echo "**✅ TIER 2 PASS**" || echo "**❌ TIER 2 FAIL**"
} >> "$REPORT_FILE"

_ghost "kill \$(cat ${WORK_REMOTE}/qemu.pid 2>/dev/null) 2>/dev/null || true" 2>/dev/null || true

VERDICT_PASS=$INSTALL_PASS
[[ $TIER -lt 2 ]] && VERDICT_PASS=1

{
  echo ""
  if [[ $VERDICT_PASS -gt 0 ]]; then
    echo "**Verdict: ✅ PASS** — all tiers passed, ready to merge"
  else
    echo "**Verdict: ❌ FAIL** — see above"
  fi
} >> "$REPORT_FILE"

cat "$REPORT_FILE"
[[ $VERDICT_PASS -gt 0 ]] && exit 0 || exit 1
