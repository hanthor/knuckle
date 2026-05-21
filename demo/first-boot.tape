Output demo/first-boot.gif

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
Set TypingSpeed 30ms

Type "$ ssh core@flatcar.local"
Enter
Sleep 0.6s
Type "core@flatcar ~ $ nvidia-smi --query-gpu=name --format=csv,noheader"
Enter
Sleep 0.4s
Type "NVIDIA GeForce RTX 4090"
Enter
Sleep 0.8s
Type "core@flatcar ~ $ docker run --rm --gpus all nvidia/cuda:12.6.3-base nvidia-smi"
Enter
Sleep 0.8s
Type "NVIDIA-SMI 570.86.15   Driver: 570.86.15   CUDA: 12.8"
Enter
Type "RTX 4090 | 46C  P8  20W/450W | 0MiB/24564MiB | 0%"
Enter
Sleep 1s
Type "core@flatcar ~ $ systemd-sysext status"
Enter
Sleep 0.3s
Type "HIERARCHY  EXTENSIONS       SINCE"
Enter
Type "/opt        docker          1min ago"
Enter
Type "/opt        nvidia-runtime  1min ago"
Enter
Type "/opt        wasmtime        1min ago"
Enter
Sleep 1s
Type "core@flatcar ~ $ docker version -f '{{.Server.Version}}'"
Enter
Type "28.0.4"
Enter
Sleep 1.5s
