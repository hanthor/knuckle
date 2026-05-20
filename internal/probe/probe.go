package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/castrojo/knuckle/internal/model"
	"github.com/castrojo/knuckle/internal/runner"
)

// pciDevicesPath is the root of the PCI sysfs device tree.
// Override in tests to use a temporary directory.
var pciDevicesPath = "/sys/bus/pci/devices"

// Prober is the interface for system hardware discovery
type Prober interface {
	ListDisks(ctx context.Context) ([]model.DiskInfo, error)
	ListNetworkInterfaces(ctx context.Context) ([]model.NetworkInterface, error)
}

// SystemProber uses real system commands via the runner
type SystemProber struct {
	Runner runner.Runner
}

func NewSystemProber(r runner.Runner) *SystemProber {
	return &SystemProber{Runner: r}
}

// lsblkOutput matches the JSON output of `lsblk --json --bytes --output NAME,PATH,MODEL,SERIAL,SIZE,TRAN,RM,TYPE,FSTYPE,LABEL,MOUNTPOINT`
type lsblkOutput struct {
	Blockdevices []lsblkDevice `json:"blockdevices"`
}

type lsblkDevice struct {
	Name       string        `json:"name"`
	Path       string        `json:"path"`
	Model      *string       `json:"model"`
	Serial     *string       `json:"serial"`
	Size       json.Number   `json:"size"`
	Tran       *string       `json:"tran"`
	RM         bool          `json:"rm"`
	Type       string        `json:"type"`
	FSType     *string       `json:"fstype"`
	Label      *string       `json:"label"`
	MountPoint *string       `json:"mountpoint"`
	Children   []lsblkDevice `json:"children,omitempty"`
}

func (p *SystemProber) ListDisks(ctx context.Context) ([]model.DiskInfo, error) {
	result, err := p.Runner.Run(ctx, "lsblk", "--json", "--bytes", "--output", "NAME,PATH,MODEL,SERIAL,SIZE,TRAN,RM,TYPE,FSTYPE,LABEL,MOUNTPOINT")
	if err != nil {
		return nil, fmt.Errorf("lsblk: %w", err)
	}

	var output lsblkOutput
	if err := json.Unmarshal([]byte(result.Stdout), &output); err != nil {
		return nil, fmt.Errorf("parsing lsblk output: %w", err)
	}

	var disks []model.DiskInfo
	for _, dev := range output.Blockdevices {
		if dev.Type != "disk" {
			continue
		}

		// Skip removable devices
		if dev.RM {
			continue
		}

		size, _ := dev.Size.Int64()

		// Skip disks smaller than 8GB (flatcar-install minimum)
		if uint64(size) < 8*1024*1024*1024 {
			continue
		}

		// Skip the boot disk (has mounted partitions like / or /boot)
		isBootDisk := false
		for _, child := range dev.Children {
			mp := deref(child.MountPoint)
			if mp == "/" || mp == "/boot" || mp == "/usr" || mp == "/sysroot" {
				isBootDisk = true
				break
			}
		}
		if isBootDisk {
			continue
		}

		disk := model.DiskInfo{
			DevPath:   dev.Path,
			Path:      dev.Path,
			Model:     deref(dev.Model),
			Serial:    deref(dev.Serial),
			Size:      uint64(size),
			SizeHuman: humanSize(uint64(size)),
			Transport: deref(dev.Tran),
			Removable: dev.RM,
		}

		// Resolve /dev/disk/by-id path for stable identification
		disk.Path = resolveByIDPath(disk.DevPath)

		// Parse partitions from children
		for _, child := range dev.Children {
			if child.Type == "part" {
				childSize, _ := child.Size.Int64()
				disk.Partitions = append(disk.Partitions, model.PartitionInfo{
					Path:       child.Path,
					Label:      deref(child.Label),
					FSType:     deref(child.FSType),
					Size:       uint64(childSize),
					MountPoint: deref(child.MountPoint),
				})
			}
		}

		disks = append(disks, disk)
	}

	return disks, nil
}

func (p *SystemProber) ListNetworkInterfaces(ctx context.Context) ([]model.NetworkInterface, error) {
	result, err := p.Runner.Run(ctx, "ip", "-j", "addr", "show")
	if err != nil {
		return nil, fmt.Errorf("ip addr: %w", err)
	}

	var ipOutput []ipAddrEntry
	if err := json.Unmarshal([]byte(result.Stdout), &ipOutput); err != nil {
		return nil, fmt.Errorf("parsing ip output: %w", err)
	}

	var ifaces []model.NetworkInterface
	for _, entry := range ipOutput {
		// Skip loopback
		if entry.IfName == "lo" {
			continue
		}

		iface := model.NetworkInterface{
			Name:   entry.IfName,
			State:  entry.OperState,
			Driver: entry.LinkType,
		}

		// Extract MAC address
		if entry.Address != "" {
			iface.MAC = entry.Address
		}

		// Extract IP addresses
		for _, addr := range entry.AddrInfo {
			switch addr.Family {
			case "inet":
				iface.IPv4Addrs = append(iface.IPv4Addrs, fmt.Sprintf("%s/%d", addr.Local, addr.PrefixLen))
			case "inet6":
				iface.IPv6Addrs = append(iface.IPv6Addrs, fmt.Sprintf("%s/%d", addr.Local, addr.PrefixLen))
			}
		}

		ifaces = append(ifaces, iface)
	}

	return ifaces, nil
}

type ipAddrEntry struct {
	IfName    string       `json:"ifname"`
	Address   string       `json:"address"`
	OperState string       `json:"operstate"`
	LinkType  string       `json:"link_type"`
	AddrInfo  []ipAddrInfo `json:"addr_info"`
}

type ipAddrInfo struct {
	Family    string `json:"family"`
	Local     string `json:"local"`
	PrefixLen int    `json:"prefixlen"`
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func humanSize(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// resolveByIDPath finds the /dev/disk/by-id/ symlink for a device path.
// Falls back to devPath if /dev/disk/by-id is unavailable (e.g., in CI).
func resolveByIDPath(devPath string) string {
	byIDDir := "/dev/disk/by-id/"
	entries, err := os.ReadDir(byIDDir)
	if err != nil {
		return devPath // fallback to raw device path
	}
	for _, entry := range entries {
		link := filepath.Join(byIDDir, entry.Name())
		target, err := os.Readlink(link)
		if err != nil {
			continue
		}
		// Resolve relative symlink
		absTarget := filepath.Join(byIDDir, target)
		absTarget, _ = filepath.Abs(absTarget)
		if absTarget == devPath {
			return link
		}
	}
	return devPath
}

// NvidiaGPUInfo represents a detected NVIDIA GPU found via the PCI sysfs tree.
type NvidiaGPUInfo struct {
	PCIAddress string // e.g. "0000:01:00.0"
	PCIClass   string // e.g. "0x030200" (3D controller)
}

// DetectNvidiaGPUs scans the PCI device tree for NVIDIA display/compute controllers.
// No NVIDIA driver needs to be loaded — reads /sys/bus/pci/devices/ directly.
// The installer runs on the target machine, so any GPU detected here will also be
// present on the installed system, making this safe for sysext auto-selection.
// Returns an empty slice when no NVIDIA GPUs are found or /sys is unavailable.
func DetectNvidiaGPUs() []NvidiaGPUInfo {
	return detectNvidiaGPUsFromPath(pciDevicesPath)
}

func detectNvidiaGPUsFromPath(devPath string) []NvidiaGPUInfo {
	const nvidiaVendorID = "0x10de"

	vendorPaths, err := filepath.Glob(filepath.Join(devPath, "*", "vendor"))
	if err != nil || len(vendorPaths) == 0 {
		return nil
	}

	var gpus []NvidiaGPUInfo
	for _, vendorFile := range vendorPaths {
		data, err := os.ReadFile(vendorFile)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) != nvidiaVendorID {
			continue
		}
		dir := filepath.Dir(vendorFile)
		classData, err := os.ReadFile(filepath.Join(dir, "class"))
		if err != nil {
			continue
		}
		class := strings.TrimSpace(string(classData))
		// Display and compute controllers occupy the 0x03xxxx PCI class range:
		// 0x030000 = VGA compatible, 0x030200 = 3D controller, etc.
		if strings.HasPrefix(class, "0x03") {
			gpus = append(gpus, NvidiaGPUInfo{
				PCIAddress: filepath.Base(dir),
				PCIClass:   class,
			})
		}
	}
	return gpus
}
