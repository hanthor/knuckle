Output demo/first-boot.gif

Set FontSize 15
Set Width 1280
Set Height 720
Set Theme "Catppuccin Mocha"
Set Padding 24
Set Framerate 24
Set TypingSpeed 30ms
Set WindowBar Colorful
Set WindowBarSize 40
Set BorderRadius 12

# Already SSH'd in — show a configured Flatcar system
Type@0 "core@flatcar ~ $ "
Sleep 0.5s
Type "hostname"
Enter
Sleep 0.5s
Type@0 "flatcar"
Enter
Sleep 0.8s

Type@0 "core@flatcar ~ $ "
Sleep 0.3s
Type "cat /etc/flatcar/update.conf"
Enter
Sleep 0.5s
Type@0 "REBOOT_STRATEGY=reboot"
Enter
Type@0 "GROUP=stable"
Enter
Sleep 1s

Type@0 "core@flatcar ~ $ "
Sleep 0.3s
Type "systemd-sysext status"
Enter
Sleep 0.5s
Type@0 "HIERARCHY  EXTENSIONS  SINCE"
Enter
Type@0 "/opt        docker      2 min ago"
Enter
Type@0 "/usr        wasmtime    2 min ago"
Enter
Sleep 1.2s

Type@0 "core@flatcar ~ $ "
Sleep 0.3s
Type "docker version --format '{{.Server.Version}}'"
Enter
Sleep 0.5s
Type@0 "28.0.4"
Enter
Sleep 1s

Type@0 "core@flatcar ~ $ "
Sleep 0.3s
Type "docker run --rm hello-world 2>/dev/null | head -3"
Enter
Sleep 0.8s
Type@0 ""
Enter
Type@0 "Hello from Docker!"
Enter
Type@0 "This message shows that your installation appears to be working correctly."
Enter
Sleep 2s

Type@0 "core@flatcar ~ $ "
Sleep 0.5s
Type "uptime"
Enter
Sleep 0.4s
Type@0 " 00:03:41 up 3 min,  1 user,  load average: 0.12, 0.08, 0.03"
Enter
Sleep 2s
