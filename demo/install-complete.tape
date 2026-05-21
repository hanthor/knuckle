Output demo/install-complete.gif

Set FontSize 16
Set Width 1000
Set Height 600
Set Theme "Catppuccin Mocha"
Set Padding 20
Set Framerate 20
Set WindowBar Colorful
Set WindowBarSize 36
Set BorderRadius 10
Set PlaybackSpeed 1.5

Type "$ knuckle --headless --config gpu-server.json"
Enter
Sleep 0.8s
Type "→ Validating configuration..."
Enter
Type "  ✓ Configuration valid"
Enter
Type "→ Resolving sysexts: docker, nvidia-runtime, wasmtime"
Enter
Sleep 0.8s
Type "  ✓ 3 sysext(s) resolved"
Enter
Type "→ Fetching SSH keys from GitHub (castrojo)..."
Enter
Sleep 0.5s
Type "  ✓ 8 keys fetched"
Enter
Type "→ Starting installation..."
Enter
Type "  • Channel:  stable (Flatcar 4593.2.1)"
Enter
Type "  • Target:   /dev/disk/by-id/nvme-Samsung_970_EVO"
Enter
Type "  • NVIDIA:   570-open kernel driver + Container Toolkit"
Enter
Type "  • Running flatcar-install..."
Enter
Sleep 2s
Type "  ✓ Installation complete (47s)"
Enter
Enter
Type "✅ Headless install finished successfully."
Enter
Sleep 2s
