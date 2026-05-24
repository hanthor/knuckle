#!/usr/bin/env bash
# Launch knuckle in a VM for manual TUI recording.
# YOU drive the TUI, asciinema captures it.
#
# Usage:
#   ./scripts/record-demo.sh        # opens Ghostty with recording session
#   # Drive the TUI manually (takes ~60s)
#   # Press Ctrl+D or type 'exit' when done
#   # GIF is auto-generated
set -euo pipefail
cd "$(dirname "$0")/.."

CAST="demo/knuckle-install.cast"
GIF="demo/knuckle-install.gif"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"

rm -f "$CAST" "$GIF"
mkdir -p demo

echo "=== Knuckle TUI Demo Recorder ==="
echo ""
echo "1. Building + booting VM..."

just build >/dev/null 2>&1
just _kill-vm 2>/dev/null || true
just _ensure-base 2>/dev/null || true
mkdir -p .vm
test -f .vm/e2e_key || ssh-keygen -t ed25519 -f .vm/e2e_key -N '' -C knuckle-demo -q
E2E_PUB=$(cat .vm/e2e_key.pub)
rm -f .vm/boot.qcow2 .vm/target.qcow2
qemu-img create -f qcow2 -b "$(pwd)/.vm/flatcar_base_amd64.img" -F qcow2 .vm/boot.qcow2 >/dev/null
qemu-img create -f qcow2 .vm/target.qcow2 20G >/dev/null
printf '{"ignition":{"version":"3.4.0"},"passwd":{"users":[{"name":"core","sshAuthorizedKeys":["%s"]}]},"systemd":{"units":[{"name":"sshd.service","enabled":true}]}}\n' "$E2E_PUB" > .vm/demo-ign.json

/home/linuxbrew/.linuxbrew/bin/qemu-system-x86_64 -m 4096 -smp 2 -enable-kvm \
  -drive if=virtio,file=.vm/boot.qcow2,format=qcow2 \
  -drive if=virtio,file=.vm/target.qcow2,format=qcow2 \
  -fw_cfg name=opt/org.flatcar-linux/config,file=.vm/demo-ign.json \
  -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
  -display none -daemonize -pidfile .vm/qemu.pid -serial file:.vm/demo-serial.log

echo "   Waiting for VM SSH..."
for _ in $(seq 1 30); do
  ssh $SSH_OPTS -o ConnectTimeout=3 -i .vm/e2e_key -p 2222 core@127.0.0.1 true 2>/dev/null && break
  sleep 3
done

scp $SSH_OPTS -i .vm/e2e_key -P 2222 bin/knuckle core@127.0.0.1:/tmp/knuckle >/dev/null
echo "   ✓ VM ready, knuckle deployed"
echo ""
echo "2. Opening Ghostty with asciinema recording..."
echo "   Drive the TUI: browse channels → select stable → DHCP → pick sysexts → install"
echo "   When done: exit the SSH session (Ctrl+D or 'exit')"
echo ""

# Launch Ghostty with the recording session
ghostty --gtk-single-instance=false -e bash -c "
  echo '━━━ RECORDING ━━━ Drive the TUI, exit when done ━━━'
  echo ''
  asciinema rec --cols 120 --rows 35 --overwrite '$CAST' -c 'ssh $SSH_OPTS -t -i .vm/e2e_key -p 2222 core@127.0.0.1 TERM=xterm-256color /tmp/knuckle --dry-run'
  echo ''
  echo '━━━ Recording saved! Close this window. ━━━'
  sleep 5
" &

echo "   Ghostty launched. Waiting for you to complete the recording..."
echo "   (press Ctrl+C here to abort)"

# Wait for the cast file to appear and be non-empty
while true; do
  if [ -s "$CAST" ]; then
    # Wait a moment for file to be fully written
    sleep 2
    break
  fi
  sleep 2
done

echo ""
echo "3. Converting to GIF..."
kill "$(cat .vm/qemu.pid)" 2>/dev/null || true
rm -f .vm/qemu.pid

agg --theme dracula --font-size 14 --speed 1.5 "$CAST" "$GIF"
echo "   ✓ GIF: $GIF ($(du -h "$GIF" | cut -f1))"
echo ""
echo "=== Done! Copy to website: ==="
echo "   cp demo/knuckle-install.gif ~/src/website/public/knuckle-install-demo.gif"
