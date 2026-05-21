package tui

import (
	"testing"

	"github.com/projectbluefin/knuckle/internal/model"
)

func TestApplyFields_Welcome_Channel(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	m.Wizard.State.CurrentStep = model.StepWelcome
	m.fields = []field{
		{key: "channel", value: "stable"},
		{key: "version", value: "3975.2.2"},
	}
	m.applyFields()
	if m.Wizard.State.Config.Channel != "stable" {
		t.Errorf("expected channel 'stable', got %q", m.Wizard.State.Config.Channel)
	}
	if m.Wizard.State.Config.Version != "3975.2.2" {
		t.Errorf("expected version '3975.2.2', got %q", m.Wizard.State.Config.Version)
	}
	if m.err != nil {
		t.Errorf("unexpected error: %v", m.err)
	}
}

func TestApplyFields_Welcome_InvalidChannel(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	m.Wizard.State.CurrentStep = model.StepWelcome
	m.fields = []field{
		{key: "channel", value: "bogus"},
	}
	m.applyFields()
	if m.err == nil {
		t.Error("expected error for invalid channel")
	}
}

func TestApplyFields_Welcome_IgnitionURL(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	m.Wizard.State.CurrentStep = model.StepWelcome
	m.fields = []field{
		{key: "ignition_url", value: "https://example.com/config.ign"},
	}
	m.applyFields()
	if m.Wizard.State.Config.IgnitionURL != "https://example.com/config.ign" {
		t.Errorf("expected IgnitionURL set, got %q", m.Wizard.State.Config.IgnitionURL)
	}
	if m.err != nil {
		t.Errorf("unexpected error: %v", m.err)
	}
}

func TestApplyFields_Welcome_InvalidIgnitionURL(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	m.Wizard.State.CurrentStep = model.StepWelcome
	m.fields = []field{
		{key: "ignition_url", value: "http://insecure.com/bad"},
	}
	m.applyFields()
	if m.err == nil {
		t.Error("expected error for http:// ignition URL")
	}
}

func TestApplyFields_Network_DHCP(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	m.Wizard.State.CurrentStep = model.StepNetwork
	m.fields = []field{
		{key: "interface", value: "eth0"},
		{key: "address", value: ""},
		{key: "gateway", value: ""},
		{key: "dns", value: ""},
	}
	m.applyFields()
	cfg := m.Wizard.State.Config
	if cfg.Network.Interface != "eth0" {
		t.Errorf("expected interface 'eth0', got %q", cfg.Network.Interface)
	}
	if cfg.Network.Mode != model.NetworkDHCP {
		t.Errorf("expected DHCP mode, got %v", cfg.Network.Mode)
	}
}

func TestApplyFields_Network_Static(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	m.Wizard.State.CurrentStep = model.StepNetwork
	m.fields = []field{
		{key: "interface", value: "eth0"},
		{key: "address", value: "10.0.0.5/24"},
		{key: "gateway", value: "10.0.0.1"},
		{key: "dns", value: "8.8.8.8,1.1.1.1"},
	}
	m.applyFields()
	cfg := m.Wizard.State.Config
	if cfg.Network.Mode != model.NetworkStatic {
		t.Errorf("expected Static mode, got %v", cfg.Network.Mode)
	}
	if cfg.Network.Address != "10.0.0.5/24" {
		t.Errorf("expected address '10.0.0.5/24', got %q", cfg.Network.Address)
	}
	if cfg.Network.Gateway != "10.0.0.1" {
		t.Errorf("expected gateway '10.0.0.1', got %q", cfg.Network.Gateway)
	}
	if len(cfg.Network.DNS) != 2 || cfg.Network.DNS[0] != "8.8.8.8" {
		t.Errorf("expected DNS [8.8.8.8, 1.1.1.1], got %v", cfg.Network.DNS)
	}
}

func TestApplyFields_User_CreatesUser(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	m.Wizard.State.CurrentStep = model.StepUser
	m.fields = []field{
		{key: "hostname", value: "myhost"},
		{key: "timezone", value: "America/New_York"},
		{key: "username", value: "admin"},
		{key: "password", value: ""},
		{key: "github_user", value: ""},
		{key: "ssh_key", value: "ssh-ed25519 AAAA test@key"},
	}
	m.applyFields()
	cfg := m.Wizard.State.Config
	if cfg.Hostname != "myhost" {
		t.Errorf("expected hostname 'myhost', got %q", cfg.Hostname)
	}
	if cfg.Timezone != "America/New_York" {
		t.Errorf("expected timezone 'America/New_York', got %q", cfg.Timezone)
	}
	if len(cfg.Users) == 0 {
		t.Fatal("expected user to be created")
	}
	if cfg.Users[0].Username != "admin" {
		t.Errorf("expected username 'admin', got %q", cfg.Users[0].Username)
	}
	if len(cfg.SSHKeys) != 1 || cfg.SSHKeys[0] != "ssh-ed25519 AAAA test@key" {
		t.Errorf("expected SSH key, got %v", cfg.SSHKeys)
	}
}

func TestApplyFields_User_EmptyTimezoneDefaultsUTC(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	m.Wizard.State.CurrentStep = model.StepUser
	m.fields = []field{
		{key: "hostname", value: "node1"},
		{key: "timezone", value: ""},
		{key: "username", value: "core"},
		{key: "password", value: ""},
		{key: "github_user", value: ""},
		{key: "ssh_key", value: ""},
	}
	m.applyFields()
	if m.Wizard.State.Config.Timezone != "UTC" {
		t.Errorf("expected UTC default, got %q", m.Wizard.State.Config.Timezone)
	}
}

func TestApplyFields_User_PasswordHash(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	m.Wizard.State.CurrentStep = model.StepUser
	m.Wizard.State.Config.Users = []model.UserConfig{{Username: "core"}}
	m.fields = []field{
		{key: "hostname", value: "node1"},
		{key: "timezone", value: "UTC"},
		{key: "username", value: "core"},
		{key: "password", value: "testpass"},
		{key: "github_user", value: ""},
		{key: "ssh_key", value: ""},
	}
	m.applyFields()
	if m.err != nil {
		t.Fatalf("unexpected error: %v", m.err)
	}
	if m.Wizard.State.Config.Users[0].PasswordHash == "" {
		t.Error("expected password hash to be set")
	}
}

func TestApplyFields_User_UpdatesExistingUser(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	m.Wizard.State.CurrentStep = model.StepUser
	m.Wizard.State.Config.Users = []model.UserConfig{{Username: "old", Groups: []string{"sudo"}}}
	m.fields = []field{
		{key: "hostname", value: "node1"},
		{key: "timezone", value: "UTC"},
		{key: "username", value: "newuser"},
		{key: "password", value: ""},
		{key: "github_user", value: ""},
		{key: "ssh_key", value: ""},
	}
	m.applyFields()
	if m.Wizard.State.Config.Users[0].Username != "newuser" {
		t.Errorf("expected username updated to 'newuser', got %q", m.Wizard.State.Config.Users[0].Username)
	}
}

func TestApplyFields_Review_Confirmed(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	m.Wizard.State.CurrentStep = model.StepReview
	m.fields = []field{
		{key: "confirm", value: "YES"},
	}
	m.applyFields()
	if !m.Wizard.State.Confirmed {
		t.Error("expected Confirmed=true after YES")
	}
}

func TestApplyFields_Review_NotConfirmed(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	m.Wizard.State.CurrentStep = model.StepReview
	m.fields = []field{
		{key: "confirm", value: "no"},
	}
	m.applyFields()
	if m.Wizard.State.Confirmed {
		t.Error("expected Confirmed=false after 'no'")
	}
}

func TestApplyFields_Review_CaseInsensitive(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	m.Wizard.State.CurrentStep = model.StepReview
	m.fields = []field{
		{key: "confirm", value: " yes "},
	}
	m.applyFields()
	if !m.Wizard.State.Confirmed {
		t.Error("expected Confirmed=true with ' yes ' (trimmed, uppercased)")
	}
}
