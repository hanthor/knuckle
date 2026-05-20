#!/usr/bin/env bash
# Build a UEFI-bootable ISO containing Flatcar Linux + knuckle installer.
#
# The ISO boots into a live Flatcar environment with knuckle auto-launching
# on the console. No network required during boot.
#
# Requirements: xorriso, mformat, mcopy (mtools), cpio, gzip
# Usage: ./scripts/build-iso.sh [--channel stable|beta|alpha|lts]
set -euo pipefail

CHANNEL="${1:-stable}"
CHANNEL="${CHANNEL#--channel=}"
CHANNEL="${CHANNEL#--channel }"
[[ "$CHANNEL" == --channel ]] && CHANNEL="${2:-stable}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BUILD_DIR="$ROOT_DIR/.iso-build"
OUTPUT_DIR="$ROOT_DIR/output"
BINARY="$ROOT_DIR/bin/knuckle"

# Flatcar release URL
if [[ "$CHANNEL" == "lts" ]]; then
    BASE_URL="https://lts.release.flatcar-linux.net/amd64-usr/current"
else
    BASE_URL="https://${CHANNEL}.release.flatcar-linux.net/amd64-usr/current"
fi

echo "=== Building knuckle installer ISO (channel: $CHANNEL) ==="

# Check dependencies
for cmd in xorriso mformat mcopy cpio gzip; do
    if ! command -v "$cmd" &>/dev/null; then
        echo "❌ Missing: $cmd"
        echo "Install: brew install xorriso mtools cpio"
        exit 1
    fi
done

# Find grub-mkstandalone (various names across distros)
GRUB_MKSTANDALONE=""
for name in grub-mkstandalone grub2-mkstandalone x86_64-elf-grub-mkstandalone; do
    if command -v "$name" &>/dev/null; then
        GRUB_MKSTANDALONE="$name"
        break
    fi
done
if [[ -z "$GRUB_MKSTANDALONE" ]]; then
    echo "❌ Missing: grub-mkstandalone"
    echo "Install: brew install x86_64-elf-grub (or grub2-tools-efi on Fedora)"
    exit 1
fi

# 1. Build knuckle binary
echo "[1/6] Building knuckle..."
(cd "$ROOT_DIR" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/knuckle ./cmd/knuckle)

# 2. Download Flatcar PXE artifacts
mkdir -p "$BUILD_DIR"
echo "[2/6] Downloading Flatcar PXE artifacts ($CHANNEL)..."

KERNEL="$BUILD_DIR/vmlinuz"
INITRD="$BUILD_DIR/initrd.cpio.gz"

if [[ ! -f "$KERNEL" ]]; then
    curl -L -o "$KERNEL" "$BASE_URL/flatcar_production_pxe.vmlinuz"
fi
if [[ ! -f "$INITRD" ]]; then
    curl -L -o "$INITRD" "$BASE_URL/flatcar_production_pxe_image.cpio.gz"
fi

echo "  kernel: $(du -h "$KERNEL" | cut -f1)"
echo "  initrd: $(du -h "$INITRD" | cut -f1)"

# 3. Create supplemental cpio with knuckle + Ignition + systemd unit
echo "[3/6] Creating knuckle overlay..."

OVERLAY_DIR="$BUILD_DIR/overlay"
rm -rf "$OVERLAY_DIR"
mkdir -p "$OVERLAY_DIR/opt"
mkdir -p "$OVERLAY_DIR/etc/systemd/system/multi-user.target.wants"
mkdir -p "$OVERLAY_DIR/etc/systemd/system"

# Binary
cp "$BINARY" "$OVERLAY_DIR/opt/knuckle"
chmod 755 "$OVERLAY_DIR/opt/knuckle"

# Systemd unit — launches knuckle on tty1
printf '[Unit]\nDescription=Knuckle Flatcar Installer\nAfter=multi-user.target network-online.target\nWants=network-online.target\nConditionPathExists=/opt/knuckle\n\n[Service]\nType=idle\nExecStart=/opt/knuckle --log-file /tmp/knuckle.log\nStandardInput=tty\nStandardOutput=tty\nTTYPath=/dev/tty1\nTTYReset=yes\nTTYVHangup=yes\nRestart=on-failure\nRestartSec=3\n\n[Install]\nWantedBy=multi-user.target\n' \
    > "$OVERLAY_DIR/etc/systemd/system/knuckle-installer.service"

# Enable the unit
ln -sf /etc/systemd/system/knuckle-installer.service \
    "$OVERLAY_DIR/etc/systemd/system/multi-user.target.wants/knuckle-installer.service"

# Add SSH key for the live ISO environment (so we can test via SSH)
if [[ -f "$HOME/.ssh/id_ed25519.pub" ]]; then
    mkdir -p "$OVERLAY_DIR/home/core/.ssh"
    cp "$HOME/.ssh/id_ed25519.pub" "$OVERLAY_DIR/home/core/.ssh/authorized_keys"
    chmod 700 "$OVERLAY_DIR/home/core/.ssh"
    chmod 600 "$OVERLAY_DIR/home/core/.ssh/authorized_keys"
fi

# Build supplemental cpio archive (newc format, appended to initrd)
OVERLAY_CPIO="$BUILD_DIR/knuckle-overlay.cpio.gz"
(cd "$OVERLAY_DIR" && find . | cpio -o -H newc 2>/dev/null | gzip > "$OVERLAY_CPIO")
echo "  overlay: $(du -h "$OVERLAY_CPIO" | cut -f1)"

# 4. Concatenate initrd + overlay (Linux supports multiple cpio in initramfs)
echo "[4/6] Combining initramfs..."
COMBINED_INITRD="$BUILD_DIR/combined-initrd.img"
cat "$INITRD" "$OVERLAY_CPIO" > "$COMBINED_INITRD"
echo "  combined: $(du -h "$COMBINED_INITRD" | cut -f1)"

# 5. Build EFI system partition with GRUB
echo "[5/6] Building EFI boot image..."

# GRUB config
GRUB_CFG="$BUILD_DIR/grub.cfg"
printf 'insmod fat\ninsmod part_gpt\ninsmod search\nsearch --no-floppy --file /vmlinuz --set=root\nset timeout=3\nset default=0\n\nmenuentry "Knuckle - Install Flatcar Container Linux" {\n    linux /vmlinuz flatcar.autologin=tty1 console=tty0 console=ttyS0\n    initrd /initrd.img\n}\n\nmenuentry "Knuckle - Install (serial console)" {\n    linux /vmlinuz flatcar.autologin=ttyS0 console=ttyS0\n    initrd /initrd.img\n}\n' > "$GRUB_CFG"

# Build standalone GRUB EFI binary with embedded config
GRUB_DIR="/home/linuxbrew/.linuxbrew/lib/x86_64-elf/grub/x86_64-efi"
if [[ ! -d "$GRUB_DIR" ]]; then
    GRUB_DIR=$(dirname $(find /home/linuxbrew -name "fat.mod" -path "*x86_64-efi*" 2>/dev/null | head -1))
fi
$GRUB_MKSTANDALONE \
    --format=x86_64-efi \
    --output="$BUILD_DIR/BOOTX64.EFI" \
    --locales="" \
    --fonts="" \
    --modules="fat part_gpt part_msdos normal linux all_video" \
    "boot/grub/grub.cfg=$GRUB_CFG"

# Size ESP to fit kernel + initrd + GRUB
INITRD_SIZE=$(stat -c%s "$COMBINED_INITRD")
KERNEL_SIZE=$(stat -c%s "$KERNEL")
GRUB_SIZE=$(stat -c%s "$BUILD_DIR/BOOTX64.EFI")
ESP_SIZE_MB=$(( (INITRD_SIZE + KERNEL_SIZE + GRUB_SIZE + 10*1024*1024) / 1024 / 1024 ))

EFI_IMG="$BUILD_DIR/efi.img"
dd if=/dev/zero of="$EFI_IMG" bs=1M count=$ESP_SIZE_MB 2>/dev/null
mformat -i "$EFI_IMG" -F ::
mmd -i "$EFI_IMG" ::/EFI ::/EFI/BOOT
mcopy -i "$EFI_IMG" "$BUILD_DIR/BOOTX64.EFI" ::/EFI/BOOT/BOOTX64.EFI
mcopy -i "$EFI_IMG" "$KERNEL" ::/vmlinuz
mcopy -i "$EFI_IMG" "$COMBINED_INITRD" ::/initrd.img

echo "  ESP image: $(du -h "$EFI_IMG" | cut -f1)"

# 6. Assemble ISO with xorriso
echo "[6/6] Assembling ISO..."

ISO_DIR="$BUILD_DIR/iso-root"
rm -rf "$ISO_DIR"
mkdir -p "$ISO_DIR"

# Put kernel+initrd on ISO (for reference/extraction)
cp "$KERNEL" "$ISO_DIR/vmlinuz"
cp "$COMBINED_INITRD" "$ISO_DIR/initrd.img"
# EFI image INSIDE the ISO filesystem (required for CDROM UEFI boot)
cp "$EFI_IMG" "$ISO_DIR/efi.img"

mkdir -p "$OUTPUT_DIR"
ISO_OUT="$OUTPUT_DIR/knuckle-installer-${CHANNEL}.iso"

xorriso -as mkisofs \
    -o "$ISO_OUT" \
    -R -J -joliet-long \
    -V "KNUCKLE_INSTALL" \
    -eltorito-alt-boot \
    -e efi.img \
    -no-emul-boot \
    -isohybrid-gpt-basdat \
    "$ISO_DIR" 2>/dev/null

echo ""
echo "✅ ISO built: $ISO_OUT ($(du -h "$ISO_OUT" | cut -f1))"
echo ""
echo "Boot with QEMU:"
echo "  qemu-system-x86_64 -m 4096 -enable-kvm -cdrom $ISO_OUT \\"
echo "    -drive if=virtio,file=target.qcow2,format=qcow2 -nographic"
echo ""
echo "Write to USB:"
echo "  sudo dd if=$ISO_OUT of=/dev/sdX bs=4M status=progress"
