#!/usr/bin/env bash
# Build a UEFI-bootable ISO for CI (uses grub-mkstandalone).
# Called by .github/workflows/release.yml
# For local dev without GRUB tools, use scripts/build-iso.sh instead.
set -euo pipefail

CHANNEL="${1:-stable}"
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="$ROOT_DIR/.iso-build"
OUTPUT_DIR="$ROOT_DIR/output"
BINARY="$ROOT_DIR/knuckle"

if [[ "$CHANNEL" == "lts" ]]; then
    BASE_URL="https://lts.release.flatcar-linux.net/amd64-usr/current"
else
    BASE_URL="https://${CHANNEL}.release.flatcar-linux.net/amd64-usr/current"
fi

echo "=== Building knuckle installer ISO (CI, channel: $CHANNEL) ==="

# 1. Download PXE artifacts
mkdir -p "$BUILD_DIR"
echo "[1/6] Downloading Flatcar PXE artifacts..."
curl -sL -o "$BUILD_DIR/vmlinuz" "$BASE_URL/flatcar_production_pxe.vmlinuz"
curl -sL -o "$BUILD_DIR/initrd.cpio.gz" "$BASE_URL/flatcar_production_pxe_image.cpio.gz"

# 2. Create overlay
echo "[2/6] Creating knuckle overlay..."
OVERLAY="$BUILD_DIR/overlay"
rm -rf "$OVERLAY"
mkdir -p "$OVERLAY/opt" "$OVERLAY/etc/systemd/system/multi-user.target.wants" "$OVERLAY/etc/systemd/system"
cp "$BINARY" "$OVERLAY/opt/knuckle"
chmod 755 "$OVERLAY/opt/knuckle"

printf '[Unit]\nDescription=Knuckle Flatcar Installer\nAfter=multi-user.target network-online.target\nWants=network-online.target\nConditionPathExists=/opt/knuckle\n\n[Service]\nType=idle\nExecStart=/opt/knuckle --log-file /tmp/knuckle.log\nStandardInput=tty\nStandardOutput=tty\nTTYPath=/dev/tty1\nTTYReset=yes\nTTYVHangup=yes\nRestart=on-failure\nRestartSec=3\n\n[Install]\nWantedBy=multi-user.target\n' \
    > "$OVERLAY/etc/systemd/system/knuckle-installer.service"

ln -sf /etc/systemd/system/knuckle-installer.service \
    "$OVERLAY/etc/systemd/system/multi-user.target.wants/knuckle-installer.service"

(cd "$OVERLAY" && find . | cpio -o -H newc 2>/dev/null | gzip > "$BUILD_DIR/overlay.cpio.gz")

# 3. Combine initramfs
echo "[3/6] Combining initramfs..."
cat "$BUILD_DIR/initrd.cpio.gz" "$BUILD_DIR/overlay.cpio.gz" > "$BUILD_DIR/combined-initrd.img"

# 4. Build GRUB EFI
echo "[4/6] Building GRUB EFI..."
printf 'set timeout=3\nset default=0\n\nmenuentry "Knuckle - Install Flatcar Container Linux" {\n    linux /vmlinuz flatcar.autologin=tty1 console=tty0 console=ttyS0\n    initrd /initrd.img\n}\n\nmenuentry "Knuckle - Install (serial console)" {\n    linux /vmlinuz flatcar.autologin=ttyS0 console=ttyS0\n    initrd /initrd.img\n}\n' \
    > "$BUILD_DIR/grub.cfg"

grub-mkstandalone \
    --format=x86_64-efi \
    --output="$BUILD_DIR/BOOTX64.EFI" \
    --locales="" \
    --fonts="" \
    "boot/grub/grub.cfg=$BUILD_DIR/grub.cfg"

# 5. Build ESP
echo "[5/6] Building ESP..."
INITRD_SIZE=$(stat -c%s "$BUILD_DIR/combined-initrd.img")
KERNEL_SIZE=$(stat -c%s "$BUILD_DIR/vmlinuz")
GRUB_SIZE=$(stat -c%s "$BUILD_DIR/BOOTX64.EFI")
ESP_SIZE_MB=$(( (INITRD_SIZE + KERNEL_SIZE + GRUB_SIZE + 10*1024*1024) / 1024 / 1024 ))

EFI_IMG="$BUILD_DIR/efi.img"
dd if=/dev/zero of="$EFI_IMG" bs=1M count=$ESP_SIZE_MB 2>/dev/null
mformat -i "$EFI_IMG" -F ::
mmd -i "$EFI_IMG" ::/EFI ::/EFI/BOOT
mcopy -i "$EFI_IMG" "$BUILD_DIR/BOOTX64.EFI" ::/EFI/BOOT/BOOTX64.EFI
mcopy -i "$EFI_IMG" "$BUILD_DIR/vmlinuz" ::/vmlinuz
mcopy -i "$EFI_IMG" "$BUILD_DIR/combined-initrd.img" ::/initrd.img

# 6. Assemble ISO
echo "[6/6] Assembling ISO..."
ISO_DIR="$BUILD_DIR/iso-root"
rm -rf "$ISO_DIR"
mkdir -p "$ISO_DIR"
cp "$BUILD_DIR/vmlinuz" "$ISO_DIR/"
cp "$BUILD_DIR/combined-initrd.img" "$ISO_DIR/initrd.img"
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
    "$ISO_DIR"

echo ""
echo "✅ ISO built: $ISO_OUT ($(du -h "$ISO_OUT" | cut -f1))"
