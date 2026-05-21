#!/usr/bin/env bash
# Record knuckle TUI demo using tmux for PTY + asciinema for capture.
# Produces demo/knuckle-install.cast → converts to GIF via agg.
#
# WHY Down/Up instead of j/k:
#   main.go makes sync HTTP fetches (FetchSysexts, FetchChannels) before
#   tui.Run(). During that window the PTY has ECHO enabled and MakeRaw has
#   not been called. Sending printable 'j' echoes it as a visible character.
#   Escape sequences (Down = \033[B) are non-printable — no visible echo.
#   Also: when huh form fields are focused, 'j' types into text inputs.
set -euo pipefail
cd "$(dirname "$0")/.."

SESSION="knuckle-demo"
CAST="demo/knuckle-install.cast"
GIF="demo/knuckle-install.gif"

rm -f "$CAST" "$GIF"
mkdir -p demo

tmux kill-session -t "$SESSION" 2>/dev/null || true

# Start tmux session with knuckle
tmux new-session -d -s "$SESSION" -x 120 -y 35 \
  "TERM=xterm-256color asciinema rec --cols 120 --rows 35 --overwrite '$CAST' -c 'bin/knuckle --dry-run'"

# Wait for HTTP fetches + TUI init (8-12s depending on network)
sleep 14

# Welcome — browse channels with arrow keys (non-printable, no PTY echo)
tmux send-keys -t "$SESSION" Down;  sleep 0.8
tmux send-keys -t "$SESSION" Down;  sleep 0.8
tmux send-keys -t "$SESSION" Down;  sleep 0.8
tmux send-keys -t "$SESSION" Up;    sleep 0.8
tmux send-keys -t "$SESSION" Up;    sleep 0.8
tmux send-keys -t "$SESSION" Up;    sleep 1.5

# Select stable
tmux send-keys -t "$SESSION" Enter; sleep 3

# Network form — submit (DHCP default)
tmux send-keys -t "$SESSION" Enter; sleep 3

# Storage — select first disk
tmux send-keys -t "$SESSION" Enter; sleep 3

# User form — submit with defaults (local SSH keys auto-detected)
tmux send-keys -t "$SESSION" Enter; sleep 4

# Sysext — navigate and select with arrow keys
sleep 1.5
tmux send-keys -t "$SESSION" Down;  sleep 0.5
tmux send-keys -t "$SESSION" Space; sleep 0.7
tmux send-keys -t "$SESSION" Down;  sleep 0.5
tmux send-keys -t "$SESSION" Down;  sleep 0.5
tmux send-keys -t "$SESSION" Space; sleep 0.7
tmux send-keys -t "$SESSION" Down;  sleep 0.5
tmux send-keys -t "$SESSION" Down;  sleep 0.5
tmux send-keys -t "$SESSION" Down;  sleep 0.5
tmux send-keys -t "$SESSION" Space; sleep 1.2
tmux send-keys -t "$SESSION" Enter; sleep 3

# Update strategy — accept default
tmux send-keys -t "$SESSION" Enter; sleep 3

# Review — pause to show the summary
sleep 4

# Quit
tmux send-keys -t "$SESSION" q; sleep 0.6
tmux send-keys -t "$SESSION" q; sleep 2

# Wait for exit
sleep 2
tmux kill-session -t "$SESSION" 2>/dev/null || true

if [ ! -s "$CAST" ]; then
  echo "ERROR: Recording failed — $CAST is empty"
  exit 1
fi
echo "✓ Recording: $CAST ($(du -h "$CAST" | cut -f1))"

agg --theme dracula --font-size 16 --speed 1.5 "$CAST" "$GIF"
echo "✓ GIF: $GIF ($(du -h "$GIF" | cut -f1))"
