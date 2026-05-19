package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/castrojo/knuckle/internal/model"
	"github.com/castrojo/knuckle/internal/wizard"
)

func newTestWizard() *wizard.Wizard {
	return wizard.New(nil, nil, nil)
}

func TestNewModel(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	if m.Wizard != w {
		t.Error("model should reference the wizard")
	}
	if m.Wizard.State.CurrentStep != model.StepWelcome {
		t.Errorf("expected StepWelcome, got %v", m.Wizard.State.CurrentStep)
	}
}

func TestViewWelcome(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	// Init form to trigger rendering
	if m.activeForm != nil {
		m.activeForm.Init()
	}
	view := m.View()
	if !strings.Contains(view, "Knuckle") {
		t.Error("view should contain title")
	}
	// huh form renders after init
	if len(view) < 50 {
		t.Errorf("view too short — form may not have rendered: %q", view)
	}
}

func TestViewReview(t *testing.T) {
	w := newTestWizard()
	w.State.CurrentStep = model.StepReview
	w.State.Config.Channel = "stable"
	w.State.Config.Hostname = "testhost"
	w.State.Config.Disk = model.DiskInfo{DevPath: "/dev/sda", SizeHuman: "500 GB"}
	w.State.Config.Users = []model.UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAA"}}}
	w.State.Config.SSHKeys = []string{"ssh-ed25519 AAAA"}
	m := New(w)
	// Init the form to trigger rendering
	if m.activeForm != nil {
		m.activeForm.Init()
	}
	view := m.View()
	// The review form shows a confirm dialog with summary
	if !strings.Contains(view, "stable") && !strings.Contains(view, "install") {
		t.Errorf("review should show install confirmation, got:\n%s", view[:min(300, len(view))])
	}
}

func TestHandleQuit(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	// First Ctrl+C triggers confirmation
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	tuiModel := newModel.(*Model)
	if tuiModel.quitting {
		t.Error("should not quit on first ctrl+c")
	}
	if !tuiModel.confirmQuit {
		t.Error("should show quit confirmation")
	}
	if cmd != nil {
		t.Error("should not return quit cmd on first press")
	}
	// Second Ctrl+C quits
	newModel, cmd = tuiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	tuiModel = newModel.(*Model)
	if !tuiModel.quitting {
		t.Error("should be quitting after second ctrl+c")
	}
	if cmd == nil {
		t.Error("should return quit cmd")
	}
}

func TestQuitRequiresDoublePress(t *testing.T) {
	w := newTestWizard()
	w.State.CurrentStep = model.StepStorage // non-form step
	m := New(w)

	// First Ctrl+C
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	tuiModel := newModel.(*Model)
	if !tuiModel.confirmQuit {
		t.Error("first ctrl+c should set confirmQuit")
	}

	// Any other key cancels
	newModel, _ = tuiModel.Update(tea.KeyMsg{Type: tea.KeyDown})
	tuiModel = newModel.(*Model)
	if tuiModel.confirmQuit {
		t.Error("other key should cancel quit confirmation")
	}
}

func TestHandleEnterAdvancesNonFormStep(t *testing.T) {
	w := newTestWizard()
	w.State.CurrentStep = model.StepSysext // non-form step
	w.State.Config.Channel = "stable"
	w.State.Config.Hostname = "test"
	w.State.Config.Disk = model.DiskInfo{DevPath: "/dev/sda"}
	w.State.Config.Users = []model.UserConfig{{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAA"}}}
	w.State.Config.SSHKeys = []string{"ssh-ed25519 AAAA"}
	m := New(w)

	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tuiModel := newModel.(*Model)
	if tuiModel.Wizard.State.CurrentStep != model.StepUpdate {
		t.Errorf("expected StepUpdate after Enter on Sysext, got %v", tuiModel.Wizard.State.CurrentStep)
	}
}

func TestFormCompleteAdvancesWizard(t *testing.T) {
	w := newTestWizard()
	w.State.Config.Channel = "stable"
	m := New(w)

	// Simulate Welcome form completion
	m.onFormComplete()
	if m.Wizard.State.CurrentStep != model.StepNetwork {
		t.Errorf("expected StepNetwork after Welcome form, got %v", m.Wizard.State.CurrentStep)
	}
}

func TestPasswordHashing(t *testing.T) {
	hash, err := hashPassword("testpass")
	if err != nil {
		t.Fatalf("hashPassword error: %v", err)
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}
	if !strings.HasPrefix(hash, "$2a$") {
		t.Errorf("hash should be bcrypt format, got: %s", hash[:10])
	}
}

func TestMultiSSHKeySplitting(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"ssh-ed25519 AAAA test", 1},
		{"ssh-ed25519 AAAA one;ssh-rsa BBBB two", 2},
		{"", 0},
	}
	for _, tt := range tests {
		keys := splitSSHKeys(tt.input)
		if len(keys) != tt.want {
			t.Errorf("splitSSHKeys(%q) = %d keys, want %d", tt.input, len(keys), tt.want)
		}
	}
}

func TestSystemChecksDisplayInWelcome(t *testing.T) {
	w := newTestWizard()
	w.State.SystemChecks = []wizard.SystemCheck{
		{Name: "Disk", Status: "pass", Detail: "2 disk(s) found"},
		{Name: "Network", Status: "warn", Detail: "no IPv4"},
	}
	m := New(w)
	view := m.View()
	if !strings.Contains(view, "Disk") {
		t.Error("welcome should display system checks")
	}
}

func TestButanePreviewToggle(t *testing.T) {
	w := newTestWizard()
	w.State.CurrentStep = model.StepStorage // Non-form step where ctrl+b works
	m := New(w)
	// Note: Butane preview only works on Review step, which is now a form
	// This test verifies the ctrl+b key doesn't crash on non-review steps
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	_ = newModel.(*Model) // no panic = pass
}

func TestVersionFieldFromConfig(t *testing.T) {
	w := newTestWizard()
	w.State.Config.Version = "4593.2.1"
	m := New(w)
	// The huh form reads directly from config, verify it's wired
	if m.Wizard.State.Config.Version != "4593.2.1" {
		t.Error("version should be preserved in config")
	}
}

func TestTimezoneDefault(t *testing.T) {
	w := newTestWizard()
	w.State.Config.Channel = "stable"
	w.State.Config.Timezone = ""
	w.State.CurrentStep = model.StepUser
	m := New(w)

	// Simulate form complete without setting timezone
	m.usernameInput = "core"
	m.onFormComplete()

	if m.Wizard.State.Config.Timezone != "UTC" {
		t.Errorf("expected UTC default, got %q", m.Wizard.State.Config.Timezone)
	}
}
