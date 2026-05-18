package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/castrojo/knuckle/internal/model"
)

func TestFullWizardNavigation(t *testing.T) {
	w := newTestWizard()
	// Set up disks so storage step can proceed
	w.State.Disks = []model.DiskInfo{
		{DevPath: "/dev/vda", Model: "QEMU", SizeHuman: "50 GB", Transport: "virtio"},
	}
	w.State.Config.Users = []model.UserConfig{
		{Username: "core", SSHKeys: []string{"ssh-ed25519 AAAA test"}},
	}
	w.State.Config.SSHKeys = []string{"ssh-ed25519 AAAA test"}
	w.State.Config.Hostname = "test-node"

	m := New(w)

	// Navigate: Welcome → Network → Storage → User → Sysext → Update → Review
	steps := []model.WizardStep{
		model.StepWelcome,
		model.StepNetwork,
		model.StepStorage,
		model.StepUser,
		model.StepSysext,
		model.StepUpdate,
		model.StepReview,
	}

	for i, expectedStep := range steps {
		if m.Wizard.State.CurrentStep != expectedStep {
			t.Fatalf("step %d: expected %v, got %v", i, expectedStep, m.Wizard.State.CurrentStep)
		}
		// Press Enter to advance (for storage, select disk first)
		if expectedStep == model.StepStorage {
			m.cursor = 0 // select first disk
		}
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = newModel.(*Model)
	}

	// Should be at Review
	if m.Wizard.State.CurrentStep != model.StepReview {
		t.Errorf("expected StepReview, got %v", m.Wizard.State.CurrentStep)
	}
}

func TestBackNavigation(t *testing.T) {
	w := newTestWizard()
	w.State.CurrentStep = model.StepUser
	m := New(w)

	// Press Esc to go back
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tuiModel := newModel.(*Model)

	if tuiModel.Wizard.State.CurrentStep != model.StepStorage {
		t.Errorf("expected StepStorage after Esc from User, got %v", tuiModel.Wizard.State.CurrentStep)
	}
}

func TestIgnitionURLSkipsSteps(t *testing.T) {
	w := newTestWizard()
	w.State.Disks = []model.DiskInfo{
		{DevPath: "/dev/vda", Model: "QEMU", SizeHuman: "50 GB"},
	}
	m := New(w)
	// Set ignition URL in welcome field (index 2 = "ignition_url")
	m.fields[2].value = "https://example.com/config.ign"

	// Press Enter on Welcome — should skip to Storage
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tuiModel := newModel.(*Model)

	if tuiModel.Wizard.State.CurrentStep != model.StepStorage {
		t.Errorf("expected skip to StepStorage with IgnitionURL, got %v", tuiModel.Wizard.State.CurrentStep)
	}
}
