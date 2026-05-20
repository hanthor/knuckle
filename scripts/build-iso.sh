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
elif [[ -f "$ROOT_DIR/bin/knuckle" ]]; then
    BINARY="$ROOT_DIR/bin/knuckle"
elif [[ -f "$ROOT_DIR/knuckle" ]]; then
    # CI fallback: binary placed at repo root (no bin/ dir in CI artifacts)
    BINARY="$ROOT_DIR/knuckle"
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
    VERSION="$(git -C "$ROOT_DIR" describe --tags --always 2>/dev/null || echo dev)"
    (cd "$ROOT_DIR" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
        go build -ldflags="-s -w -X main.version=${VERSION}" -o bin/knuckle ./cmd/knuckle)
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

# ── 3. Inject knuckle into Flatcar usr.squashfs ───────────────────────────────
# Flatcar PXE initrd = cpio(etc/ + usr.squashfs).  The squashfs is mounted at
# /usr/ in the live environment.  We unpack it, add our binary + systemd units,
# and repack.  The modified initrd is self-contained — no network, no Ignition,
# no fw_cfg tricks needed.
#
# Cache: squashfs is only rebuilt when the knuckle binary changes.
BINARY_HASH=$(sha256sum "$BINARY" | cut -c1-16)
MODIFIED_SQUASH="$BUILD_DIR/usr-modified-${BINARY_HASH}.squashfs"
MODIFIED_INITRD="$BUILD_DIR/modified-initrd-${BINARY_HASH}.cpio.gz"

if [[ -f "$MODIFIED_INITRD" ]]; then
    echo "[3/5] Using cached modified initrd (binary unchanged)."
else
    echo "[3/5] Injecting knuckle into Flatcar squashfs..."
    # Remove stale cached files from previous binary versions
    rm -f "$BUILD_DIR"/usr-modified-*.squashfs "$BUILD_DIR"/modified-initrd-*.cpio.gz

    SQUASH_WORK="$BUILD_DIR/squash-work"
    rm -rf "$SQUASH_WORK"
    mkdir -p "$SQUASH_WORK"

    # Extract usr.squashfs (and the empty etc/) from the Flatcar PXE cpio
    echo "  extracting squashfs from initrd..."
    (cd "$SQUASH_WORK" && zcat "$INITRD" | cpio -id --quiet 2>/dev/null)

    # Unpack the squashfs into squashfs-root/ (= Flatcar's /usr/ tree)
    # -no-xattrs: skip SELinux xattrs (require root); Flatcar doesn't enforce SELinux
    echo "  unpacking squashfs ($(du -h "$SQUASH_WORK/usr.squashfs" | cut -f1))..."
    unsquashfs -no-xattrs -d "$SQUASH_WORK/squashfs-root" "$SQUASH_WORK/usr.squashfs" > /dev/null 2>&1

    SR="$SQUASH_WORK/squashfs-root"   # = /usr/ in the live system

    # Install knuckle binary at /usr/bin/knuckle
    cp "$BINARY" "$SR/bin/knuckle"
    chmod 755 "$SR/bin/knuckle"

    # Install systemd units at /usr/lib/systemd/system/
    mkdir -p \
        "$SR/lib/systemd/system/multi-user.target.wants" \
        "$SR/lib/systemd/system/getty@tty1.service.d"

    # Autologin core on tty1 so the TUI gets a real PTY immediately
    cat > "$SR/lib/systemd/system/getty@tty1.service.d/autologin.conf" <<'DROPIN'
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin core --noclear %I $TERM
DROPIN

    cat > "$SR/lib/systemd/system/knuckle-installer.service" <<'UNIT'
[Unit]
Description=Knuckle Flatcar Installer
After=multi-user.target network-online.target
Wants=network-online.target
Conflicts=getty@tty1.service
ConditionPathExists=/usr/bin/knuckle

[Service]
Type=idle
ExecStart=/usr/bin/knuckle --log-file /tmp/knuckle.log
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

    ln -sf /usr/lib/systemd/system/knuckle-installer.service \
        "$SR/lib/systemd/system/multi-user.target.wants/knuckle-installer.service"

    # Repack the squashfs (lz4 for speed; -no-xattrs since we extracted without them)
    echo "  repacking squashfs..."
    mksquashfs "$SR" "$MODIFIED_SQUASH" -noappend -comp lz4 -no-xattrs -quiet

    # Rebuild the initrd cpio: etc/ (empty, from original) + modified usr.squashfs
    echo "  repacking initrd cpio..."
    (
        cd "$SQUASH_WORK"
        cp "$MODIFIED_SQUASH" usr.squashfs
        { echo .; find etc usr.squashfs; } | cpio -o -H newc --quiet | gzip > "$MODIFIED_INITRD"
    )
    rm -rf "$SQUASH_WORK"
    echo "  done: $(du -h "$MODIFIED_INITRD" | cut -f1)"
fi

# ── 4. Build FAT32 ESP with systemd-boot ─────────────────────────────────────
echo "[4/5] Building EFI System Partition (systemd-boot)..."

ISO_DIR="$BUILD_DIR/iso-root"
rm -rf "$ISO_DIR"
mkdir -p "$ISO_DIR"

INITRD_SIZE=$(stat -c%s "$MODIFIED_INITRD")
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

printf 'default knuckle\ntimeout 5\neditor no\n' \
    | mcopy -i "$EFI_IMG" - ::/loader/loader.conf

# Primary entry: VGA + serial console
printf 'title   Knuckle \xe2\x80\x93 Install Flatcar Container Linux\nlinux   /vmlinuz\ninitrd  /initrd.img\noptions console=tty0 console=ttyS0,115200n8\n' \
    | mcopy -i "$EFI_IMG" - ::/loader/entries/knuckle.conf

# Alternate entry: serial only (headless servers)
printf 'title   Knuckle \xe2\x80\x93 Install Flatcar (serial console)\nlinux   /vmlinuz\ninitrd  /initrd.img\noptions console=ttyS0,115200n8\n' \
    | mcopy -i "$EFI_IMG" - ::/loader/entries/knuckle-serial.conf

mcopy -i "$EFI_IMG" "$KERNEL"          ::/vmlinuz
mcopy -i "$EFI_IMG" "$MODIFIED_INITRD" ::/initrd.img

echo "  ESP : $(du -h "$EFI_IMG" | cut -f1)"

# ── 5. Assemble ISO ───────────────────────────────────────────────────────────
echo "[5/5] Assembling ISO..."

# iso-root/ contains only the ESP image; all content is in the ESP.
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
