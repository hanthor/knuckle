# knuckle installer demo
# Records an animated GIF showing the TUI wizard flow

Output demo/knuckle-demo.gif
Set FontSize 14
Set Width 1200
Set Height 800
Set Theme "Dracula"
Set Padding 20
Set Framerate 30

# Start knuckle in dry-run mode
Type "bin/knuckle --dry-run"
Enter
Sleep 2s

# Welcome screen — select stable channel (already default)
Sleep 1.5s
# Press Enter to accept stable channel
Enter
Sleep 1.5s

# Network screen — DHCP is default
Sleep 1s
# Tab through the form, accept DHCP defaults
Enter
Sleep 1.5s

# Storage screen — select first disk
Sleep 1s
Enter
Sleep 1.5s

# User screen — fill in hostname and username
Sleep 0.5s
# hostname field should be pre-filled with "flatcar"
# Tab to next field (username, pre-filled "core")
Tab
Sleep 0.3s
# Tab to password (skip)
Tab
Sleep 0.3s
# Tab to GitHub username
Tab
Sleep 0.3s
# Type a GitHub username
Type "castrojo"
Sleep 0.5s
# Submit the form
Enter
Sleep 3s

# Sysext screen — browse extensions
Sleep 1s
# Move down through sysexts
Down
Sleep 0.5s
Down
Sleep 0.5s
Down
Sleep 0.5s
# Select one with space
Space
Sleep 0.5s
# Move down more
Down
Sleep 0.5s
Down
Sleep 0.5s
# Enter to continue
Enter
Sleep 1.5s

# Update strategy screen
Sleep 1s
Enter
Sleep 1.5s

# Review screen
Sleep 2s

# Toggle Butane preview with Ctrl+B
Ctrl+B
Sleep 2s
Ctrl+B
Sleep 1s

# Go back with shift+tab to show navigation
Shift+Tab
Sleep 1s
Shift+Tab
Sleep 1s

# Come back forward
Enter
Sleep 1s
Enter
Sleep 1.5s

# On review, quit instead of installing (this is a demo)
Type "q"
Sleep 0.5s
Type "q"
Sleep 1s
