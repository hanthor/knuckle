#!/usr/bin/env bash
# Record knuckle TUI demo using tmux for PTY + asciinema for capture.
# Produces demo/knuckle-install.cast → converts to GIF via agg.
set -euo pipefail
cd "$(dirname "$0")/.."

SESSION="knuckle-demo"
CAST="demo/knuckle-install.cast"
GIF="demo/knuckle-install.gif"

rm -f "$CAST" "$GIF"
mkdir -p demo

# Kill any existing session
tmux kill-session -t "$SESSION" 2>/dev/null || true

# Start tmux session with knuckle in dry-run
tmux new-session -d -s "$SESSION" -x 120 -y 35 \
  "TERM=xterm-256color asciinema rec --cols 120 --rows 35 --overwrite '$CAST' -c 'bin/knuckle --dry-run'"

sleep 4  # Wait for TUI to render

# Welcome — browse channels
tmux send-keys -t "$SESSION" j; sleep 0.7
tmux send-keys -t "$SESSION" j; sleep 0.7
tmux send-keys -t "$SESSION" j; sleep 0.7
tmux send-keys -t "$SESSION" k; sleep 0.7
tmux send-keys -t "$SESSION" k; sleep 0.7
tmux send-keys -t "$SESSION" k; sleep 1.2

# Select stable
tmux send-keys -t "$SESSION" Enter; sleep 2.5

# Network form — submit
tmux send-keys -t "$SESSION" Enter; sleep 2.5

# Storage — select
tmux send-keys -t "$SESSION" Enter; sleep 2.5

# User form — submit with defaults
tmux send-keys -t "$SESSION" Enter; sleep 3.5

# Sysext — navigate and select
sleep 1
tmux send-keys -t "$SESSION" j; sleep 0.4
tmux send-keys -t "$SESSION" Space; sleep 0.6
tmux send-keys -t "$SESSION" j; sleep 0.4
tmux send-keys -t "$SESSION" j; sleep 0.4
tmux send-keys -t "$SESSION" Space; sleep 0.6
tmux send-keys -t "$SESSION" j; sleep 0.4
tmux send-keys -t "$SESSION" j; sleep 0.4
tmux send-keys -t "$SESSION" j; sleep 0.4
tmux send-keys -t "$SESSION" Space; sleep 1
tmux send-keys -t "$SESSION" Enter; sleep 2.5

# Update strategy
tmux send-keys -t "$SESSION" Enter; sleep 2.5

# Review — pause to show
sleep 3

# Quit
tmux send-keys -t "$SESSION" q; sleep 0.5
tmux send-keys -t "$SESSION" q; sleep 2

# Wait for session to end
sleep 2
tmux kill-session -t "$SESSION" 2>/dev/null || true

# Check cast file
if [ ! -s "$CAST" ]; then
  echo "ERROR: Recording failed — $CAST is empty"
  exit 1
fi
echo "✓ Recording: $CAST ($(du -h "$CAST" | cut -f1))"

# Convert to GIF
agg --theme dracula --font-size 16 --speed 1.5 "$CAST" "$GIF"
echo "✓ GIF: $GIF ($(du -h "$GIF" | cut -f1))"
