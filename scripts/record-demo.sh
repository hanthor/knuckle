#!/usr/bin/env bash
# Record a complete knuckle headless install inside a real Flatcar VM.
# Uses the proven vm-e2e infrastructure — always produces clean output.
set -euo pipefail
cd "$(dirname "$0")/.."

SESSION="knuckle-demo"
CAST="demo/knuckle-install.cast"
GIF="demo/knuckle-install.gif"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"

rm -f "$CAST" "$GIF"
mkdir -p demo
tmux kill-session -t "$SESSION" 2>/dev/null || true

# Build + boot VM
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

# Headless config: realistic GPU server setup
printf '{"channel":"stable","hostname":"gpu-server","timezone":"UTC","network":{"mode":"dhcp"},"users":[{"username":"core","ssh_keys":["%s"],"github_user":"castrojo"}],"disk":"/dev/vdb","sysexts":["docker","wasmtime"],"nvidia_driver_version":"570-open","update_strategy":"reboot","reboot":false}\n' "$E2E_PUB" > .vm/demo-config.json

/home/linuxbrew/.linuxbrew/bin/qemu-system-x86_64 -m 4096 -smp 2 -enable-kvm \
  -drive if=virtio,file=.vm/boot.qcow2,format=qcow2 \
  -drive if=virtio,file=.vm/target.qcow2,format=qcow2 \
  -fw_cfg name=opt/org.flatcar-linux/config,file=.vm/demo-ign.json \
  -net nic,model=virtio -net user,hostfwd=tcp::2222-:22 \
  -display none -daemonize -pidfile .vm/qemu.pid -serial file:.vm/demo-serial.log

# Wait for SSH
for i in $(seq 1 40); do
  ssh $SSH_OPTS -o ConnectTimeout=3 -i .vm/e2e_key -p 2222 core@127.0.0.1 true 2>/dev/null && break
  sleep 3
done

# Deploy
scp $SSH_OPTS -i .vm/e2e_key -P 2222 bin/knuckle core@127.0.0.1:/tmp/knuckle >/dev/null
scp $SSH_OPTS -i .vm/e2e_key -P 2222 .vm/demo-config.json core@127.0.0.1:/tmp/config.json >/dev/null

# Record the headless install via tmux + asciinema
SSH_CMD="ssh $SSH_OPTS -i .vm/e2e_key -p 2222 core@127.0.0.1 -t 'TERM=xterm-256color; echo \"$ cat config.json | jq .\"; cat /tmp/config.json | python3 -m json.tool 2>/dev/null || cat /tmp/config.json; echo; echo \"$ sudo knuckle --headless --config config.json\"; sudo /tmp/knuckle --headless --config /tmp/config.json --log-file /tmp/knuckle.log; sleep 3'"

tmux new-session -d -s "$SESSION" -x 100 -y 30 \
  "asciinema rec --cols 100 --rows 30 --overwrite '$CAST' -c \"$SSH_CMD\""

# Wait for it to complete (headless install takes ~40s in VM)
sleep 55

tmux kill-session -t "$SESSION" 2>/dev/null || true
kill "$(cat .vm/qemu.pid)" 2>/dev/null || true
rm -f .vm/qemu.pid

if [ ! -s "$CAST" ]; then
  echo "ERROR: Recording failed"
  exit 1
fi
echo "✓ Recording: $CAST ($(du -h "$CAST" | cut -f1))"

agg --theme dracula --font-size 16 --speed 2 "$CAST" "$GIF"
echo "✓ GIF: $GIF ($(du -h "$GIF" | cut -f1))"
