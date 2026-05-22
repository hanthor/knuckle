package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestMain re-uses the compiled test binary as the knuckle subprocess.
// When KNUCKLE_TEST_MAIN=1, the process delegates directly to main() —
// so early-exit flag-validation paths can be tested without starting the TUI.
func TestMain(m *testing.M) {
	if os.Getenv("KNUCKLE_TEST_MAIN") == "1" {
		main()
		os.Exit(0) // reached only if main() returned without calling os.Exit
	}
	os.Exit(m.Run())
}

// helperCmd builds a subprocess that runs main() with the supplied args.
// A 10-second timeout is applied so a future blocking-before-exit regression
// fails fast rather than hanging CI indefinitely.
func helperCmd(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], args...)
	cmd.Env = append(os.Environ(), "KNUCKLE_TEST_MAIN=1")
	return cmd
}

// TestMain_Version verifies --version prints "knuckle <ver>" and exits 0.
func TestMain_Version(t *testing.T) {
	cmd := helperCmd(t, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("--version exited non-zero: %v\noutput: %s", err, out)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(out)), "knuckle ") {
		t.Errorf("--version output %q does not match 'knuckle <version>'", out)
	}
}

// TestMain_InvalidChannel verifies an unrecognised channel exits 1 with a
// descriptive error on stderr.
func TestMain_InvalidChannel(t *testing.T) {
	cmd := helperCmd(t, "--channel=bogus", "--log-file=/dev/null")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for invalid channel, got exit 0")
	}
	if cmd.ProcessState.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got %d", cmd.ProcessState.ExitCode())
	}
	if !strings.Contains(string(out), "bogus") {
		t.Errorf("expected output to contain the bad channel name %q; got: %s", "bogus", out)
	}
}

// TestMain_HeadlessRequiresConfig verifies that --headless without --config
// exits 1 and prints the usage hint.
func TestMain_HeadlessRequiresConfig(t *testing.T) {
	cmd := helperCmd(t, "--headless")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for --headless without --config, got exit 0")
	}
	if cmd.ProcessState.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got %d", cmd.ProcessState.ExitCode())
	}
	if !strings.Contains(string(out), "--config") {
		t.Errorf("expected output to mention '--config'; got: %s", out)
	}
}

// TestMain_HeadlessConfigNotFound verifies that --headless with a
// non-existent config file exits 1 with an error message.
func TestMain_HeadlessConfigNotFound(t *testing.T) {
	cmd := helperCmd(t, "--headless",
		"--config=/nonexistent-knuckle-config-xyz.json",
		"--log-file=/dev/null")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for missing config file, got exit 0")
	}
	if cmd.ProcessState.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got %d", cmd.ProcessState.ExitCode())
	}
	if !strings.Contains(string(out), "Error") {
		t.Errorf("expected output to contain 'Error'; got: %s", out)
	}
}

// TestMain_ConfigWithoutHeadless verifies that --config alone (no --headless)
// also triggers the headless path and exits 1 when the file is missing.
// This covers the `configFile != ""` branch of the headless guard.
func TestMain_ConfigWithoutHeadless(t *testing.T) {
	cmd := helperCmd(t, "--config=/nonexistent-knuckle-config-xyz.json",
		"--log-file=/dev/null")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for missing config file, got exit 0")
	}
	if cmd.ProcessState.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got %d", cmd.ProcessState.ExitCode())
	}
	if !strings.Contains(string(out), "Error") {
		t.Errorf("expected output to contain 'Error'; got: %s", out)
	}
}
