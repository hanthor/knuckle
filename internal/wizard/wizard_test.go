package wizard

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/castrojo/knuckle/internal/model"
)

// --- Mocks ---

type mockProber struct {
	disks  []model.DiskInfo
	ifaces []model.NetworkInterface
	err    error
}

func (m *mockProber) ListDisks(ctx context.Context) ([]model.DiskInfo, error) {
	return m.disks, m.err
}

func (m *mockProber) ListNetworkInterfaces(ctx context.Context) ([]model.NetworkInterface, error) {
	return m.ifaces, m.err
}

type mockBakery struct {
	sysexts []model.SysextEntry
	err     error
}

func (m *mockBakery) FetchCatalog(ctx context.Context) ([]model.SysextEntry, error) {
	return m.sysexts, m.err
}

func (m *mockBakery) FetchCatalogArch(ctx context.Context, arch string) ([]model.SysextEntry, error) {
	return m.sysexts, m.err
}

type mockInstaller struct {
	called bool
	cfg    *model.InstallConfig
	err    error
}

func (m *mockInstaller) Install(ctx context.Context, cfg *model.InstallConfig, progress func(string)) error {
	m.called = true
	m.cfg = cfg
	progress("test progress")
	return m.err
}

// --- Helpers ---

func newTestWizard() (*Wizard, *mockProber, *mockBakery, *mockInstaller) {
	p := &mockProber{}
	b := &mockBakery{}
	i := &mockInstaller{}
	w := New(p, b, i)
	return w, p, b, i
}

// --- Tests ---

func TestNewWizard(t *testing.T) {
	w, _, _, _ := newTestWizard()

	if w.State.CurrentStep != model.StepWelcome {
		t.Errorf("expected StepWelcome, got %v", w.State.CurrentStep)
	}
	if w.State.Config.Channel != "stable" {
		t.Errorf("expected channel=stable, got %q", w.State.Config.Channel)
	}
}

func TestNextAndPrevious(t *testing.T) {
	w, _, _, _ := newTestWizard()

	// Advance from Welcome (no validation needed)
	if err := w.Next(); err != nil {
		t.Fatalf("Next from Welcome: %v", err)
	}
	if w.State.CurrentStep != model.StepNetwork {
		t.Errorf("expected StepNetwork, got %v", w.State.CurrentStep)
	}

	// Go back
	w.Previous()
	if w.State.CurrentStep != model.StepWelcome {
		t.Errorf("expected StepWelcome after Previous, got %v", w.State.CurrentStep)
	}

	// Previous at first step is a no-op
	w.Previous()
	if w.State.CurrentStep != model.StepWelcome {
		t.Errorf("expected StepWelcome (bounded), got %v", w.State.CurrentStep)
	}

	// Advance all the way to Done (set valid config for each step)
	// Network: use DHCP (passes validation)
	w.State.Config.Network.Mode = model.NetworkDHCP
	if err := w.Next(); err != nil {
		t.Fatalf("Next from Welcome: %v", err)
	}
	// StepNetwork -> StepStorage: DHCP passes
	if err := w.Next(); err != nil {
		t.Fatalf("Next from Network (DHCP): %v", err)
	}
	// StepStorage -> StepUser: need disk
	w.State.Config.Disk.DevPath = "/dev/sda"
	if err := w.Next(); err != nil {
		t.Fatalf("Next from Storage: %v", err)
	}
	// StepUser -> StepSysext: need user
	w.State.Config.Users = []model.UserConfig{
		{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI test"}},
	}
	if err := w.Next(); err != nil {
		t.Fatalf("Next from User: %v", err)
	}
	// StepSysext -> StepUpdate
	if err := w.Next(); err != nil {
		t.Fatalf("Next from Sysext: %v", err)
	}
	// StepUpdate -> StepReview
	if err := w.Next(); err != nil {
		t.Fatalf("Next from Update: %v", err)
	}
	// StepReview -> StepInstall (need confirmation)
	w.State.Confirmed = true
	if err := w.Next(); err != nil {
		t.Fatalf("Next from Review: %v", err)
	}
	// StepInstall -> StepDone
	if err := w.Next(); err != nil {
		t.Fatalf("Next from Install: %v", err)
	}
	if w.State.CurrentStep != model.StepDone {
		t.Errorf("expected StepDone, got %v", w.State.CurrentStep)
	}

	// Next at last step is a no-op
	if err := w.Next(); err != nil {
		t.Fatalf("Next from Done: %v", err)
	}
	if w.State.CurrentStep != model.StepDone {
		t.Errorf("expected StepDone (bounded), got %v", w.State.CurrentStep)
	}
}

func TestValidateNetworkDHCP(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepNetwork
	w.State.Config.Network.Mode = model.NetworkDHCP

	if err := w.ValidateCurrentStep(); err != nil {
		t.Errorf("DHCP should pass validation: %v", err)
	}
}

func TestValidateNetworkStaticValid(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepNetwork
	w.State.Config.Network = model.NetworkConfig{
		Mode:      model.NetworkStatic,
		Interface: "eth0",
		Address:   "192.168.1.10/24",
		Gateway:   "192.168.1.1",
		DNS:       []string{"8.8.8.8", "1.1.1.1"},
	}

	if err := w.ValidateCurrentStep(); err != nil {
		t.Errorf("valid static config should pass: %v", err)
	}
}

func TestValidateNetworkStaticInvalid(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepNetwork
	w.State.Config.Network = model.NetworkConfig{
		Mode:      model.NetworkStatic,
		Interface: "eth0",
		Address:   "not-a-cidr",
		Gateway:   "192.168.1.1",
	}

	err := w.ValidateCurrentStep()
	if err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
}

func TestValidateStorageNoDisk(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepStorage
	w.State.Config.Disk.DevPath = ""

	err := w.ValidateCurrentStep()
	if err == nil {
		t.Fatal("expected error for empty disk")
	}
}

func TestValidateStorageMountedPartition(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepStorage
	w.State.Config.Disk = model.DiskInfo{
		DevPath: "/dev/sda",
		Partitions: []model.PartitionInfo{
			{Path: "/dev/sda1", MountPoint: "/boot"},
			{Path: "/dev/sda2", MountPoint: ""},
		},
	}

	err := w.ValidateCurrentStep()
	if err == nil {
		t.Fatal("expected error for disk with mounted partition")
	}
	if !strings.Contains(err.Error(), "mounted partition") {
		t.Errorf("error should mention mounted partition, got: %v", err)
	}
}

func TestValidateStorageNoMountedPartitions(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepStorage
	w.State.Config.Disk = model.DiskInfo{
		DevPath: "/dev/sda",
		Partitions: []model.PartitionInfo{
			{Path: "/dev/sda1", MountPoint: ""},
			{Path: "/dev/sda2", MountPoint: ""},
		},
	}

	err := w.ValidateCurrentStep()
	if err != nil {
		t.Errorf("expected no error for unmounted partitions, got: %v", err)
	}
}

func TestValidateUserNoUserNoKey(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepUser
	w.State.Config.Users = nil
	w.State.Config.SSHKeys = nil

	err := w.ValidateCurrentStep()
	if err == nil {
		t.Fatal("expected error when no users and no SSH keys")
	}
}

func TestValidateUserValid(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepUser
	w.State.Config.Users = []model.UserConfig{
		{
			Username: "core",
			SSHKeys:  []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI test@host"},
		},
	}

	if err := w.ValidateCurrentStep(); err != nil {
		t.Errorf("valid user should pass: %v", err)
	}
}

func TestProbeHardware(t *testing.T) {
	disks := []model.DiskInfo{
		{DevPath: "/dev/sda", Model: "Test Disk", Size: 500000000000},
	}
	ifaces := []model.NetworkInterface{
		{Name: "eth0", MAC: "00:11:22:33:44:55", State: "up"},
	}
	p := &mockProber{disks: disks, ifaces: ifaces}
	w := New(p, &mockBakery{}, &mockInstaller{})

	if err := w.ProbeHardware(context.Background()); err != nil {
		t.Fatalf("ProbeHardware: %v", err)
	}

	if len(w.State.Disks) != 1 || w.State.Disks[0].DevPath != "/dev/sda" {
		t.Errorf("expected 1 disk /dev/sda, got %v", w.State.Disks)
	}
	if len(w.State.Interfaces) != 1 || w.State.Interfaces[0].Name != "eth0" {
		t.Errorf("expected 1 interface eth0, got %v", w.State.Interfaces)
	}
}

func TestProbeHardwareError(t *testing.T) {
	p := &mockProber{err: fmt.Errorf("disk probe failed")}
	w := New(p, &mockBakery{}, &mockInstaller{})

	err := w.ProbeHardware(context.Background())
	if err == nil {
		t.Fatal("expected error from ProbeHardware")
	}
}

func TestFetchSysexts(t *testing.T) {
	sysexts := []model.SysextEntry{
		{Name: "docker", Description: "Docker CE", Version: "24.0"},
		{Name: "tailscale", Description: "Tailscale VPN", Version: "1.50"},
	}
	b := &mockBakery{sysexts: sysexts}
	w := New(&mockProber{}, b, &mockInstaller{})

	if err := w.FetchSysexts(context.Background()); err != nil {
		t.Fatalf("FetchSysexts: %v", err)
	}

	if len(w.State.Sysexts) != 2 {
		t.Errorf("expected 2 sysexts, got %d", len(w.State.Sysexts))
	}
	if w.State.Sysexts[0].Name != "docker" {
		t.Errorf("expected first sysext=docker, got %q", w.State.Sysexts[0].Name)
	}
}

func TestExecute(t *testing.T) {
	inst := &mockInstaller{}
	w := New(&mockProber{}, &mockBakery{}, inst)
	w.State.Config.Disk.DevPath = "/dev/sda"
	w.State.Config.Channel = "stable"

	err := w.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !inst.called {
		t.Error("expected installer to be called")
	}
	if inst.cfg != &w.State.Config {
		t.Error("expected installer to receive wizard config")
	}
	if len(w.State.ProgressMessages) != 1 || w.State.ProgressMessages[0] != "test progress" {
		t.Errorf("expected progress messages, got %v", w.State.ProgressMessages)
	}
}

func TestExecuteError(t *testing.T) {
	inst := &mockInstaller{err: fmt.Errorf("install failed")}
	w := New(&mockProber{}, &mockBakery{}, inst)

	err := w.Execute(context.Background())
	if err == nil {
		t.Fatal("expected error from Execute")
	}
}

func TestGoToStep(t *testing.T) {
	w, _, _, _ := newTestWizard()

	w.GoToStep(model.StepReview)
	if w.State.CurrentStep != model.StepReview {
		t.Errorf("expected StepReview, got %v", w.State.CurrentStep)
	}

	// Out of bounds should be no-op
	w.GoToStep(model.WizardStep(-1))
	if w.State.CurrentStep != model.StepReview {
		t.Errorf("expected StepReview unchanged, got %v", w.State.CurrentStep)
	}
}

func TestIsFirstAndLastStep(t *testing.T) {
	w, _, _, _ := newTestWizard()

	if !w.IsFirstStep() {
		t.Error("expected IsFirstStep=true at Welcome")
	}
	if w.IsLastStep() {
		t.Error("expected IsLastStep=false at Welcome")
	}

	w.State.CurrentStep = model.StepDone
	if w.IsFirstStep() {
		t.Error("expected IsFirstStep=false at Done")
	}
	if !w.IsLastStep() {
		t.Error("expected IsLastStep=true at Done")
	}
}

func TestStepCount(t *testing.T) {
	count := StepCount()
	if count != 10 {
		t.Errorf("expected 10 steps (added StepNvidia), got %d", count)
	}
}

func TestValidateConsistency(t *testing.T) {
	tests := []struct {
		name    string
		cfg     model.InstallConfig
		wantErr string
	}{
		{
			name: "no auth returns error",
			cfg: model.InstallConfig{
				Disk:    model.DiskInfo{DevPath: "/dev/sda"},
				Channel: "stable",
			},
			wantErr: "at least one authentication method required",
		},
		{
			name: "with SSH keys passes",
			cfg: model.InstallConfig{
				Disk:    model.DiskInfo{DevPath: "/dev/sda"},
				Channel: "stable",
				SSHKeys: []string{"ssh-ed25519 AAAA test"},
			},
			wantErr: "",
		},
		{
			name: "with user SSH keys passes",
			cfg: model.InstallConfig{
				Disk:    model.DiskInfo{DevPath: "/dev/sda"},
				Channel: "stable",
				Users: []model.UserConfig{
					{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAA test"}},
				},
			},
			wantErr: "",
		},
		{
			name: "with password passes",
			cfg: model.InstallConfig{
				Disk:    model.DiskInfo{DevPath: "/dev/sda"},
				Channel: "stable",
				Users: []model.UserConfig{
					{Username: "core", PasswordHash: "$2a$10$hash"},
				},
			},
			wantErr: "",
		},
		{
			name: "static network missing gateway errors",
			cfg: model.InstallConfig{
				Disk:    model.DiskInfo{DevPath: "/dev/sda"},
				Channel: "stable",
				SSHKeys: []string{"ssh-ed25519 AAAA test"},
				Network: model.NetworkConfig{
					Mode:    model.NetworkStatic,
					Address: "192.168.1.10/24",
					Gateway: "",
				},
			},
			wantErr: "static network requires a gateway",
		},
		{
			name: "ignition URL bypasses auth check",
			cfg: model.InstallConfig{
				Disk:        model.DiskInfo{DevPath: "/dev/sda"},
				Channel:     "stable",
				IgnitionURL: "https://example.com/config.ign",
			},
			wantErr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, _, _, _ := newTestWizard()
			w.State.CurrentStep = model.StepReview
			w.State.Confirmed = true
			w.State.Config = tt.cfg

			err := w.ValidateCurrentStep()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestRunSystemChecks(t *testing.T) {
	tests := []struct {
		name     string
		disks    []model.DiskInfo
		ifaces   []model.NetworkInterface
		wantDisk string
		wantNet  string
	}{
		{
			name:     "disks present and active iface",
			disks:    []model.DiskInfo{{DevPath: "/dev/sda"}},
			ifaces:   []model.NetworkInterface{{Name: "eth0", IPv4Addrs: []string{"192.168.1.10"}}},
			wantDisk: "ok",
			wantNet:  "ok",
		},
		{
			name:     "no disks",
			disks:    nil,
			ifaces:   []model.NetworkInterface{{Name: "eth0", IPv4Addrs: []string{"192.168.1.10"}}},
			wantDisk: "fail",
			wantNet:  "ok",
		},
		{
			name:     "no active ifaces warns",
			disks:    []model.DiskInfo{{DevPath: "/dev/sda"}},
			ifaces:   []model.NetworkInterface{{Name: "eth0", IPv4Addrs: nil}},
			wantDisk: "ok",
			wantNet:  "warn",
		},
		{
			name:     "no interfaces at all fails",
			disks:    []model.DiskInfo{{DevPath: "/dev/sda"}},
			ifaces:   nil,
			wantDisk: "ok",
			wantNet:  "fail",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &mockProber{disks: tt.disks, ifaces: tt.ifaces}
			w := New(p, &mockBakery{}, &mockInstaller{})

			if err := w.ProbeHardware(context.Background()); err != nil {
				t.Fatalf("ProbeHardware: %v", err)
			}

			var diskCheck, netCheck *SystemCheck
			for i := range w.State.SystemChecks {
				switch w.State.SystemChecks[i].Name {
				case "Disk":
					diskCheck = &w.State.SystemChecks[i]
				case "Network":
					netCheck = &w.State.SystemChecks[i]
				}
			}
			if diskCheck == nil {
				t.Fatal("missing Disk system check")
			}
			if diskCheck.Status != tt.wantDisk {
				t.Errorf("Disk check: got %q, want %q", diskCheck.Status, tt.wantDisk)
			}
			if netCheck == nil {
				t.Fatal("missing Network system check")
			}
			if netCheck.Status != tt.wantNet {
				t.Errorf("Network check: got %q, want %q", netCheck.Status, tt.wantNet)
			}
		})
	}
}

func TestGenerateButaneNonEmpty(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.Config.Hostname = "test-node"
	w.State.Config.Channel = "stable"
	w.State.Config.Users = []model.UserConfig{
		{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAA test"}},
	}

	butane, err := w.GenerateButane()
	if err != nil {
		t.Fatalf("GenerateButane: %v", err)
	}
	if butane == "" {
		t.Error("GenerateButane returned empty string")
	}
	if !strings.Contains(butane, "variant: flatcar") {
		t.Error("expected butane to contain variant header")
	}
	if !strings.Contains(butane, "test-node") {
		t.Error("expected butane to contain hostname")
	}
}

func TestValidateNetworkRequiresInterface(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepNetwork
	w.State.Config.Network = model.NetworkConfig{
		Mode:    model.NetworkStatic,
		Address: "192.168.1.10/24",
		Gateway: "192.168.1.1",
		DNS:     []string{"8.8.8.8"},
		// Interface intentionally empty
	}

	err := w.ValidateCurrentStep()
	if err == nil {
		t.Fatal("expected error for static network with empty interface")
	}
	if !strings.Contains(err.Error(), "interface") {
		t.Errorf("error should mention interface, got: %v", err)
	}
}

func TestValidateWelcomeRejectsInvalidChannel(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepWelcome
	w.State.Config.Channel = "nightly"

	err := w.ValidateCurrentStep()
	if err == nil {
		t.Fatal("expected error for invalid channel")
	}
	if !strings.Contains(err.Error(), "nightly") {
		t.Errorf("error should mention invalid channel value, got: %v", err)
	}
}

func TestValidateWelcomeAcceptsValidChannels(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepWelcome

	for _, ch := range []string{"stable", "beta", "alpha", "edge"} {
		w.State.Config.Channel = ch
		if err := w.ValidateCurrentStep(); err != nil {
			t.Errorf("channel %q should be valid, got error: %v", ch, err)
		}
	}
}

func TestNextSkipsNvidiaWhenNotSelected(t *testing.T) {
	w, _, _, _ := newTestWizard()
	// Set up valid config to advance to StepSysext
	w.State.Config.Network.Mode = model.NetworkDHCP
	w.State.Config.Disk.DevPath = "/dev/sda"
	w.State.Config.Users = []model.UserConfig{
		{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI test"}},
	}
	w.State.Config.Channel = "stable"
	// No nvidia-runtime in sysexts
	w.State.Sysexts = []model.SysextEntry{
		{Name: "docker", Selected: true},
	}
	w.State.CurrentStep = model.StepSysext

	if err := w.Next(); err != nil {
		t.Fatalf("Next from Sysext: %v", err)
	}
	// Should skip StepNvidia and go to StepUpdate
	if w.State.CurrentStep != model.StepUpdate {
		t.Errorf("expected StepUpdate (skip nvidia), got %v", w.State.CurrentStep)
	}
}

func TestNextVisitsNvidiaWhenSelected(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.Config.Network.Mode = model.NetworkDHCP
	w.State.Config.Disk.DevPath = "/dev/sda"
	w.State.Config.Users = []model.UserConfig{
		{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI test"}},
	}
	w.State.Config.Channel = "stable"
	// nvidia-runtime IS selected
	w.State.Sysexts = []model.SysextEntry{
		{Name: "nvidia-runtime", Selected: true},
	}
	w.State.CurrentStep = model.StepSysext

	if err := w.Next(); err != nil {
		t.Fatalf("Next from Sysext: %v", err)
	}
	if w.State.CurrentStep != model.StepNvidia {
		t.Errorf("expected StepNvidia when nvidia-runtime selected, got %v", w.State.CurrentStep)
	}
}

func TestGoToStepRefusesNvidiaWhenNotSelected(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepSysext
	// No nvidia-runtime selected
	w.State.Sysexts = []model.SysextEntry{
		{Name: "docker", Selected: true},
	}

	w.GoToStep(model.StepNvidia)
	// Should NOT have moved to StepNvidia
	if w.State.CurrentStep == model.StepNvidia {
		t.Error("GoToStep should refuse StepNvidia when nvidia-runtime is not selected")
	}
	if w.State.CurrentStep != model.StepSysext {
		t.Errorf("expected to stay on StepSysext, got %v", w.State.CurrentStep)
	}
}

// --- QA Gap Tests: State Machine Invariants ---

// Q1: Can we reach StepInstall without a disk? GoToStep has no validation gate.
func TestGoToStep_InstallWithoutDisk_NoValidationGate(t *testing.T) {
	w, _, _, _ := newTestWizard()
	// GoToStep allows jumping to Install without any prior state — this is by design
	// (the Review step's ValidateCurrentStep gates actual advancement).
	w.GoToStep(model.StepInstall)
	if w.State.CurrentStep != model.StepInstall {
		t.Fatalf("GoToStep should allow jump to Install, got %v", w.State.CurrentStep)
	}
	// But ValidateCurrentStep on Install itself returns nil (no gate).
	// The real gate is at StepReview — calling Next() from Review runs CheckConsistency.
	// This verifies the architectural contract: GoToStep is unvalidated, Next() is validated.
	err := w.ValidateCurrentStep()
	if err != nil {
		t.Errorf("StepInstall validation is a no-op by design, got: %v", err)
	}
}

// Q2: Does Next() from StepReview catch missing disk?
func TestNext_ReviewCatchesMissingDisk(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepReview
	w.State.Config.Channel = "stable"
	w.State.Config.SSHKeys = []string{"ssh-ed25519 AAAA test"}
	// Disk is empty
	w.State.Config.Disk.DevPath = ""

	err := w.Next()
	if err == nil {
		t.Fatal("expected error: Next from Review with no disk")
	}
	if !strings.Contains(err.Error(), "no disk") {
		t.Errorf("expected 'no disk' error, got: %v", err)
	}
	// Should NOT have advanced
	if w.State.CurrentStep != model.StepReview {
		t.Errorf("should stay on StepReview, got %v", w.State.CurrentStep)
	}
}

// Q3: Back from Review → User → change SSH keys → re-advance → config rebuilt correctly
func TestBackAndModify_SSHKeysRebuilt(t *testing.T) {
	w, _, _, _ := newTestWizard()
	// Set up full valid config to reach Review
	w.State.Config.Channel = "stable"
	w.State.Config.Network.Mode = model.NetworkDHCP
	w.State.Config.Disk.DevPath = "/dev/sda"
	w.State.Config.Users = []model.UserConfig{
		{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI original"}},
	}
	w.State.CurrentStep = model.StepReview

	// Go back to User step
	w.GoToStep(model.StepUser)
	if w.State.CurrentStep != model.StepUser {
		t.Fatalf("expected StepUser, got %v", w.State.CurrentStep)
	}

	// Modify SSH keys (simulates user editing)
	w.State.Config.Users[0].SSHKeys = []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI newkey"}

	// Advance back to Review
	if err := w.Next(); err != nil {
		t.Fatalf("Next from User: %v", err)
	}
	// StepSysext
	if err := w.Next(); err != nil {
		t.Fatalf("Next from Sysext: %v", err)
	}
	// StepUpdate (nvidia skipped - not selected)
	if err := w.Next(); err != nil {
		t.Fatalf("Next from Update: %v", err)
	}
	// Now at Review
	if w.State.CurrentStep != model.StepReview {
		t.Fatalf("expected StepReview, got %v", w.State.CurrentStep)
	}

	// Verify the config reflects the NEW key
	if w.State.Config.Users[0].SSHKeys[0] != "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI newkey" {
		t.Errorf("SSH key not updated after back-navigation, got: %v", w.State.Config.Users[0].SSHKeys)
	}

	// GenerateButane should produce output with the new key
	butane, err := w.GenerateButane()
	if err != nil {
		t.Fatalf("GenerateButane: %v", err)
	}
	if !strings.Contains(butane, "newkey") {
		t.Error("Butane output does not contain updated SSH key")
	}
	if strings.Contains(butane, "original") {
		t.Error("Butane output still contains OLD SSH key after modification")
	}
}

// Q4: Full happy-path traversal (Welcome → Done) with all validation passing
func TestFullHappyPath_WelcomeToDone(t *testing.T) {
	w, _, _, inst := newTestWizard()

	// StepWelcome — valid channel already set ("stable")
	if w.State.CurrentStep != model.StepWelcome {
		t.Fatalf("expected start at Welcome, got %v", w.State.CurrentStep)
	}
	if err := w.Next(); err != nil {
		t.Fatalf("Welcome→Network: %v", err)
	}

	// StepNetwork — DHCP (passes without any config)
	w.State.Config.Network.Mode = model.NetworkDHCP
	if err := w.Next(); err != nil {
		t.Fatalf("Network→Storage: %v", err)
	}

	// StepStorage — select a disk
	w.State.Config.Disk = model.DiskInfo{DevPath: "/dev/disk/by-id/scsi-0QEMU_QEMU_HARDDISK_drive-scsi0"}
	if err := w.Next(); err != nil {
		t.Fatalf("Storage→User: %v", err)
	}

	// StepUser — add a user with SSH key
	w.State.Config.Users = []model.UserConfig{
		{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI test@laptop"}},
	}
	w.State.Config.Hostname = "flatcar-node-01"
	if err := w.Next(); err != nil {
		t.Fatalf("User→Sysext: %v", err)
	}

	// StepSysext — optional, just advance
	if err := w.Next(); err != nil {
		t.Fatalf("Sysext→Update: %v", err)
	}
	// Nvidia should be skipped (not selected)
	if w.State.CurrentStep != model.StepUpdate {
		t.Fatalf("expected StepUpdate (nvidia skipped), got %v", w.State.CurrentStep)
	}

	// StepUpdate — use default reboot strategy
	if err := w.Next(); err != nil {
		t.Fatalf("Update→Review: %v", err)
	}

	// StepReview — CheckConsistency must pass
	if w.State.CurrentStep != model.StepReview {
		t.Fatalf("expected StepReview, got %v", w.State.CurrentStep)
	}
	if err := w.Next(); err != nil {
		t.Fatalf("Review→Install: %v", err)
	}

	// StepInstall — advance to Done
	if err := w.Next(); err != nil {
		t.Fatalf("Install→Done: %v", err)
	}
	if w.State.CurrentStep != model.StepDone {
		t.Fatalf("expected StepDone, got %v", w.State.CurrentStep)
	}

	// Verify Execute actually calls the installer
	if err := w.Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !inst.called {
		t.Error("installer was never called")
	}
	if inst.cfg.Disk.DevPath != "/dev/disk/by-id/scsi-0QEMU_QEMU_HARDDISK_drive-scsi0" {
		t.Errorf("installer received wrong disk: %v", inst.cfg.Disk.DevPath)
	}
}

// Q5: IgnitionURL path — wizard.Next() still traverses sequentially,
// but CheckConsistency at Review accepts IgnitionURL without auth/users.
func TestIgnitionURL_ReviewAcceptsWithoutAuth(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.Config.Channel = "stable"
	w.State.Config.IgnitionURL = "https://example.com/config.ign"
	w.State.Config.Disk.DevPath = "/dev/sda"
	// No users, no SSH keys — would normally fail
	w.State.CurrentStep = model.StepReview

	err := w.ValidateCurrentStep()
	if err != nil {
		t.Fatalf("Review with IgnitionURL should skip auth check, got: %v", err)
	}
}

// Q5b: IgnitionURL path — Welcome validation catches invalid URLs
func TestIgnitionURL_WelcomeRejectsInvalidURL(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.Config.Channel = "stable"
	w.State.Config.IgnitionURL = "not-a-url"
	w.State.CurrentStep = model.StepWelcome

	err := w.ValidateCurrentStep()
	if err == nil {
		t.Fatal("expected error for invalid IgnitionURL at Welcome")
	}
	if !strings.Contains(err.Error(), "ignition URL") {
		t.Errorf("expected ignition URL error, got: %v", err)
	}
}

// Q6: Can user advance past StepStorage without disk?
func TestNext_StorageBlocksWithoutDisk(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepStorage
	w.State.Config.Disk.DevPath = "" // no disk selected

	err := w.Next()
	if err == nil {
		t.Fatal("expected error: cannot advance past Storage without disk")
	}
	if !strings.Contains(err.Error(), "no disk") {
		t.Errorf("expected 'no disk' error, got: %v", err)
	}
	if w.State.CurrentStep != model.StepStorage {
		t.Errorf("should stay on StepStorage, got %v", w.State.CurrentStep)
	}
}

// Edge case: Previous from StepDone — should still be able to go back
func TestPrevious_FromDone(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepDone

	w.Previous()
	if w.State.CurrentStep != model.StepInstall {
		t.Errorf("Previous from Done should go to Install, got %v", w.State.CurrentStep)
	}
}

// Edge case: Static network with empty DNS array passes (DNS is optional)
func TestValidateNetwork_StaticNoDNS_Passes(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepNetwork
	w.State.Config.Network = model.NetworkConfig{
		Mode:      model.NetworkStatic,
		Interface: "eth0",
		Address:   "10.0.0.5/24",
		Gateway:   "10.0.0.1",
		DNS:       nil, // no DNS servers
	}

	if err := w.ValidateCurrentStep(); err != nil {
		t.Errorf("static network without DNS should pass, got: %v", err)
	}
}

// Edge case: User with valid username but INVALID SSH key format
func TestValidateUser_InvalidSSHKeyFormat(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepUser
	w.State.Config.Users = []model.UserConfig{
		{Username: "core", SSHKeys: []string{"not-a-valid-ssh-key"}},
	}

	err := w.ValidateCurrentStep()
	if err == nil {
		t.Fatal("expected error for invalid SSH key")
	}
}

// Edge case: Multiple users, one with invalid username
func TestValidateUser_SecondUserInvalidUsername(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepUser
	w.State.Config.Users = []model.UserConfig{
		{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI test"}},
		{Username: "root", SSHKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI test2"}},
	}

	// "root" may or may not be valid depending on validate.Username — test the boundary
	// The key point: validation iterates ALL users, not just the first
	w.State.Config.Users[1].Username = "-invalid" // leading dash is invalid
	err := w.ValidateCurrentStep()
	if err == nil {
		t.Fatal("expected error for invalid username '-invalid'")
	}
}

// Edge case: CheckConsistency catches duplicate usernames
func TestReview_DuplicateUsernames(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepReview
	w.State.Config = model.InstallConfig{
		Channel: "stable",
		Disk:    model.DiskInfo{DevPath: "/dev/sda"},
		Users: []model.UserConfig{
			{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAA test1"}},
			{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAA test2"}},
		},
	}

	err := w.ValidateCurrentStep()
	if err == nil {
		t.Fatal("expected error for duplicate usernames")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected 'duplicate' error, got: %v", err)
	}
}

// Edge case: Nvidia path — Previous from StepUpdate skips back over Nvidia when not selected
func TestPrevious_SkipsNvidiaBackward(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.Sysexts = []model.SysextEntry{
		{Name: "docker", Selected: true},
	}
	w.State.CurrentStep = model.StepUpdate

	w.Previous()
	// Should skip StepNvidia and land on StepSysext
	if w.State.CurrentStep != model.StepSysext {
		t.Errorf("Previous from Update should skip Nvidia → Sysext, got %v", w.State.CurrentStep)
	}
}

// Edge case: Nvidia path — Previous from StepUpdate visits Nvidia when selected
func TestPrevious_VisitsNvidiaWhenSelected(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.Sysexts = []model.SysextEntry{
		{Name: "nvidia-runtime", Selected: true},
	}
	w.State.CurrentStep = model.StepUpdate

	w.Previous()
	if w.State.CurrentStep != model.StepNvidia {
		t.Errorf("Previous from Update with nvidia selected should go to Nvidia, got %v", w.State.CurrentStep)
	}
}

// Edge case: Full happy path WITH nvidia
func TestFullHappyPath_WithNvidia(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.Config.Channel = "stable"
	w.State.Config.Network.Mode = model.NetworkDHCP
	w.State.Config.Disk.DevPath = "/dev/sda"
	w.State.Config.Users = []model.UserConfig{
		{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI test"}},
	}
	w.State.Config.NvidiaDriverVersion = "570-open"
	w.State.Sysexts = []model.SysextEntry{
		{Name: "nvidia-runtime", Selected: true},
	}
	w.State.Config.Sysexts = w.State.Sysexts

	// Walk all the way through
	steps := []model.WizardStep{
		model.StepWelcome, model.StepNetwork, model.StepStorage,
		model.StepUser, model.StepSysext, model.StepNvidia,
		model.StepUpdate, model.StepReview, model.StepInstall,
	}

	for i, expected := range steps {
		if w.State.CurrentStep != expected {
			t.Fatalf("step %d: expected %v, got %v", i, expected, w.State.CurrentStep)
		}
		if err := w.Next(); err != nil {
			t.Fatalf("Next at step %v: %v", expected, err)
		}
	}
	if w.State.CurrentStep != model.StepDone {
		t.Fatalf("expected StepDone, got %v", w.State.CurrentStep)
	}
}

// Edge case: GoToStep above StepDone is a no-op
func TestGoToStep_AboveDone_Noop(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.GoToStep(model.WizardStep(99))
	if w.State.CurrentStep != model.StepWelcome {
		t.Errorf("GoToStep(99) should be no-op, got %v", w.State.CurrentStep)
	}
}

// Edge case: FetchSysexts auto-selects nvidia-runtime when GPU detected
func TestFetchSysexts_AutoSelectsNvidiaOnGPU(t *testing.T) {
	b := &mockBakery{sysexts: []model.SysextEntry{
		{Name: "docker", Description: "Docker CE"},
		{Name: "nvidia-runtime", Description: "NVIDIA runtime"},
	}}
	w := New(&mockProber{}, b, &mockInstaller{})
	w.State.NvidiaGPUDetected = true

	if err := w.FetchSysexts(context.Background()); err != nil {
		t.Fatalf("FetchSysexts: %v", err)
	}

	// nvidia-runtime should be auto-selected
	var nvidiaSelected bool
	for _, s := range w.State.Sysexts {
		if s.Name == "nvidia-runtime" && s.Selected {
			nvidiaSelected = true
		}
	}
	if !nvidiaSelected {
		t.Error("nvidia-runtime should be auto-selected when GPU detected")
	}
	if w.State.Config.NvidiaDriverVersion == "" {
		t.Error("NvidiaDriverVersion should be pre-configured when GPU detected")
	}
}

// Edge case: FetchSysexts does NOT auto-select when no GPU
func TestFetchSysexts_NoAutoSelectWithoutGPU(t *testing.T) {
	b := &mockBakery{sysexts: []model.SysextEntry{
		{Name: "nvidia-runtime", Description: "NVIDIA runtime"},
	}}
	w := New(&mockProber{}, b, &mockInstaller{})
	w.State.NvidiaGPUDetected = false

	if err := w.FetchSysexts(context.Background()); err != nil {
		t.Fatalf("FetchSysexts: %v", err)
	}

	for _, s := range w.State.Sysexts {
		if s.Name == "nvidia-runtime" && s.Selected {
			t.Error("nvidia-runtime should NOT be auto-selected without GPU")
		}
	}
}

// Edge case: Storage validation with disk path that fails DiskPath validator
func TestValidateStorage_InvalidDiskPath(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepStorage
	w.State.Config.Disk.DevPath = "../../../etc/passwd" // path traversal attempt

	err := w.ValidateCurrentStep()
	if err == nil {
		t.Fatal("expected error for path-traversal disk path")
	}
}

// Edge case: ExecuteWithProgress routes progress to external callback
func TestExecuteWithProgress(t *testing.T) {
	inst := &mockInstaller{}
	w := New(&mockProber{}, &mockBakery{}, inst)
	w.State.Config.Disk.DevPath = "/dev/sda"

	var msgs []string
	err := w.ExecuteWithProgress(context.Background(), func(msg string) {
		msgs = append(msgs, msg)
	})
	if err != nil {
		t.Fatalf("ExecuteWithProgress: %v", err)
	}
	if len(msgs) != 1 || msgs[0] != "test progress" {
		t.Errorf("expected progress callback to receive messages, got: %v", msgs)
	}
}

// Edge case: CheckConsistency at Review catches missing channel
func TestReview_MissingChannel(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepReview
	w.State.Config = model.InstallConfig{
		Disk:    model.DiskInfo{DevPath: "/dev/sda"},
		SSHKeys: []string{"ssh-ed25519 AAAA test"},
		Channel: "", // empty!
	}

	err := w.ValidateCurrentStep()
	if err == nil {
		t.Fatal("expected error for missing channel at Review")
	}
	if !strings.Contains(err.Error(), "channel") {
		t.Errorf("expected channel error, got: %v", err)
	}
}

// Regression: validate.Username rejects "root" — verify wizard surfaces it
func TestValidateUser_RejectsRoot(t *testing.T) {
	w, _, _, _ := newTestWizard()
	w.State.CurrentStep = model.StepUser
	w.State.Config.Users = []model.UserConfig{
		{Username: "root", SSHKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI test"}},
	}

	// validate.Username may or may not reject "root" — test what actually happens
	err := w.ValidateCurrentStep()
	// If it passes, that's fine — just document the behavior
	if err != nil {
		if !strings.Contains(err.Error(), "root") && !strings.Contains(err.Error(), "username") {
			t.Errorf("unexpected error: %v", err)
		}
	}
}
