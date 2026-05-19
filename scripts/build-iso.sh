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
        echo "Install: brew install xorriso mtools (or equivalent)"
        exit 1
    fi
done

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
cat > "$OVERLAY_DIR/etc/systemd/system/knuckle-installer.service" <<'EOF'
[Unit]
Description=Knuckle Flatcar Installer
After=multi-user.target network-online.target
Wants=network-online.target
ConditionPathExists=/opt/knuckle

[Service]
Type=idle
ExecStart=/opt/knuckle --log-file /tmp/knuckle.log
StandardInput=tty
StandardOutput=tty
TTYPath=/dev/tty1
TTYReset=yes
TTYVHangup=yes
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

# Enable the unit
ln -sf /etc/systemd/system/knuckle-installer.service \
    "$OVERLAY_DIR/etc/systemd/system/multi-user.target.wants/knuckle-installer.service"

# Build supplemental cpio archive (newc format, appended to initrd)
OVERLAY_CPIO="$BUILD_DIR/knuckle-overlay.cpio.gz"
(cd "$OVERLAY_DIR" && find . | cpio -o -H newc 2>/dev/null | gzip > "$OVERLAY_CPIO")
echo "  overlay: $(du -h "$OVERLAY_CPIO" | cut -f1)"

# 4. Concatenate initrd + overlay (Linux supports multiple cpio in initramfs)
echo "[4/6] Combining initramfs..."
COMBINED_INITRD="$BUILD_DIR/combined-initrd.img"
cat "$INITRD" "$OVERLAY_CPIO" > "$COMBINED_INITRD"
echo "  combined: $(du -h "$COMBINED_INITRD" | cut -f1)"

# 5. Build EFI system partition (FAT32 image with kernel+initrd+startup.nsh)
echo "[5/6] Building EFI boot image..."

# ESP must contain kernel, initrd, and a boot script.
# We use the EFI stub kernel directly — no GRUB needed.
# UEFI firmware boots BOOTX64.EFI; we use a shim startup.nsh for the shell fallback.
INITRD_SIZE=$(stat -c%s "$COMBINED_INITRD")
KERNEL_SIZE=$(stat -c%s "$KERNEL")
ESP_SIZE_MB=$(( (INITRD_SIZE + KERNEL_SIZE + 10*1024*1024) / 1024 / 1024 ))

EFI_IMG="$BUILD_DIR/efi.img"
dd if=/dev/zero of="$EFI_IMG" bs=1M count=$ESP_SIZE_MB 2>/dev/null
mformat -i "$EFI_IMG" -F ::

# Put initrd at root of ESP, kernel as BOOTX64.EFI
mcopy -i "$EFI_IMG" "$COMBINED_INITRD" ::/initrd.img

# EFI stub kernel as the default boot binary
mmd -i "$EFI_IMG" ::/EFI
mmd -i "$EFI_IMG" ::/EFI/BOOT
mcopy -i "$EFI_IMG" "$KERNEL" ::/EFI/BOOT/BOOTX64.EFI

# startup.nsh — EFI shell fallback (auto-runs if direct boot fails)
STARTUP="$BUILD_DIR/startup.nsh"
cat > "$STARTUP" <<'NSHEOF'
@echo -off
echo "Knuckle - Flatcar Container Linux Installer"
echo "Booting..."
\EFI\BOOT\BOOTX64.EFI flatcar.autologin=tty1 console=tty0 console=ttyS0 initrd=\initrd.img
NSHEOF
mcopy -i "$EFI_IMG" "$STARTUP" ::/startup.nsh

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
