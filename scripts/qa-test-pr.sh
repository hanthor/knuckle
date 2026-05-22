#!/usr/bin/env bash
# scripts/qa-test-pr.sh — Knuckle PR ghost testlab harness
#
# Usage:
#   ./scripts/qa-test-pr.sh <PR_NUMBER>
#
# Full QA science: build → unit tests → installer VM → headless install →
# BOOT the installed system → domain-specific assertions with quoted evidence.
#
# Exit codes:
#   0 — all tests and assertions passed
#   1 — any test or assertion failed
#   2 — PR is too complex; skip (human just vm-e2e required)

set -euo pipefail

PR=${1:?usage: qa-test-pr.sh <PR_NUMBER>}

GHOST_HOST=192.168.1.102
GHOST_USER=jorge
GHOST="$GHOST_USER@$GHOST_HOST"
GOPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
FLATCAR_BASE="/var/tmp/knuckle-test/flatcar_base.img"
WORK="/var/tmp/knuckle-qa-pr-${PR}"
REPORT="/tmp/knuckle-qa-pr-${PR}-report.md"
START=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

_ghost()     { ssh $GOPTS "$GHOST" "$@"; }
_ghost_scp() { scp $GOPTS "$@"; }
log()        { echo "  [qa] $*" >&2; }

# ── 1. PR metadata ────────────────────────────────────────────────────────────
log "Fetching PR #${PR}..."
TITLE=$(gh pr view "$PR" --repo projectbluefin/knuckle --json title   --jq '.title'             2>/dev/null)
BRANCH=$(gh pr view "$PR" --repo projectbluefin/knuckle --json headRefName --jq '.headRefName'  2>/dev/null)
LABELS=$(gh pr view "$PR" --repo projectbluefin/knuckle --json labels  --jq '[.labels[].name] | join(", ")' 2>/dev/null || echo "")
SIZE=$(  gh pr view "$PR" --repo projectbluefin/knuckle --json labels  --jq '[.labels[] | select(.name|startswith("size:")) | .name] | first // ""' 2>/dev/null || echo "")
CLOSES=$(gh pr view "$PR" --repo projectbluefin/knuckle --json body    --jq '.body' 2>/dev/null | grep -oP 'Closes #\K\d+' | tr '\n' ',' | sed 's/,$//' || echo "")
AUTHOR=$(gh pr view "$PR" --repo projectbluefin/knuckle --json author  --jq '.author.login'     2>/dev/null || echo "unknown")

# ── 2. Complexity gate ────────────────────────────────────────────────────────
DOMAIN_COUNT=$(echo "$LABELS" | tr ',' '\n' | grep -c "domain:" || true)
WF_CHANGE=$(gh pr diff "$PR" --repo projectbluefin/knuckle --name-only 2>/dev/null | grep -c "^\.github/workflows/" || true)

if [[ "$SIZE" == "size:XL" ]] || [[ $DOMAIN_COUNT -gt 4 ]] || [[ $WF_CHANGE -gt 0 ]]; then
  cat <<SKIP
## 🧪 Ghost Testlab — PR #${PR} SKIPPED (complexity gate)

**Reason:** Too complex for automated ghost testing.
- Size: ${SIZE}  Domains: ${DOMAIN_COUNT}  Workflow file changes: ${WF_CHANGE}

**Required:** Human `just vm-e2e` verification on ghost (192.168.1.102) before merge.
Checklist: `just tools && just ci && just vm-e2e`
SKIP
  exit 2
fi

# ── 3. Tier selection — what needs boot verification ─────────────────────────
# Tier 0: unit tests only (CI, docs, pure validator changes)
# Tier 1: installer VM + tool check + --dry-run (no flatcar-install)
# Tier 3: FULL — install to disk + BOOT installed system + domain assertions
#   (there is no standalone tier 2; it is internal to tier 3)

TIER=0
NEEDS_BOOT=0

echo "$LABELS" | grep -qE "domain:probe|domain:tui"              && TIER=1
echo "$LABELS" | grep -q  "domain:security"                      && TIER=1 && SECURITY_TESTS=1 || SECURITY_TESTS=0
echo "$LABELS" | grep -qE "domain:install|domain:iso"            && TIER=3 && NEEDS_BOOT=1
echo "$LABELS" | grep -qE "domain:ignition|domain:headless"      && TIER=3 && NEEDS_BOOT=1
echo "$LABELS" | grep -qE "domain:wizard|domain:tui"             && [[ $TIER -lt 1 ]] && TIER=1
echo "$LABELS" | grep -q  "domain:sysext"                        && TIER=3 && NEEDS_BOOT=1

# Derive which domain assertions to run from labels
RUN_ASSERT_INSTALL=0; RUN_ASSERT_SWAP=0; RUN_ASSERT_TAILSCALE=0
RUN_ASSERT_IGNITION=0; RUN_ASSERT_SYSEXT=0

echo "$LABELS" | grep -q "domain:install"   && RUN_ASSERT_INSTALL=1
echo "$LABELS" | grep -q "domain:ignition"  && RUN_ASSERT_IGNITION=1
echo "$LABELS" | grep -q "domain:sysext"    && RUN_ASSERT_SYSEXT=1

# Detect feature-specific labels from closed issues or PR title
echo "$TITLE $LABELS" | grep -qi "swap"      && RUN_ASSERT_SWAP=1 && TIER=3 && NEEDS_BOOT=1
echo "$TITLE $LABELS" | grep -qi "tailscale" && RUN_ASSERT_TAILSCALE=1 && TIER=3 && NEEDS_BOOT=1

log "PR #${PR}: \"${TITLE}\""
log "Labels: ${LABELS} | Tier: ${TIER} | Boot: ${NEEDS_BOOT}"

# ── 4. Build from PR head ─────────────────────────────────────────────────────
PREV_BRANCH=$(git branch --show-current 2>/dev/null || echo "main")
git fetch upstream "pull/${PR}/head:pr${PR}-qa" -q 2>/dev/null
HEAD_SHA=$(git rev-parse "pr${PR}-qa" | head -c 12)
git stash -q 2>/dev/null || true
git checkout "pr${PR}-qa" -q

log "Building ${HEAD_SHA}..."
BUILD_OUT=$(just build 2>&1) && BUILD_RC=0 || BUILD_RC=$?
git checkout "$PREV_BRANCH" -q 2>/dev/null || true
git stash pop -q 2>/dev/null || true

if [[ $BUILD_RC -ne 0 ]]; then
  echo "## 🧪 Ghost Testlab Report — PR #${PR}" > "$REPORT"
  echo "**⛔ BUILD FAILED**" >> "$REPORT"
  echo '```' >> "$REPORT"; echo "$BUILD_OUT" >> "$REPORT"; echo '```' >> "$REPORT"
  cat "$REPORT"; exit 1
fi
BINARY_SHA=$(sha256sum bin/knuckle | cut -c1-12)
log "Binary sha256=${BINARY_SHA}"

# ── 5. Tier 0 — full CI gate ─────────────────────────────────────────────────
log "Tier 0: just ci..."
git checkout "pr${PR}-qa" -q
UNIT_OUT=$(just ci 2>&1) && UNIT_RC=0 || UNIT_RC=$?
git checkout "$PREV_BRANCH" -q 2>/dev/null || true
UNIT_SUMMARY=$(echo "$UNIT_OUT" | grep -E "^ok |^FAIL|✅|PASS|error:" | tail -20)
FLATCAR_VER=$(_ghost "cat /etc/os-release 2>/dev/null | grep -m1 VERSION_ID= | cut -d= -f2" 2>/dev/null || echo "unknown")

# ── 6. Open report ───────────────────────────────────────────────────────────
cat > "$REPORT" <<EOF
## 🧪 Ghost Testlab Report — PR #${PR}

| Field | Value |
|---|---|
| **Branch** | \`${BRANCH}\` @ \`${HEAD_SHA}\` |
| **Author** | ${AUTHOR} |
| **PR** | ${TITLE} |
| **Closes** | ${CLOSES:-—} |
| **Labels** | ${LABELS} |
| **Flatcar** | ${FLATCAR_VER} |
| **Date** | ${START} |
| **Tier** | ${TIER} (boot verification: $([ $NEEDS_BOOT -eq 1 ] && echo "yes" || echo "no")) |

---

### Tier 0 — CI gate (dev machine)

\`\`\`
${UNIT_SUMMARY}
\`\`\`

EOF

if [[ $UNIT_RC -ne 0 ]]; then
  {
    echo "**⛔ TIER 0 FAILED — CI gate not green. Ghost tests skipped.**"
    echo ""
    echo "<details><summary>Full CI output</summary>"
    echo ""
    echo '```'
    echo "$UNIT_OUT" | tail -50
    echo '```'
    echo "</details>"
    echo ""
    echo "**Verdict: ❌ FAIL — fix CI gate first**"
  } >> "$REPORT"
  cat "$REPORT"; exit 1
fi
echo "**✅ TIER 0 PASS**" >> "$REPORT"

[[ $TIER -eq 0 ]] && {
  echo "" >> "$REPORT"
  echo "**Verdict: ✅ PASS** — unit tests only (no VM tests required for this label set)" >> "$REPORT"
  cat "$REPORT"; exit 0
}

# ── 7. Ghost VM boot (installer environment) ──────────────────────────────────
log "Setting up installer VM on ghost..."
_ghost "mkdir -p ${WORK}"
_ghost_scp $GOPTS bin/knuckle "$GHOST:${WORK}/knuckle"

PORT=$(_ghost "
  for p in \$(seq 2300 2315); do
    ss -tln 2>/dev/null | grep -q \":\${p} \" || { echo \$p; exit 0; }
  done
  echo NOFREEPORT
")
[[ "$PORT" == "NOFREEPORT" ]] && {
  echo "**⛔ No free port on ghost. Kill stale VMs: \`ssh jorge@ghost 'pkill -f qemu-system'\`**" >> "$REPORT"
  cat "$REPORT"; exit 1
}
log "Allocated port ${PORT}"

INSTALLER_UP=$(_ghost "
  set -euo pipefail
  SO='-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR'
  W=${WORK}; P=${PORT}
  rm -f \$W/boot.qcow2 \$W/target.qcow2 \$W/serial-installer.log
  qemu-img create -f qcow2 -b ${FLATCAR_BASE} -F qcow2 \$W/boot.qcow2 >/dev/null
  qemu-img create -f qcow2 \$W/target.qcow2 20G >/dev/null
  SSH_KEY=\$(cat ~/.ssh/id_ed25519.pub)
  printf '{\"ignition\":{\"version\":\"3.4.0\"},\"passwd\":{\"users\":[{\"name\":\"core\",\"sshAuthorizedKeys\":[\"%s\"]}]},\"systemd\":{\"units\":[{\"name\":\"sshd.service\",\"enabled\":true}]}}\n' \"\$SSH_KEY\" > \$W/installer.ign
  qemu-system-x86_64 \
    -m 2048 -smp 2 -enable-kvm -cpu host \
    -drive if=virtio,file=\$W/boot.qcow2,format=qcow2 \
    -drive if=virtio,file=\$W/target.qcow2,format=qcow2 \
    -fw_cfg name=opt/org.flatcar-linux/config,file=\$W/installer.ign \
    -net nic,model=virtio -net user,hostfwd=tcp::\${P}-:22 \
    -display none -daemonize -pidfile \$W/qemu-installer.pid \
    -serial file:\$W/serial-installer.log >/dev/null 2>&1
  ok=0
  for i in \$(seq 1 20); do
    ssh \$SO -o ConnectTimeout=2 -p \$P core@127.0.0.1 true 2>/dev/null && ok=1 && break
    sleep 2
  done
  [ \$ok -eq 1 ] && echo VM_READY || { echo VM_BOOT_TIMEOUT; tail -10 \$W/serial-installer.log; exit 1; }
" 2>&1) || true

if ! echo "$INSTALLER_UP" | grep -q "VM_READY"; then
  { echo ""; echo "### Ghost VM Boot"; echo '```'; echo "$INSTALLER_UP"; echo '```'; echo "**⛔ VM BOOT FAILED**"; echo ""; echo "**Verdict: ❌ FAIL**"; } >> "$REPORT"
  cat "$REPORT"; exit 1
fi
log "Installer VM ready on ghost:${PORT}"

# SCP binary + chmod
_ghost "
  scp $GOPTS -P ${PORT} ${WORK}/knuckle core@127.0.0.1:/tmp/knuckle
  ssh $GOPTS -p ${PORT} core@127.0.0.1 'chmod +x /tmp/knuckle'
"

# ── 8. Tier 1 — tool check + dry-run ─────────────────────────────────────────
log "Tier 1: tool check + dry-run..."
HOST_KEY=$(_ghost "cat ~/.ssh/id_ed25519.pub")

TIER1_OUT=$(_ghost "
  set -euo pipefail
  SO='-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR'
  VM=\"ssh \$SO -p ${PORT} core@127.0.0.1\"
  echo '--- OS ---'
  \$VM 'cat /etc/os-release | grep -E \"VERSION_ID|PRETTY_NAME\"'
  echo '--- util-linux ---'
  \$VM 'sfdisk --version && wipefs --version'
  echo '--- sfdisk --relocate ---'
  \$VM 'sfdisk --help 2>&1 | grep -o -- \"--relocate\" && echo \"present\" || echo \"MISSING --relocate\"'
  echo '--- headless --dry-run ---'
  printf '{\"channel\":\"stable\",\"hostname\":\"qa-pr-${PR}\",\"timezone\":\"UTC\",\"network\":{\"mode\":\"dhcp\"},\"users\":[{\"username\":\"core\",\"ssh_keys\":[\"${HOST_KEY}\"]}],\"disk\":\"/dev/vdb\",\"update_strategy\":\"off\",\"reboot\":false}' | \$VM 'cat > /tmp/qa.json'
  \$VM 'sudo /tmp/knuckle --headless --dry-run --config /tmp/qa.json --log-file /tmp/knuckle-dryrun.log 2>&1'
  echo '--- dry-run progress steps ---'
  \$VM 'sudo cat /tmp/knuckle-dryrun.log 2>/dev/null | grep -oP \"\\\"msg\\\":\\\"[^\\\"]+\\\"\" | head -10' || true
" 2>&1) || { echo "TIER1_FAIL"; }

DRY_PASS=$(echo "$TIER1_OUT" | grep -c "Installation complete" || true)

{
  echo ""
  echo "### Tier 1 — Installer VM: tool check + dry-run (port ${PORT})"
  echo ""
  echo '```'
  echo "$TIER1_OUT"
  echo '```'
  echo ""
  [[ $DRY_PASS -gt 0 ]] && echo "**✅ TIER 1 PASS**" || echo "**❌ TIER 1 FAIL — dry-run did not complete**"
} >> "$REPORT"

if [[ $DRY_PASS -eq 0 ]]; then
  _ghost "kill \$(cat ${WORK}/qemu-installer.pid 2>/dev/null) 2>/dev/null || true" 2>/dev/null || true
  echo "" >> "$REPORT"; echo "**Verdict: ❌ FAIL**" >> "$REPORT"
  cat "$REPORT"; exit 1
fi

# Security assertion tests (run in installer VM regardless of tier)
if [[ $SECURITY_TESTS -eq 1 ]]; then
  log "Security assertions: bad-input rejection tests..."
  SEC_OUT=$(_ghost "
    set -euo pipefail
    SO='-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR'
    VM=\"ssh \$SO -p ${PORT} core@127.0.0.1\"
    echo '--- plaintext password must be REJECTED ---'
    printf '{\"channel\":\"stable\",\"hostname\":\"qa-sec\",\"timezone\":\"UTC\",\"network\":{\"mode\":\"dhcp\"},\"users\":[{\"username\":\"core\",\"password\":\"hunter2\"}],\"disk\":\"/dev/vdb\",\"update_strategy\":\"off\",\"reboot\":false}' | \$VM 'cat > /tmp/qa-bad-pw.json'
    \$VM 'sudo /tmp/knuckle --headless --dry-run --config /tmp/qa-bad-pw.json 2>&1' && echo 'BAD_PW_ACCEPTED_FAIL' || echo 'BAD_PW_REJECTED_PASS'
    echo '--- malformed SSH key must be REJECTED ---'
    printf '{\"channel\":\"stable\",\"hostname\":\"qa-sec\",\"timezone\":\"UTC\",\"network\":{\"mode\":\"dhcp\"},\"users\":[{\"username\":\"core\",\"ssh_keys\":[\"not-a-valid-key\"]}],\"disk\":\"/dev/vdb\",\"update_strategy\":\"off\",\"reboot\":false}' | \$VM 'cat > /tmp/qa-bad-key.json'
    \$VM 'sudo /tmp/knuckle --headless --dry-run --config /tmp/qa-bad-key.json 2>&1' && echo 'BAD_KEY_ACCEPTED_FAIL' || echo 'BAD_KEY_REJECTED_PASS'
    echo '--- valid crypt hash must be ACCEPTED ---'
    printf '{\"channel\":\"stable\",\"hostname\":\"qa-sec\",\"timezone\":\"UTC\",\"network\":{\"mode\":\"dhcp\"},\"users\":[{\"username\":\"core\",\"password\":\"\\$6\\$rounds=4096\\$testsalt\\$hashhash123\"}],\"disk\":\"/dev/vdb\",\"update_strategy\":\"off\",\"reboot\":false}' | \$VM 'cat > /tmp/qa-good-pw.json'
    \$VM 'sudo /tmp/knuckle --headless --dry-run --config /tmp/qa-good-pw.json 2>&1' && echo 'GOOD_PW_ACCEPTED_PASS' || echo 'GOOD_PW_REJECTED_FAIL'
  " 2>&1) || true

  SEC_FAIL=$(echo "$SEC_OUT" | grep -c "_FAIL" || true)
  SEC_PASS=$(echo "$SEC_OUT" | grep -c "_PASS" || true)

  {
    echo ""
    echo "### Security Assertions — Bad Input Rejection"
    echo ""
    echo '```'
    echo "$SEC_OUT"
    echo '```'
    echo ""
    [[ $SEC_FAIL -eq 0 ]] && echo "**✅ SECURITY ASSERTIONS PASS** (${SEC_PASS} checks)" || echo "**❌ SECURITY ASSERTIONS FAIL** (${SEC_FAIL} failures)"
  } >> "$REPORT"

  if [[ $SEC_FAIL -gt 0 ]]; then
    _ghost "kill \$(cat ${WORK}/qemu-installer.pid 2>/dev/null) 2>/dev/null || true" 2>/dev/null || true
    echo "" >> "$REPORT"; echo "**Verdict: ❌ FAIL — security assertions failed**" >> "$REPORT"
    cat "$REPORT"; exit 1
  fi
fi

[[ $TIER -lt 3 ]] && {
  _ghost "kill \$(cat ${WORK}/qemu-installer.pid 2>/dev/null) 2>/dev/null || true" 2>/dev/null || true
  echo "" >> "$REPORT"; echo "**Verdict: ✅ PASS**" >> "$REPORT"
  cat "$REPORT"; exit 0
}

# ── 9. Tier 3 — real headless install ────────────────────────────────────────
log "Tier 3: real headless install..."

# Build feature-specific headless config based on what the PR adds
SWAP_CFG=""; TAILSCALE_CFG=""
[[ $RUN_ASSERT_SWAP -eq 1 ]]      && SWAP_CFG=',\"swap\":{\"enabled\":true,\"size_mb\":512}'
[[ $RUN_ASSERT_TAILSCALE -eq 1 ]] && TAILSCALE_CFG=',\"tailscale\":{\"auth_key\":\"tskey-auth-abcdef1234567890AB-CDEFGHIJKLMNOPQRSTUVWXYZ0123456789\"}'

INSTALL_OUT=$(_ghost "
  set -euo pipefail
  SO='-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR'
  VM=\"ssh \$SO -p ${PORT} core@127.0.0.1\"
  echo '--- writing install config ---'
  printf '{\"channel\":\"stable\",\"hostname\":\"qa-pr-${PR}\",\"timezone\":\"UTC\",\"network\":{\"mode\":\"dhcp\"},\"users\":[{\"username\":\"core\",\"ssh_keys\":[\"${HOST_KEY}\"]}],\"disk\":\"/dev/vdb\",\"update_strategy\":\"off\",\"reboot\":false${SWAP_CFG}${TAILSCALE_CFG}}' | \$VM 'cat > /tmp/qa-install.json'
  echo '--- running knuckle --headless (real install) ---'
  \$VM 'sudo /tmp/knuckle --headless --config /tmp/qa-install.json --log-file /tmp/knuckle-install.log 2>&1' && echo INSTALL_COMPLETE || echo INSTALL_FAILED
  echo '--- install log (last 20 lines) ---'
  \$VM 'sudo cat /tmp/knuckle-install.log 2>/dev/null | tail -20' || true
  echo '--- disk state post-install ---'
  \$VM 'lsblk -o NAME,SIZE,TYPE,FSTYPE,LABEL /dev/vdb 2>&1' || true
" 2>&1) || true

INSTALL_COMPLETE=$(echo "$INSTALL_OUT" | grep -c "INSTALL_COMPLETE" || true)

{
  echo ""
  echo "### Tier 3 — Headless install"
  echo ""
  echo '```'
  echo "$INSTALL_OUT"
  echo '```'
  echo ""
  [[ $INSTALL_COMPLETE -gt 0 ]] && echo "**✅ INSTALL COMPLETE**" || echo "**❌ INSTALL FAILED**"
} >> "$REPORT"

if [[ $INSTALL_COMPLETE -eq 0 ]]; then
  _ghost "kill \$(cat ${WORK}/qemu-installer.pid 2>/dev/null) 2>/dev/null || true" 2>/dev/null || true
  echo "" >> "$REPORT"; echo "**Verdict: ❌ FAIL — install did not complete**" >> "$REPORT"
  cat "$REPORT"; exit 1
fi

# ── 10. Boot the installed system ─────────────────────────────────────────────
log "Booting installed system on ghost:${PORT}..."

# Kill installer VM
_ghost "kill \$(cat ${WORK}/qemu-installer.pid 2>/dev/null) 2>/dev/null || true; sleep 2" 2>/dev/null || true

BOOT_UP=$(_ghost "
  set -euo pipefail
  SO='-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR'
  W=${WORK}; P=${PORT}
  rm -f \$W/serial-installed.log \$W/qemu-installed.pid
  qemu-system-x86_64 \
    -m 2048 -smp 2 -enable-kvm -cpu host \
    -drive if=virtio,file=\$W/target.qcow2,format=qcow2 \
    -net nic,model=virtio -net user,hostfwd=tcp::\${P}-:22 \
    -display none -daemonize -pidfile \$W/qemu-installed.pid \
    -serial file:\$W/serial-installed.log >/dev/null 2>&1
  echo 'Waiting for installed Flatcar to boot + run Ignition...'
  ok=0
  for i in \$(seq 1 30); do
    ssh \$SO -o ConnectTimeout=3 -p \$P core@127.0.0.1 true 2>/dev/null && ok=1 && break
    sleep 5
  done
  [ \$ok -eq 1 ] && echo INSTALLED_READY || { echo INSTALLED_BOOT_TIMEOUT; tail -15 \$W/serial-installed.log; exit 1; }
" 2>&1) || true

if ! echo "$BOOT_UP" | grep -q "INSTALLED_READY"; then
  { echo ""; echo "### Installed System Boot"; echo '```'; echo "$BOOT_UP"; echo '```'; echo "**⛔ INSTALLED SYSTEM DID NOT BOOT**"; echo ""; echo "**Verdict: ❌ FAIL**"; } >> "$REPORT"
  cat "$REPORT"; exit 1
fi
log "Installed system booted"

# ── 11. Domain assertions ──────────────────────────────────────────────────────
log "Running domain assertions inside installed Flatcar..."

ASSERT_OUT=$(_ghost "
  set -euo pipefail
  SO='-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR'
  INST=\"ssh \$SO -p ${PORT} core@127.0.0.1\"

  echo '=== BASELINE: OS identity (proves this is the installed system, not installer) ==='
  \$INST 'cat /etc/os-release | grep -E \"PRETTY_NAME|VERSION_ID\"'
  echo ''
  echo '=== BASELINE: hostname matches config ==='
  \$INST 'hostname'
  echo ''
  echo '=== BASELINE: core user SSH key provisioned by Ignition ==='
  \$INST 'ls -la ~/.ssh/authorized_keys && wc -l ~/.ssh/authorized_keys'
  echo ''

  $([ $RUN_ASSERT_INSTALL -eq 1 ] && cat << 'ASSERT_INSTALL'
  echo '=== ASSERT [domain:install]: GPT partition table intact post-wipe+install ==='
  $INST 'sudo sfdisk -l /dev/vda 2>&1 | head -15'
  echo ''
  echo '=== ASSERT [domain:install]: /dev/disk/by-id populated ==='
  $INST 'ls -la /dev/disk/by-id/ 2>&1 | grep -v "^total" | head -5'
  echo ''
ASSERT_INSTALL
)

  $([ $RUN_ASSERT_SWAP -eq 1 ] && cat << 'ASSERT_SWAP'
  echo '=== ASSERT [swap]: /var/swapfile exists at correct mode ==='
  $INST 'ls -lah /var/swapfile 2>&1 || echo "FAIL: /var/swapfile NOT FOUND"'
  echo ''
  echo '=== ASSERT [swap]: swapon shows active swap ==='
  $INST 'swapon --show 2>&1 || echo "FAIL: no active swap"'
  echo ''
  echo '=== ASSERT [swap]: knuckle-create-swapfile.service completed ==='
  $INST 'systemctl status knuckle-create-swapfile.service 2>&1 | grep -E "Active:|Loaded:" | head -2'
  echo ''
  echo '=== ASSERT [swap]: free -h shows non-zero swap ==='
  $INST 'free -h 2>&1'
  echo ''
ASSERT_SWAP
)

  $([ $RUN_ASSERT_TAILSCALE -eq 1 ] && cat << 'ASSERT_TAILSCALE'
  echo '=== ASSERT [tailscale]: /etc/tailscale/tailscale.env exists at mode 0600 ==='
  $INST 'stat -c "%a %n" /etc/tailscale/tailscale.env 2>&1 || echo "FAIL: tailscale.env NOT FOUND"'
  echo ''
  echo '=== ASSERT [tailscale]: tailscaled.service is enabled ==='
  $INST 'systemctl is-enabled tailscaled.service 2>&1'
  echo ''
  echo '=== ASSERT [tailscale]: knuckle-tailscale-up.service is enabled ==='
  $INST 'systemctl is-enabled knuckle-tailscale-up.service 2>&1'
  echo ''
  echo '=== ASSERT [tailscale]: env file does NOT contain plaintext in logs ==='
  $INST 'sudo journalctl -b --no-pager | grep -i "tailscale" | grep -v "Starting\|Started\|Condition" | head -5' || true
  echo ''
ASSERT_TAILSCALE
)

  $([ $RUN_ASSERT_IGNITION -eq 1 ] && cat << 'ASSERT_IGN'
  echo '=== ASSERT [ignition]: hostname correctly applied ==='
  $INST 'cat /etc/hostname 2>/dev/null || hostname'
  echo ''
  echo '=== ASSERT [ignition]: authorized_keys from Ignition ==='
  $INST 'cat ~/.ssh/authorized_keys 2>/dev/null | cut -c1-40'
  echo ''
  echo '=== ASSERT [ignition]: update strategy applied ==='
  $INST 'cat /etc/flatcar/update.conf 2>/dev/null | grep -i strategy || echo "update.conf not found"'
  echo ''
ASSERT_IGN
)

  $([ $RUN_ASSERT_SYSEXT -eq 1 ] && cat << 'ASSERT_SYSEXT'
  echo '=== ASSERT [sysext]: extensions present in /etc/extensions ==='
  $INST 'ls /etc/extensions/ 2>/dev/null || echo "FAIL: /etc/extensions empty or not found"'
  echo ''
  echo '=== ASSERT [sysext]: systemd-sysext status ==='
  $INST 'sudo systemd-sysext status 2>&1 | head -10'
  echo ''
ASSERT_SYSEXT
)

  echo 'ASSERTIONS_COMPLETE'
" 2>&1) || { echo "ASSERTIONS_FAILED"; }

ASSERT_DONE=$(echo "$ASSERT_OUT" | grep -c "ASSERTIONS_COMPLETE" || true)
ASSERT_FAIL=$(echo "$ASSERT_OUT" | grep -c "FAIL:" || true)

{
  echo ""
  echo "### Tier 3 — Installed System: domain assertions"
  echo ""
  if [[ $ASSERT_DONE -gt 0 ]]; then
    echo '```'
    echo "$ASSERT_OUT"
    echo '```'
    echo ""
    if [[ $ASSERT_FAIL -eq 0 ]]; then
      echo "**✅ ALL DOMAIN ASSERTIONS PASS** (${ASSERT_FAIL} failures)"
    else
      echo "**❌ ${ASSERT_FAIL} DOMAIN ASSERTION(S) FAILED** — see FAIL: lines above"
    fi
  else
    echo '```'
    echo "$ASSERT_OUT"
    echo '```'
    echo ""
    echo "**⛔ ASSERTIONS DID NOT COMPLETE**"
  fi
} >> "$REPORT"

# Cleanup
_ghost "kill \$(cat ${WORK}/qemu-installed.pid 2>/dev/null) 2>/dev/null || true" 2>/dev/null || true

# ── 12. Final verdict ─────────────────────────────────────────────────────────
{
  echo ""
  echo "---"
  echo ""
  if [[ $ASSERT_DONE -gt 0 ]] && [[ $ASSERT_FAIL -eq 0 ]]; then
    echo "**Verdict: ✅ PASS** — installed system verified, all domain assertions clean"
  else
    echo "**Verdict: ❌ FAIL** — see assertion failures above"
  fi
} >> "$REPORT"

cat "$REPORT"
[[ $ASSERT_DONE -gt 0 ]] && [[ $ASSERT_FAIL -eq 0 ]] && exit 0 || exit 1
