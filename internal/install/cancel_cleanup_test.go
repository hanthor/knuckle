package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/projectbluefin/knuckle/internal/model"
	"github.com/projectbluefin/knuckle/internal/runner"
)

// TestInstallCancelledContextCleansUpIgnitionFile verifies that even when
// the context is cancelled, the deferred cleanupIgnitionFile() removes the
// temp file containing SSH key material from disk.
func TestInstallCancelledContextCleansUpIgnitionFile(t *testing.T) {
	spy := runner.NewSpyRunner()
	installer := NewFlatcarInstaller(spy, testLogger())

	cfg := &model.InstallConfig{
		Channel:  "stable",
		Hostname: "cancel-cleanup-test",
		Disk:     model.DiskInfo{DevPath: "/dev/sda"},
		Network:  model.NetworkConfig{Mode: model.NetworkDHCP},
		Users: []model.UserConfig{
			{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest cancel-cleanup-test"}},
		},
	}

	// Cancel context before calling Install — simulates cancellation race.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Install may succeed or fail depending on SpyRunner ctx handling,
	// but the critical property is: the ignition file must be cleaned up.
	_ = installer.Install(ctx, cfg, func(string) {})

	// After Install returns, the ignition file must have been removed by defer.
	if installer.ignitionPath != "" {
		t.Errorf("ignitionPath should be empty after cleanup, got %q", installer.ignitionPath)
	}

	// Verify no knuckle-ignition temp files remain with our test marker.
	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "knuckle-ignition-*.json"))
	for _, m := range matches {
		content, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		if strings.Contains(string(content), "cancel-cleanup-test") {
			t.Errorf("found orphaned ignition file with test SSH key: %s", m)
			_ = os.Remove(m)
		}
	}
}

// TestInstallWipeFailureCleansUpIgnitionFile verifies that when wipefs fails
// mid-install, the ignition file is still cleaned up (SSH keys not left on disk).
func TestInstallWipeFailureCleansUpIgnitionFile(t *testing.T) {
	spy := runner.NewSpyRunner()
	spy.StubError("wipefs --all --force /dev/sda", os.ErrPermission)

	installer := NewFlatcarInstaller(spy, testLogger())

	cfg := &model.InstallConfig{
		Channel:  "stable",
		Hostname: "wipe-fail-cleanup",
		Disk:     model.DiskInfo{DevPath: "/dev/sda"},
		Network:  model.NetworkConfig{Mode: model.NetworkDHCP},
		Users: []model.UserConfig{
			{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest wipe-fail-cleanup"}},
		},
	}

	err := installer.Install(context.Background(), cfg, func(string) {})
	if err == nil {
		t.Fatal("expected error from wipefs failure")
	}

	// Critical security property: ignition file with SSH keys must be gone.
	if installer.ignitionPath != "" {
		t.Errorf("ignitionPath should be empty after cleanup, got %q", installer.ignitionPath)
	}

	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "knuckle-ignition-*.json"))
	for _, m := range matches {
		content, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		if strings.Contains(string(content), "wipe-fail-cleanup") {
			t.Errorf("found orphaned ignition file with test SSH key: %s", m)
			_ = os.Remove(m)
		}
	}
}

// TestInstallSuccessPathCleansUpIgnitionFile verifies that even on the happy
// path, the deferred cleanup removes the temp ignition file from disk.
func TestInstallSuccessPathCleansUpIgnitionFile(t *testing.T) {
	spy := runner.NewSpyRunner()
	installer := NewFlatcarInstaller(spy, testLogger())

	cfg := &model.InstallConfig{
		Channel:  "stable",
		Hostname: "success-cleanup",
		Disk:     model.DiskInfo{DevPath: "/dev/sda"},
		Network:  model.NetworkConfig{Mode: model.NetworkDHCP},
		Users: []model.UserConfig{
			{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest success-cleanup"}},
		},
	}

	err := installer.Install(context.Background(), cfg, func(string) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Even on success, cleanup must have run — no SSH key material on disk.
	if installer.ignitionPath != "" {
		t.Errorf("ignitionPath should be empty after successful install, got %q", installer.ignitionPath)
	}

	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "knuckle-ignition-*.json"))
	for _, m := range matches {
		content, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		if strings.Contains(string(content), "success-cleanup") {
			t.Errorf("found orphaned ignition file: %s", m)
			_ = os.Remove(m)
		}
	}
}

// TestInstallByIdDiskPathUsedThroughEntirePipeline verifies that when
// Disk.Path (/dev/disk/by-id/...) is set, all commands (wipefs, flatcar-install,
// sfdisk) receive the stable path, not DevPath.
func TestInstallByIdDiskPathUsedThroughEntirePipeline(t *testing.T) {
	spy := runner.NewSpyRunner()
	installer := NewFlatcarInstaller(spy, testLogger())

	byIDPath := "/dev/disk/by-id/scsi-SATA_VBOX_HARDDISK_VB12345678-90abcdef"
	cfg := &model.InstallConfig{
		Channel:  "stable",
		Hostname: "byid-test",
		Disk: model.DiskInfo{
			DevPath: "/dev/sda",
			Path:    byIDPath,
		},
		Network: model.NetworkConfig{Mode: model.NetworkDHCP},
		Users: []model.UserConfig{
			{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest byid-test"}},
		},
	}

	err := installer.Install(context.Background(), cfg, func(string) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify wipefs received the by-id path.
	var wipeCalled bool
	for _, call := range spy.Calls {
		if call.Name == "wipefs" {
			wipeCalled = true
			lastArg := call.Args[len(call.Args)-1]
			if lastArg != byIDPath {
				t.Errorf("wipefs received disk=%q, want by-id path %q", lastArg, byIDPath)
			}
		}
	}
	if !wipeCalled {
		t.Error("wipefs was not called")
	}

	// Verify flatcar-install received the by-id path via -d flag.
	var fiCalled bool
	for _, call := range spy.Calls {
		if call.Name == "flatcar-install" {
			fiCalled = true
			for i, arg := range call.Args {
				if arg == "-d" && i+1 < len(call.Args) {
					if call.Args[i+1] != byIDPath {
						t.Errorf("flatcar-install -d got %q, want %q", call.Args[i+1], byIDPath)
					}
				}
			}
		}
	}
	if !fiCalled {
		t.Error("flatcar-install was not called")
	}

	// Verify sfdisk --relocate received the by-id path.
	var sfdiskCalled bool
	for _, call := range spy.Calls {
		if call.Name == "sfdisk" {
			sfdiskCalled = true
			lastArg := call.Args[len(call.Args)-1]
			if lastArg != byIDPath {
				t.Errorf("sfdisk received disk=%q, want by-id path %q", lastArg, byIDPath)
			}
		}
	}
	if !sfdiskCalled {
		t.Error("sfdisk was not called")
	}
}

// TestInstallByIdDiskPathFallsBackToDevPath verifies that when Disk.Path
// is empty, DevPath is used (regression guard for installDiskPath logic).
func TestInstallByIdDiskPathFallsBackToDevPath(t *testing.T) {
	spy := runner.NewSpyRunner()
	installer := NewFlatcarInstaller(spy, testLogger())

	cfg := &model.InstallConfig{
		Channel:  "stable",
		Hostname: "devpath-fallback",
		Disk: model.DiskInfo{
			DevPath: "/dev/nvme0n1",
			Path:    "", // empty — should fall back to DevPath
		},
		Network: model.NetworkConfig{Mode: model.NetworkDHCP},
		Users: []model.UserConfig{
			{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest devpath-fallback"}},
		},
	}

	err := installer.Install(context.Background(), cfg, func(string) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// wipefs should get /dev/nvme0n1
	for _, call := range spy.Calls {
		if call.Name == "wipefs" {
			lastArg := call.Args[len(call.Args)-1]
			if lastArg != "/dev/nvme0n1" {
				t.Errorf("wipefs received %q, want /dev/nvme0n1", lastArg)
			}
		}
	}
}
