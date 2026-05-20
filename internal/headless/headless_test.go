package headless

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"log/slog"

	"github.com/castrojo/knuckle/internal/bakery"
	"github.com/castrojo/knuckle/internal/model"
)

func TestLoadConfig(t *testing.T) {
	cfg := Config{
		Channel:        "stable",
		Hostname:       "test-node",
		Timezone:       "UTC",
		Network:        NetworkConfig{Mode: "dhcp"},
		Users:          []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz test@test"}}},
		Disk:           "/dev/vdb",
		UpdateStrategy: "reboot",
		Reboot:         true,
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.Channel != "stable" {
		t.Errorf("got channel=%q, want stable", loaded.Channel)
	}
	if loaded.Hostname != "test-node" {
		t.Errorf("got hostname=%q, want test-node", loaded.Hostname)
	}
	if loaded.Disk != "/dev/vdb" {
		t.Errorf("got disk=%q, want /dev/vdb", loaded.Disk)
	}
	if len(loaded.Users) != 1 {
		t.Fatalf("got %d users, want 1", len(loaded.Users))
	}
	if loaded.Users[0].Username != "core" {
		t.Errorf("got username=%q, want core", loaded.Users[0].Username)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json {{{"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestValidate_ValidDHCP(t *testing.T) {
	cfg := &Config{
		Channel:        "stable",
		Hostname:       "node01",
		Network:        NetworkConfig{Mode: "dhcp"},
		Users:          []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz test@test"}}},
		Disk:           "/dev/vdb",
		UpdateStrategy: "reboot",
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_ValidStatic(t *testing.T) {
	cfg := &Config{
		Channel:  "beta",
		Hostname: "node02",
		Network: NetworkConfig{
			Mode:      "static",
			Interface: "eth0",
			Address:   "192.168.1.100/24",
			Gateway:   "192.168.1.1",
			DNS:       []string{"1.1.1.1"},
		},
		Users:          []UserConfig{{Username: "admin", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz test@test"}}},
		Disk:           "/dev/sda",
		UpdateStrategy: "off",
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_MissingDisk(t *testing.T) {
	cfg := &Config{
		Channel:  "stable",
		Hostname: "node01",
		Network:  NetworkConfig{Mode: "dhcp"},
		Users:    []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz test@test"}}},
		Disk:     "",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing disk")
	}
}

func TestValidate_InvalidDiskPath(t *testing.T) {
	cases := []struct {
		disk string
		desc string
	}{
		{"../../etc/passwd", "path traversal"},
		{"sda", "no /dev/ prefix"},
		{"/etc/passwd", "not a /dev/ path"},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := &Config{
				Channel:  "stable",
				Hostname: "node01",
				Network:  NetworkConfig{Mode: "dhcp"},
				Users:    []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz test@test"}}},
				Disk:     tc.disk,
			}
			if err := cfg.Validate(); err == nil {
				t.Errorf("expected error for disk=%q (%s)", tc.disk, tc.desc)
			}
		})
	}
}

func TestValidate_MissingUsers(t *testing.T) {
	cfg := &Config{
		Channel:  "stable",
		Hostname: "node01",
		Network:  NetworkConfig{Mode: "dhcp"},
		Users:    nil,
		Disk:     "/dev/vdb",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing users")
	}
}

func TestValidate_UserNoAuth(t *testing.T) {
	cfg := &Config{
		Channel:  "stable",
		Hostname: "node01",
		Network:  NetworkConfig{Mode: "dhcp"},
		Users:    []UserConfig{{Username: "core"}}, // no keys, no password, no github
		Disk:     "/dev/vdb",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for user without auth method")
	}
}

func TestValidate_InvalidChannel(t *testing.T) {
	cfg := &Config{
		Channel:  "nightly",
		Hostname: "node01",
		Network:  NetworkConfig{Mode: "dhcp"},
		Users:    []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz test@test"}}},
		Disk:     "/dev/vdb",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid channel")
	}
}

func TestValidate_InvalidHostname(t *testing.T) {
	cfg := &Config{
		Channel:  "stable",
		Hostname: "INVALID HOST NAME!",
		Network:  NetworkConfig{Mode: "dhcp"},
		Users:    []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz test@test"}}},
		Disk:     "/dev/vdb",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid hostname")
	}
}

func TestValidate_InvalidStaticNetwork(t *testing.T) {
	cfg := &Config{
		Channel:  "stable",
		Hostname: "node01",
		Network: NetworkConfig{
			Mode:    "static",
			Address: "not-a-cidr",
		},
		Users: []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz test@test"}}},
		Disk:  "/dev/vdb",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
}

func TestValidate_StaticNetworkMissingInterface(t *testing.T) {
	cfg := &Config{
		Channel:  "stable",
		Hostname: "node02",
		Network: NetworkConfig{
			Mode:    "static",
			Address: "192.168.1.100/24",
			Gateway: "192.168.1.1",
			// Interface intentionally omitted
		},
		Users: []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz test@test"}}},
		Disk:  "/dev/sda",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for static network with empty interface")
	}
}

func TestValidate_InvalidNetworkMode(t *testing.T) {
	cfg := &Config{
		Channel:  "stable",
		Hostname: "node01",
		Network:  NetworkConfig{Mode: "bonded"},
		Users:    []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz test@test"}}},
		Disk:     "/dev/vdb",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for unrecognised network mode")
	}
}

func TestValidate_InvalidIgnitionURL(t *testing.T) {
	cases := []struct {
		desc string
		url  string
	}{
		{"no scheme", "example.com/config.ign"},
		{"ftp scheme", "ftp://example.com/config.ign"},
		{"bare text", "not-a-url"},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := &Config{
				Channel:     "stable",
				Disk:        "/dev/vdb",
				IgnitionURL: tc.url,
			}
			if err := cfg.Validate(); err == nil {
				t.Errorf("expected error for ignition_url=%q (%s)", tc.url, tc.desc)
			}
		})
	}
}

func TestValidate_DuplicateUsername(t *testing.T) {
	cfg := &Config{
		Channel:  "stable",
		Hostname: "node01",
		Network:  NetworkConfig{Mode: "dhcp"},
		Users: []UserConfig{
			{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz test@test"}},
			{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz other@host"}},
		},
		Disk: "/dev/vdb",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for duplicate username")
	}
}

func TestValidate_InvalidUpdateStrategy(t *testing.T) {
	cfg := &Config{
		Channel:        "stable",
		Hostname:       "node01",
		Network:        NetworkConfig{Mode: "dhcp"},
		Users:          []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz test@test"}}},
		Disk:           "/dev/vdb",
		UpdateStrategy: "invalid-strategy",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid update strategy")
	}
}

func TestToInstallConfig(t *testing.T) {
	cfg := &Config{
		Channel:        "beta",
		Hostname:       "test-host",
		Timezone:       "America/New_York",
		Network:        NetworkConfig{Mode: "dhcp"},
		Users:          []UserConfig{{Username: "admin", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz k"}, Groups: []string{"wheel"}}},
		Disk:           "/dev/nvme0n1",
		UpdateStrategy: "off",
	}

	installCfg, err := cfg.ToInstallConfig()
	if err != nil {
		t.Fatalf("ToInstallConfig: %v", err)
	}

	if installCfg.Channel != "beta" {
		t.Errorf("channel: got %q, want beta", installCfg.Channel)
	}
	if installCfg.Hostname != "test-host" {
		t.Errorf("hostname: got %q, want test-host", installCfg.Hostname)
	}
	if installCfg.Disk.DevPath != "/dev/nvme0n1" {
		t.Errorf("disk: got %q, want /dev/nvme0n1", installCfg.Disk.DevPath)
	}
	if installCfg.Network.Mode != model.NetworkDHCP {
		t.Errorf("network mode: got %v, want DHCP", installCfg.Network.Mode)
	}
	if len(installCfg.Users) != 1 {
		t.Fatalf("users: got %d, want 1", len(installCfg.Users))
	}
	if installCfg.Users[0].Groups[0] != "wheel" {
		t.Errorf("groups: got %v, want [wheel]", installCfg.Users[0].Groups)
	}
	if installCfg.UpdateStrategy.RebootStrategy != "off" {
		t.Errorf("update strategy: got %q, want off", installCfg.UpdateStrategy.RebootStrategy)
	}
}

func TestToInstallConfig_Defaults(t *testing.T) {
	cfg := &Config{
		Users: []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz k"}}},
		Disk:  "/dev/vdb",
	}

	installCfg, err := cfg.ToInstallConfig()
	if err != nil {
		t.Fatalf("ToInstallConfig: %v", err)
	}

	if installCfg.Channel != "stable" {
		t.Errorf("default channel: got %q, want stable", installCfg.Channel)
	}
	if installCfg.Timezone != "UTC" {
		t.Errorf("default timezone: got %q, want UTC", installCfg.Timezone)
	}
	if installCfg.UpdateStrategy.RebootStrategy != "reboot" {
		t.Errorf("default strategy: got %q, want reboot", installCfg.UpdateStrategy.RebootStrategy)
	}
}

// mockInstaller for testing Run()
type mockInstaller struct {
	installErr error
	called     bool
	lastCfg    *model.InstallConfig
}

func (m *mockInstaller) Install(ctx context.Context, cfg *model.InstallConfig, progress func(string)) error {
	m.called = true
	m.lastCfg = cfg
	progress("mock step 1")
	progress("mock step 2")
	return m.installErr
}

func TestRun_Success(t *testing.T) {
	cfg := &Config{
		Channel:        "stable",
		Hostname:       "test-node",
		Network:        NetworkConfig{Mode: "dhcp"},
		Users:          []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz test@test"}}},
		Disk:           "/dev/vdb",
		UpdateStrategy: "reboot",
		DryRun:         true,
	}

	installer := &mockInstaller{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	err := Run(context.Background(), cfg, installer, logger)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !installer.called {
		t.Error("installer.Install was not called")
	}
}

func TestRun_ValidationFailure(t *testing.T) {
	cfg := &Config{
		Channel: "invalid-channel",
		Disk:    "/dev/vdb",
		Users:   []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz k"}}},
	}

	installer := &mockInstaller{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	err := Run(context.Background(), cfg, installer, logger)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if installer.called {
		t.Error("installer should not be called on validation failure")
	}
}

func TestToInstallConfig_StaticNetwork(t *testing.T) {
	cfg := &Config{
		Channel:  "stable",
		Hostname: "static-node",
		Timezone: "UTC",
		Network: NetworkConfig{
			Mode:      "static",
			Interface: "eth0",
			Address:   "192.168.1.50/24",
			Gateway:   "192.168.1.1",
		},
		Users:          []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz k"}}},
		Disk:           "/dev/vdb",
		UpdateStrategy: "off",
	}

	installCfg, err := cfg.ToInstallConfig()
	if err != nil {
		t.Fatalf("ToInstallConfig: %v", err)
	}

	if installCfg.Network.Mode != model.NetworkStatic {
		t.Errorf("mode: got %v, want Static", installCfg.Network.Mode)
	}
	if installCfg.Network.Interface != "eth0" {
		t.Errorf("interface: got %q, want eth0", installCfg.Network.Interface)
	}
	if installCfg.Network.Address != "192.168.1.50/24" {
		t.Errorf("address: got %q, want 192.168.1.50/24", installCfg.Network.Address)
	}
	if installCfg.Network.Gateway != "192.168.1.1" {
		t.Errorf("gateway: got %q, want 192.168.1.1", installCfg.Network.Gateway)
	}
}

func TestRun_GitHubUser(t *testing.T) {
	const fakeKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGitHubKey github-test-key"

	old := fetchGitHubKeysFunc
	fetchGitHubKeysFunc = func(_ context.Context, username string) ([]string, error) {
		if username != "testuser" {
			return nil, fmt.Errorf("unexpected username %q", username)
		}
		return []string{fakeKey}, nil
	}
	defer func() { fetchGitHubKeysFunc = old }()

	cfg := &Config{
		Channel:  "stable",
		Hostname: "gh-node",
		Network:  NetworkConfig{Mode: "dhcp"},
		Users: []UserConfig{{
			Username:   "core",
			GithubUser: "testuser",
		}},
		Disk:   "/dev/vdb",
		DryRun: true,
	}

	installer := &mockInstaller{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	err := Run(context.Background(), cfg, installer, logger)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !installer.called {
		t.Error("installer.Install was not called")
	}
	if len(cfg.Users[0].SSHKeys) == 0 || cfg.Users[0].SSHKeys[0] != fakeKey {
		t.Errorf("SSH keys not populated from GitHub: %v", cfg.Users[0].SSHKeys)
	}
}

func mockBakery(entries []model.SysextEntry, err error) func() {
	old := newBakeryClientFunc
	newBakeryClientFunc = func() bakery.Client {
		return &bakery.MockClient{Entries: entries, Err: err}
	}
	return func() { newBakeryClientFunc = old }
}

func TestResolveSysexts_Success(t *testing.T) {
	catalog := []model.SysextEntry{
		{Name: "docker", Version: "24.0.7", URL: "https://example.com/docker-24.0.7.raw"},
		{Name: "tailscale", Version: "1.56.1", URL: "https://example.com/tailscale-1.56.1.raw"},
	}
	defer mockBakery(catalog, nil)()

	cfg := &Config{
		Channel:  "stable",
		Hostname: "sysext-node",
		Network:  NetworkConfig{Mode: "dhcp"},
		Users:    []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz k"}}},
		Disk:     "/dev/vdb",
		Sysexts:  []string{"docker", "tailscale"},
		DryRun:   true,
	}

	installer := &mockInstaller{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	if err := Run(context.Background(), cfg, installer, logger); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !installer.called {
		t.Error("installer.Install was not called")
	}
	// Verify sysexts were passed to install with Selected=true
	installCfg := installer.lastCfg
	if installCfg == nil {
		t.Fatal("no install config captured")
	}
	if len(installCfg.Sysexts) != 2 {
		t.Fatalf("expected 2 sysexts, got %d", len(installCfg.Sysexts))
	}
	if installCfg.Sysexts[0].Name != "docker" || !installCfg.Sysexts[0].Selected {
		t.Errorf("docker sysext wrong: %+v", installCfg.Sysexts[0])
	}
	if installCfg.Sysexts[1].Name != "tailscale" || !installCfg.Sysexts[1].Selected {
		t.Errorf("tailscale sysext wrong: %+v", installCfg.Sysexts[1])
	}
}

func TestResolveSysexts_NotFound(t *testing.T) {
	catalog := []model.SysextEntry{
		{Name: "docker", URL: "https://example.com/docker.raw"},
	}
	defer mockBakery(catalog, nil)()

	cfg := &Config{
		Channel: "stable",
		Disk:    "/dev/vdb",
		Network: NetworkConfig{Mode: "dhcp"},
		Users:   []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz k"}}},
		Sysexts: []string{"nonexistent-sysext"},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	err := Run(context.Background(), cfg, &mockInstaller{}, logger)
	if err == nil {
		t.Fatal("expected error for unknown sysext name")
	}
	if !strings.Contains(err.Error(), "nonexistent-sysext") {
		t.Errorf("error should name the missing sysext, got: %v", err)
	}
}

func TestResolveSysexts_CatalogError(t *testing.T) {
	defer mockBakery(nil, fmt.Errorf("network timeout"))()

	cfg := &Config{
		Channel: "stable",
		Disk:    "/dev/vdb",
		Network: NetworkConfig{Mode: "dhcp"},
		Users:   []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz k"}}},
		Sysexts: []string{"docker"},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	err := Run(context.Background(), cfg, &mockInstaller{}, logger)
	if err == nil {
		t.Fatal("expected error when bakery is unavailable")
	}
}

func TestResolveSysexts_Empty(t *testing.T) {
	// No bakery call should happen when sysexts list is empty
	called := false
	old := newBakeryClientFunc
	newBakeryClientFunc = func() bakery.Client {
		called = true
		return &bakery.MockClient{}
	}
	defer func() { newBakeryClientFunc = old }()

	cfg := &Config{
		Channel: "stable",
		Disk:    "/dev/vdb",
		Network: NetworkConfig{Mode: "dhcp"},
		Users:   []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz k"}}},
		Sysexts: []string{}, // explicitly empty
		DryRun:  true,
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	if err := Run(context.Background(), cfg, &mockInstaller{}, logger); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if called {
		t.Error("bakery client should not be called when sysexts list is empty")
	}
}

func TestValidate_InvalidArch(t *testing.T) {
	cfg := Config{
		Arch:           "riscv64",
		Channel:        "stable",
		Hostname:       "node",
		Network:        NetworkConfig{Mode: "dhcp"},
		Users:          []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAA test"}}},
		Disk:           "/dev/vda",
		UpdateStrategy: "reboot",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid arch")
	}
	if !strings.Contains(err.Error(), "arch") {
		t.Errorf("error should mention arch, got: %v", err)
	}
}

func TestValidate_Arm64LTSRejected(t *testing.T) {
	cfg := Config{
		Arch:           "arm64",
		Channel:        "lts",
		Hostname:       "node",
		Network:        NetworkConfig{Mode: "dhcp"},
		Users:          []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAA test"}}},
		Disk:           "/dev/vda",
		UpdateStrategy: "reboot",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for arm64+lts")
	}
	if !strings.Contains(err.Error(), "LTS") {
		t.Errorf("error should mention LTS, got: %v", err)
	}
}

func TestToInstallConfig_DefaultArch(t *testing.T) {
	cfg := Config{
		// Arch omitted — should default to amd64
		Channel:        "stable",
		Hostname:       "node",
		Network:        NetworkConfig{Mode: "dhcp"},
		Users:          []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAA test"}}},
		Disk:           "/dev/vda",
		UpdateStrategy: "reboot",
	}
	ic, err := cfg.ToInstallConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ic.Arch != "amd64" {
		t.Errorf("default arch: got %q, want \"amd64\"", ic.Arch)
	}
}

func TestLoadConfig_NvidiaDriverVersion(t *testing.T) {
	cfg := Config{
		Channel:             "stable",
		Hostname:            "gpu-node",
		Network:             NetworkConfig{Mode: "dhcp"},
		Users:               []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz test@test"}}},
		Disk:                "/dev/vdb",
		NvidiaDriverVersion: "570-open",
		UpdateStrategy:      "reboot",
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "nvidia.json")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.NvidiaDriverVersion != "570-open" {
		t.Errorf("nvidia_driver_version: got %q, want 570-open", loaded.NvidiaDriverVersion)
	}
}

func TestValidate_InvalidNvidiaDriver(t *testing.T) {
	cfg := &Config{
		Channel:             "stable",
		Hostname:            "test",
		Network:             NetworkConfig{Mode: "dhcp"},
		Users:               []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAAC3Nz test@test"}}},
		Disk:                "/dev/vdb",
		NvidiaDriverVersion: "bogus-driver",
		UpdateStrategy:      "reboot",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid nvidia_driver_version")
	}
	if !strings.Contains(err.Error(), "nvidia_driver_version") {
		t.Errorf("error should mention nvidia_driver_version, got: %v", err)
	}
}

func TestToInstallConfig_Arm64(t *testing.T) {
	cfg := Config{
		Arch:           "arm64",
		Channel:        "stable",
		Hostname:       "node",
		Network:        NetworkConfig{Mode: "dhcp"},
		Users:          []UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAA test"}}},
		Disk:           "/dev/vda",
		UpdateStrategy: "reboot",
	}
	ic, err := cfg.ToInstallConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ic.Arch != "arm64" {
		t.Errorf("got arch %q, want \"arm64\"", ic.Arch)
	}
}
