# Complete knuckle installation demo — website hero GIF
# Shows: channel browsing → stable → DHCP → sysext selection → install progress

Output demo/install-complete.gif

Set FontSize 15
Set Width 1280
Set Height 720
Set Theme "Catppuccin Mocha"
Set Padding 24
Set Framerate 24
Set TypingSpeed 50ms
Set WindowBar Colorful
Set WindowBarSize 40
Set BorderRadius 12

# Launch knuckle
Type "knuckle --dry-run"
Enter
Sleep 2.5s

# Welcome screen — browse channels to show them off
Down
Sleep 0.8s
Down
Sleep 0.8s
Down
Sleep 0.8s
# Back to stable
Up
Sleep 0.8s
Up
Sleep 0.8s
Up
Sleep 0.6s
# Select stable
Enter
Sleep 2s

# Network — DHCP (default), just accept
Enter
Sleep 2s

# Storage — select first disk
Enter
Sleep 2s

# User — fill in details
# hostname is pre-filled "flatcar", username "core"
# Tab to GitHub username field
Tab
Sleep 0.3s
Tab
Sleep 0.3s
Tab
Sleep 0.3s
Type "castrojo"
Sleep 0.8s
Enter
Sleep 3.5s

# Sysext screen — browse and select a few
Sleep 1s
Down
Sleep 0.4s
Down
Sleep 0.4s
Space
Sleep 0.6s
Down
Sleep 0.4s
Down
Sleep 0.4s
Down
Sleep 0.4s
Space
Sleep 0.6s
Down
Sleep 0.4s
Down
Sleep 0.4s
Space
Sleep 0.6s
Sleep 1s
Enter
Sleep 2s

# Update strategy — accept default (reboot)
Enter
Sleep 2s

# Review screen — pause to show the summary
Sleep 3s

# Show Butane preview
Ctrl+B
Sleep 3s
Ctrl+B
Sleep 1s

# Confirm install
Enter
Sleep 1s

# Watch progress
Sleep 8s
