package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrojo/knuckle/internal/model"
)

// --- fetchKeysMsg: error path ---

// TestFetchKeysError_ShowsRetryHint verifies that when the async GitHub key
// fetch returns an error, the model surfaces a human-readable message that
// includes the retry instruction and stays on StepUser.
func TestFetchKeysError_ShowsRetryHint(t *testing.T) {
	w := newTestWizard()
	w.State.CurrentStep = model.StepUser
	w.State.Config.Users = []model.UserConfig{{Username: "core"}}

	m := New(w)
	newModel, _ := m.Update(fetchKeysMsg{keys: nil, err: fmt.Errorf("user not found")})
	got := newModel.(*Model)

	if got.err == nil {
		t.Fatal("expected err to be set after fetch error, got nil")
	}
	if !strings.Contains(got.err.Error(), "edit the username and press Enter to retry") {
		t.Errorf("expected retry hint in error message, got: %v", got.err)
	}
	if got.Wizard.State.CurrentStep != model.StepUser {
		t.Errorf("expected to remain on StepUser after fetch error, got %v",
			got.Wizard.State.CurrentStep)
	}
}

// --- fetchKeysMsg: empty keys, no other auth ---

// TestFetchKeysEmpty_NoOtherAuth_ShowsError verifies that when GitHub returns
// zero keys and the user has no password and no manually-entered SSH keys, the
// model sets a "no SSH keys found" error and stays on StepUser.
func TestFetchKeysEmpty_NoOtherAuth_ShowsError(t *testing.T) {
	// Point HOME at an empty temp dir so detectLocalSSHKeys() returns nothing.
	t.Setenv("HOME", t.TempDir())

	w := newTestWizard()
	w.State.CurrentStep = model.StepUser
	w.State.Config.Users = []model.UserConfig{{Username: "core"}}
	// No PasswordHash, no pre-existing SSHKeys.

	m := New(w)
	m.sshKeyInput = "" // no manual key input

	newModel, _ := m.Update(fetchKeysMsg{keys: nil, err: nil})
	got := newModel.(*Model)

	if got.err == nil {
		t.Fatal("expected err to be set when no SSH keys and no password, got nil")
	}
	if !strings.Contains(got.err.Error(), "no SSH keys found") {
		t.Errorf("expected 'no SSH keys found' in error message, got: %v", got.err)
	}
	if got.Wizard.State.CurrentStep != model.StepUser {
		t.Errorf("expected to remain on StepUser when no auth available, got %v",
			got.Wizard.State.CurrentStep)
	}
}

// --- fetchKeysMsg: empty keys, but password is set ---

// TestFetchKeysEmpty_WithPassword_Advances verifies that when GitHub returns
// zero keys but the user already has a PasswordHash set, the model treats that
// as sufficient authentication and advances past StepUser.
func TestFetchKeysEmpty_WithPassword_Advances(t *testing.T) {
	// Point HOME at an empty temp dir so detectLocalSSHKeys() returns nothing.
	t.Setenv("HOME", t.TempDir())

	w := newTestWizard()
	w.State.CurrentStep = model.StepUser
	// Bcrypt hash for "testpass" — value just needs to look like a hash; validation
	// only checks the presence of a non-empty string here.
	w.State.Config.Users = []model.UserConfig{{
		Username:     "core",
		PasswordHash: "$2a$10$aaaabbbbccccddddeeeeff0000111122223333445",
	}}

	m := New(w)
	m.sshKeyInput = "" // no manual key input

	newModel, _ := m.Update(fetchKeysMsg{keys: nil, err: nil})
	got := newModel.(*Model)

	if got.Wizard.State.CurrentStep == model.StepUser {
		t.Errorf("expected to advance past StepUser when password is set, still on StepUser (err=%v)",
			got.err)
	}
	if got.err != nil {
		t.Errorf("unexpected error when password hash is present: %v", got.err)
	}
}

// --- handleEnter: StepUser, no auth, no GitHub field ---

// TestEnterUserStep_NoAuth_NoGithub_ShowsError verifies that pressing Enter on
// StepUser without any authentication configured (no SSH keys, no password, no
// GitHub username) sets a "no authentication configured" error and keeps the
// wizard on StepUser.
func TestEnterUserStep_NoAuth_NoGithub_ShowsError(t *testing.T) {
	// Point HOME at an empty temp dir so detectLocalSSHKeys() returns nothing.
	t.Setenv("HOME", t.TempDir())

	w := newTestWizard()
	w.State.CurrentStep = model.StepUser
	w.State.Config.Users = []model.UserConfig{{Username: "core"}}
	// No PasswordHash, no SSHKeys.

	m := New(w)
	// Bypass the huh form so handleEnter() is reached directly.
	m.activeForm = nil
	// Ensure no github_user field carries a value.
	for i := range m.fields {
		if m.fields[i].key == "github_user" {
			m.fields[i].value = ""
		}
	}

	_, _ = m.handleEnter()

	if m.err == nil {
		t.Fatal("expected err to be set when no auth configured, got nil")
	}
	if !strings.Contains(m.err.Error(), "no authentication configured") {
		t.Errorf("expected 'no authentication configured' in error message, got: %v", m.err)
	}
	if m.Wizard.State.CurrentStep != model.StepUser {
		t.Errorf("expected to remain on StepUser when no auth configured, got %v",
			m.Wizard.State.CurrentStep)
	}
}

// --- handleEnter: StepUser, local SSH keys available ---

// TestEnterUserStep_WithLocalKeys_Advances verifies that when ~/.ssh/*.pub
// contains a valid SSH key, handleEnter() on StepUser detects it via
// detectLocalSSHKeys(), treats it as sufficient authentication, and advances
// the wizard past StepUser.
func TestEnterUserStep_WithLocalKeys_Advances(t *testing.T) {
	// Create a temp HOME containing a valid SSH public key.
	dir := t.TempDir()
	sshDir := filepath.Join(dir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("creating temp .ssh dir: %v", err)
	}
	// Key format only needs type + base64-ish blob to pass validate.SSHPublicKey.
	keyLine := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIBXntestlocalkey test@local\n"
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519.pub"), []byte(keyLine), 0600); err != nil {
		t.Fatalf("writing temp pubkey: %v", err)
	}
	t.Setenv("HOME", dir)

	w := newTestWizard()
	w.State.CurrentStep = model.StepUser
	w.State.Config.Users = []model.UserConfig{{Username: "core"}}

	m := New(w)
	// Bypass the huh form so handleEnter() is reached directly.
	m.activeForm = nil
	// No GitHub username — no async fetch triggered.
	for i := range m.fields {
		if m.fields[i].key == "github_user" {
			m.fields[i].value = ""
		}
	}

	_, _ = m.handleEnter()

	if m.err != nil {
		t.Errorf("unexpected error with local SSH key present: %v", m.err)
	}
	if m.Wizard.State.CurrentStep == model.StepUser {
		t.Errorf("expected to advance past StepUser when local SSH key is present, still on StepUser")
	}
}
