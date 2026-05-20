#!/usr/bin/env bash
# Build a UEFI-bootable ISO containing Flatcar Linux + knuckle installer.
#
# Uses systemd-boot (UEFI-only, BLS entries). GRUB is not used.
# The ISO boots into a live Flatcar environment with knuckle auto-launching
# on the console. No network required during boot.
#
# Requirements (Linux): xorriso mformat mcopy (mtools) cpio gzip systemd-boot-efi
#   Ubuntu:  sudo apt-get install -y xorriso mtools cpio systemd-boot-efi
#   Fedora:  sudo dnf install -y xorriso mtools cpio systemd-boot-unsigned
#
# Usage: ./scripts/build-iso.sh [--channel stable|beta|alpha|lts] [--binary /path/to/knuckle]
set -euo pipefail

# ── Argument parsing ─────────────────────────────────────────────────────────
CHANNEL="stable"
BINARY_OVERRIDE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --channel=*) CHANNEL="${1#--channel=}"; shift ;;
        --channel)   CHANNEL="$2"; shift 2 ;;
        --binary=*)  BINARY_OVERRIDE="${1#--binary=}"; shift ;;
        --binary)    BINARY_OVERRIDE="$2"; shift 2 ;;
        stable|beta|alpha|lts) CHANNEL="$1"; shift ;;
        *) echo "Unknown argument: $1" >&2; exit 1 ;;
    esac
done

# ── Paths ────────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BUILD_DIR="$ROOT_DIR/.iso-build"
OUTPUT_DIR="$ROOT_DIR/output"

if [[ -n "$BINARY_OVERRIDE" ]]; then
    BINARY="$BINARY_OVERRIDE"
elif [[ -f "$ROOT_DIR/knuckle" ]]; then
    # CI: built binary placed at repo root
    BINARY="$ROOT_DIR/knuckle"
else
    BINARY="$ROOT_DIR/bin/knuckle"
fi

if [[ "$CHANNEL" == "lts" ]]; then
    BASE_URL="https://lts.release.flatcar-linux.net/amd64-usr/current"
else
    BASE_URL="https://${CHANNEL}.release.flatcar-linux.net/amd64-usr/current"
fi

# ── Dependency check ─────────────────────────────────────────────────────────
for cmd in xorriso mformat mcopy mmd cpio gzip; do
    if ! command -v "$cmd" &>/dev/null; then
        echo "Missing: $cmd" >&2
        echo "  Ubuntu: sudo apt-get install -y xorriso mtools cpio" >&2
        exit 1
    fi
done

# Locate systemd-boot EFI binary (UEFI-only; no GRUB)
SDBOOT_EFI=""
for candidate in \
    /usr/lib/systemd/boot/efi/systemd-bootx64.efi \
    /lib/systemd/boot/efi/systemd-bootx64.efi \
    /usr/share/systemd/boot/efi/systemd-bootx64.efi; do
    if [[ -f "$candidate" ]]; then
        SDBOOT_EFI="$candidate"
        break
    fi
done
if [[ -z "$SDBOOT_EFI" ]]; then
    echo "Missing: systemd-bootx64.efi" >&2
    echo "  Ubuntu: sudo apt-get install -y systemd-boot-efi" >&2
    echo "  Fedora: sudo dnf install -y systemd-boot-unsigned" >&2
    exit 1
fi

echo "=== Building knuckle installer ISO (channel: $CHANNEL) ==="
echo "  systemd-boot: $SDBOOT_EFI"

# ── 1. Build knuckle binary (skipped when binary already present) ─────────────
if [[ ! -f "$BINARY" ]]; then
    echo "[1/5] Building knuckle..."
    (cd "$ROOT_DIR" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
        go build -ldflags="-s -w" -o bin/knuckle ./cmd/knuckle)
    BINARY="$ROOT_DIR/bin/knuckle"
else
    echo "[1/5] Using existing knuckle binary: $BINARY"
fi

# ── 2. Download Flatcar PXE artifacts ────────────────────────────────────────
mkdir -p "$BUILD_DIR"
echo "[2/5] Fetching Flatcar PXE artifacts ($CHANNEL)..."

KERNEL="$BUILD_DIR/vmlinuz"
INITRD="$BUILD_DIR/initrd.cpio.gz"

if [[ ! -f "$KERNEL" ]]; then
    curl -fsSL -o "$KERNEL" "$BASE_URL/flatcar_production_pxe.vmlinuz"
fi
if [[ ! -f "$INITRD" ]]; then
    curl -fsSL -o "$INITRD" "$BASE_URL/flatcar_production_pxe_image.cpio.gz"
fi

echo "  kernel : $(du -h "$KERNEL"  | cut -f1)"
echo "  initrd : $(du -h "$INITRD"  | cut -f1)"

# ── 3. Build supplemental cpio overlay ───────────────────────────────────────
echo "[3/5] Building knuckle overlay..."

OVERLAY_DIR="$BUILD_DIR/overlay"
rm -rf "$OVERLAY_DIR"
mkdir -p \
    "$OVERLAY_DIR/opt" \
    "$OVERLAY_DIR/etc/systemd/system/multi-user.target.wants" \
    "$OVERLAY_DIR/etc/systemd/system"

cp "$BINARY" "$OVERLAY_DIR/opt/knuckle"
chmod 755 "$OVERLAY_DIR/opt/knuckle"

cat > "$OVERLAY_DIR/etc/systemd/system/knuckle-installer.service" <<'UNIT'
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
UNIT

ln -sf /etc/systemd/system/knuckle-installer.service \
    "$OVERLAY_DIR/etc/systemd/system/multi-user.target.wants/knuckle-installer.service"

# Optional: carry the build host's SSH key into the live environment
if [[ -f "$HOME/.ssh/id_ed25519.pub" ]]; then
    mkdir -p "$OVERLAY_DIR/home/core/.ssh"
    cp "$HOME/.ssh/id_ed25519.pub" "$OVERLAY_DIR/home/core/.ssh/authorized_keys"
    chmod 700 "$OVERLAY_DIR/home/core/.ssh"
    chmod 600 "$OVERLAY_DIR/home/core/.ssh/authorized_keys"
fi

OVERLAY_CPIO="$BUILD_DIR/knuckle-overlay.cpio.gz"
(cd "$OVERLAY_DIR" && find . | cpio -o -H newc 2>/dev/null | gzip > "$OVERLAY_CPIO")
echo "  overlay: $(du -h "$OVERLAY_CPIO" | cut -f1)"

# Concatenate: Linux supports multiple cpio archives in initramfs
COMBINED_INITRD="$BUILD_DIR/combined-initrd.img"
cat "$INITRD" "$OVERLAY_CPIO" > "$COMBINED_INITRD"
echo "  combined: $(du -h "$COMBINED_INITRD" | cut -f1)"

# ── 4. Build FAT32 ESP with systemd-boot ─────────────────────────────────────
echo "[4/5] Building EFI System Partition (systemd-boot)..."

# Size: kernel + combined-initrd + small EFI binary + BLS files + 32 MB slack
INITRD_SIZE=$(stat -c%s "$COMBINED_INITRD")
KERNEL_SIZE=$(stat -c%s "$KERNEL")
ESP_SIZE_MB=$(( (INITRD_SIZE + KERNEL_SIZE + 32*1024*1024) / 1024 / 1024 ))

EFI_IMG="$BUILD_DIR/efi.img"
dd if=/dev/zero of="$EFI_IMG" bs=1M count="$ESP_SIZE_MB" 2>/dev/null
mformat -i "$EFI_IMG" -F ::

# Boot loader
mmd -i "$EFI_IMG" ::/EFI ::/EFI/BOOT
mcopy -i "$EFI_IMG" "$SDBOOT_EFI" ::/EFI/BOOT/BOOTX64.EFI

# systemd-boot configuration (BLS)
mmd -i "$EFI_IMG" ::/loader ::/loader/entries

# loader.conf — timeout + default
printf 'default knuckle\ntimeout 5\neditor no\n' \
    | mcopy -i "$EFI_IMG" - ::/loader/loader.conf

# Primary entry: dual console (VGA + serial)
printf 'title   Knuckle \xe2\x80\x93 Install Flatcar Container Linux\nlinux   /vmlinuz\ninitrd  /initrd.img\noptions flatcar.autologin=tty1 console=tty0 console=ttyS0,115200n8 quiet\n' \
    | mcopy -i "$EFI_IMG" - ::/loader/entries/knuckle.conf

# Alternate entry: serial console only (headless servers)
printf 'title   Knuckle \xe2\x80\x93 Install Flatcar (serial console)\nlinux   /vmlinuz\ninitrd  /initrd.img\noptions flatcar.autologin=ttyS0 console=ttyS0,115200n8\n' \
    | mcopy -i "$EFI_IMG" - ::/loader/entries/knuckle-serial.conf

# Kernel + initrd live inside the ESP (not duplicated in iso-root/)
mcopy -i "$EFI_IMG" "$KERNEL"          ::/vmlinuz
mcopy -i "$EFI_IMG" "$COMBINED_INITRD" ::/initrd.img

echo "  ESP : $(du -h "$EFI_IMG" | cut -f1)"

# ── 5. Assemble ISO ───────────────────────────────────────────────────────────
echo "[5/5] Assembling ISO..."

ISO_DIR="$BUILD_DIR/iso-root"
rm -rf "$ISO_DIR"
mkdir -p "$ISO_DIR"

# Only the ESP image goes into iso-root/; kernel+initrd are inside the ESP.
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
echo "ISO built: $ISO_OUT ($(du -h "$ISO_OUT" | cut -f1))"
echo ""
echo "Test with QEMU (UEFI):"
echo "  OVMF=/usr/share/OVMF/OVMF_CODE.fd"
echo "  qemu-system-x86_64 -m 4096 -enable-kvm \\"
echo "    -drive if=pflash,format=raw,readonly=on,file=\$OVMF \\"
echo "    -cdrom $ISO_OUT \\"
echo "    -drive if=virtio,file=target.qcow2,format=qcow2 \\"
echo "    -nographic"
echo ""
echo "Write to USB:"
echo "  sudo dd if=$ISO_OUT of=/dev/sdX bs=4M status=progress"
