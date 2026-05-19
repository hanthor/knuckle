package install

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/castrojo/knuckle/internal/ignition"
	"github.com/castrojo/knuckle/internal/model"
	"github.com/castrojo/knuckle/internal/runner"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestInstallWithGeneratedConfig(t *testing.T) {
	spy := runner.NewSpyRunner()

	installer := NewFlatcarInstaller(spy, testLogger())
	cfg := &model.InstallConfig{
		Channel:  "stable",
		Hostname: "test-node",
		Disk: model.DiskInfo{
			DevPath: "/dev/sda",
		},
		Network: model.NetworkConfig{Mode: model.NetworkDHCP},
		Users: []model.UserConfig{
			{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAA test"}},
		},
	}

	err := installer.Install(context.Background(), cfg, func(string) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify butane compilation happened in-process (no CLI call)
	// The Go library handles it directly — verify no butane CLI call was made
	for i := range spy.Calls {
		if spy.Calls[i].Name == "butane" {
			t.Error("butane CLI should not be called — using Go library")
		}
	}

	// Verify ignition file was written securely (umask 077, not world-readable tee)
	var writeCall *runner.SpyCall
	for i := range spy.Calls {
		if spy.Calls[i].Name == "sh" {
			writeCall = &spy.Calls[i]
			break
		}
	}
	if writeCall == nil {
		t.Fatal("secure write (sh -c umask) was not called")
	}
	wantWriteArgs := []string{"-c", "umask 077 && cat > /tmp/knuckle-ignition.json"}
	if len(writeCall.Args) != len(wantWriteArgs) {
		t.Fatalf("write args = %v, want %v", writeCall.Args, wantWriteArgs)
	}
	for i, arg := range wantWriteArgs {
		if writeCall.Args[i] != arg {
			t.Errorf("write arg[%d] = %q, want %q", i, writeCall.Args[i], arg)
		}
	}
	if writeCall.Input == "" {
		t.Error("write call had no stdin input (ignition JSON)")
	}

	// Verify flatcar-install was called with correct args
	var installCall *runner.SpyCall
	for i := range spy.Calls {
		if spy.Calls[i].Name == "flatcar-install" {
			installCall = &spy.Calls[i]
			break
		}
	}
	if installCall == nil {
		t.Fatal("flatcar-install was not called")
	}
	wantArgs := []string{"-d", "/dev/sda", "-C", "stable", "-i", "/tmp/knuckle-ignition.json"}
	if len(installCall.Args) != len(wantArgs) {
		t.Fatalf("flatcar-install args = %v, want %v", installCall.Args, wantArgs)
	}
	for i, arg := range wantArgs {
		if installCall.Args[i] != arg {
			t.Errorf("flatcar-install arg[%d] = %q, want %q", i, installCall.Args[i], arg)
		}
	}
}

func TestInstallWithExternalURL(t *testing.T) {
	spy := runner.NewSpyRunner()
	installer := NewFlatcarInstaller(spy, testLogger())
	cfg := &model.InstallConfig{
		Channel:     "beta",
		Hostname:    "ext-node",
		Disk:        model.DiskInfo{DevPath: "/dev/vda"},
		IgnitionURL: "https://example.com/config.ign",
	}

	err := installer.Install(context.Background(), cfg, func(string) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// butane must NOT be called
	for _, call := range spy.Calls {
		if call.Name == "butane" {
			t.Error("butane should not be called when IgnitionURL is set")
		}
	}

	// flatcar-install must use -I flag
	var installCall *runner.SpyCall
	for i := range spy.Calls {
		if spy.Calls[i].Name == "flatcar-install" {
			installCall = &spy.Calls[i]
			break
		}
	}
	if installCall == nil {
		t.Fatal("flatcar-install was not called")
	}
	wantArgs := []string{"-d", "/dev/vda", "-C", "beta", "-I", "https://example.com/config.ign"}
	if len(installCall.Args) != len(wantArgs) {
		t.Fatalf("flatcar-install args = %v, want %v", installCall.Args, wantArgs)
	}
	for i, arg := range wantArgs {
		if installCall.Args[i] != arg {
			t.Errorf("flatcar-install arg[%d] = %q, want %q", i, installCall.Args[i], arg)
		}
	}
}

func TestInstallNilConfig(t *testing.T) {
	spy := runner.NewSpyRunner()
	installer := NewFlatcarInstaller(spy, testLogger())

	err := installer.Install(context.Background(), nil, func(string) {})
	if err == nil {
		t.Fatal("expected error for nil config")
	}
	if got := err.Error(); got != "install config cannot be nil" {
		t.Errorf("error = %q, want %q", got, "install config cannot be nil")
	}
}

func TestInstallButaneCompilationFailure(t *testing.T) {
	// Test that CompileToIgnition properly rejects invalid Butane YAML.
	// Since the install path now uses the Go library directly, we test
	// the compilation function with intentionally malformed input.
	_, err := ignition.CompileToIgnition("not: valid: butane: {{{")
	if err == nil {
		t.Fatal("expected error for invalid Butane YAML")
	}
	if got := err.Error(); got == "" {
		t.Error("error message should not be empty")
	}
}

func TestInstallFlatcarInstallFailure(t *testing.T) {
	spy := runner.NewSpyRunner()
	spy.StubResponse("flatcar-install -d /dev/sda -C stable -i /tmp/knuckle-ignition.json", &runner.Result{
		Command:  "flatcar-install",
		Args:     []string{"-d", "/dev/sda", "-C", "stable", "-i", "/tmp/knuckle-ignition.json"},
		Stderr:   "error: disk not found",
		ExitCode: 1,
	})
	spy.StubError("flatcar-install -d /dev/sda -C stable -i /tmp/knuckle-ignition.json",
		fmt.Errorf("command exited with code 1"))

	installer := NewFlatcarInstaller(spy, testLogger())
	cfg := &model.InstallConfig{
		Channel:  "stable",
		Hostname: "fail-node",
		Disk:     model.DiskInfo{DevPath: "/dev/sda"},
		Network:  model.NetworkConfig{Mode: model.NetworkDHCP},
		Users:    []model.UserConfig{{Username: "core"}},
	}

	err := installer.Install(context.Background(), cfg, func(string) {})
	if err == nil {
		t.Fatal("expected error when flatcar-install fails")
	}
	if got := err.Error(); got != "flatcar-install: command exited with code 1" {
		t.Errorf("error = %q", got)
	}
}

func TestBuildInstallArgs(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *model.InstallConfig
		ignitionJSON string
		want         []string
	}{
		{
			name: "basic with ignition file",
			cfg: &model.InstallConfig{
				Channel: "stable",
				Disk:    model.DiskInfo{DevPath: "/dev/sda"},
			},
			ignitionJSON: `{"ignition":{}}`,
			want:         []string{"-d", "/dev/sda", "-C", "stable", "-i", "/tmp/knuckle-ignition.json"},
		},
		{
			name: "external URL",
			cfg: &model.InstallConfig{
				Channel:     "alpha",
				Disk:        model.DiskInfo{DevPath: "/dev/nvme0n1"},
				IgnitionURL: "https://example.com/ign.json",
			},
			ignitionJSON: "",
			want:         []string{"-d", "/dev/nvme0n1", "-C", "alpha", "-I", "https://example.com/ign.json"},
		},
		{
			name: "no ignition",
			cfg: &model.InstallConfig{
				Channel: "beta",
				Disk:    model.DiskInfo{DevPath: "/dev/vda"},
			},
			ignitionJSON: "",
			want:         []string{"-d", "/dev/vda", "-C", "beta"},
		},
		{
			name: "version pinning",
			cfg: &model.InstallConfig{
				Channel: "stable",
				Disk:    model.DiskInfo{DevPath: "/dev/sda"},
				Version: "3510.2.8",
			},
			ignitionJSON: `{"ignition":{}}`,
			want:         []string{"-d", "/dev/sda", "-C", "stable", "-V", "3510.2.8", "-i", "/tmp/knuckle-ignition.json"},
		},
		{
			name: "prefers by-id path over devpath",
			cfg: &model.InstallConfig{
				Channel: "stable",
				Disk: model.DiskInfo{
					DevPath: "/dev/sda",
					Path:    "/dev/disk/by-id/ata-Samsung_SSD_870_S5PXNG0R312345",
				},
			},
			ignitionJSON: `{"ignition":{}}`,
			want:         []string{"-d", "/dev/disk/by-id/ata-Samsung_SSD_870_S5PXNG0R312345", "-C", "stable", "-i", "/tmp/knuckle-ignition.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildInstallArgs(tt.cfg, tt.ignitionJSON)
			if len(got) != len(tt.want) {
				t.Fatalf("buildInstallArgs() = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("arg[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestProgressCallback(t *testing.T) {
	spy := runner.NewSpyRunner()

	installer := NewFlatcarInstaller(spy, testLogger())
	cfg := &model.InstallConfig{
		Channel:  "stable",
		Hostname: "progress-node",
		Disk:     model.DiskInfo{DevPath: "/dev/sda"},
		Network:  model.NetworkConfig{Mode: model.NetworkDHCP},
		Users:    []model.UserConfig{{Username: "core"}},
	}

	var steps []string
	err := installer.Install(context.Background(), cfg, func(step string) {
		steps = append(steps, step)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedSteps := []string{
		"Generating Butane config...",
		"Compiling Ignition config...",
		"Writing Ignition config...",
		"Running flatcar-install...",
		"Installation complete!",
	}

	if len(steps) != len(expectedSteps) {
		t.Fatalf("got %d progress steps, want %d\nsteps: %v", len(steps), len(expectedSteps), steps)
	}
	for i, want := range expectedSteps {
		if steps[i] != want {
			t.Errorf("step[%d] = %q, want %q", i, steps[i], want)
		}
	}
}
