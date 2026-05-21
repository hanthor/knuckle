#!/usr/bin/env bash
# Record the knuckle TUI demo.
# Usage: TERM=xterm-256color asciinema rec -c "./scripts/drive-demo.sh" demo/out.cast
set -u

FIFO=$(mktemp -u)
mkfifo "$FIFO"

# Launch knuckle reading from the FIFO
bin/knuckle --dry-run < "$FIFO" &
KNUCKLE_PID=$!

# Open the fifo for writing (keeps it open so knuckle doesn't get EOF)
exec 3>"$FIFO"

send() { printf "$1" >&3; }
pause() { sleep "$1"; }

# Wait for TUI to render
pause 3

# Welcome — browse channels
send "j"; pause 0.7
send "j"; pause 0.7
send "j"; pause 0.7
send "k"; pause 0.7
send "k"; pause 0.7
send "k"; pause 1.2

# Select stable
send "\r"; pause 2.5

# Network form — Enter
send "\r"; pause 2.5

# Storage — Enter
send "\r"; pause 2.5

# User form — Enter (uses local SSH keys)
send "\r"; pause 3

# Sysext — select a few
pause 1
send "j"; pause 0.4
send " "; pause 0.6
send "j"; pause 0.4
send "j"; pause 0.4
send " "; pause 0.6
send "j"; pause 0.4
send "j"; pause 0.4
send "j"; pause 0.4
send " "; pause 1
send "\r"; pause 2.5

# Update strategy
send "\r"; pause 2.5

# Review
pause 3

# Quit
send "q"; pause 0.5
send "q"; pause 1.5

# Cleanup
exec 3>&-
rm -f "$FIFO"
wait $KNUCKLE_PID 2>/dev/null
exit 0
