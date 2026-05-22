package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/projectbluefin/knuckle/internal/bakery"
	"github.com/projectbluefin/knuckle/internal/model"
)

// ── forms.go ─────────────────────────────────────────────────────────────────

func TestBuildNetworkForm_ReturnsForm(t *testing.T) {
	m := New(newTestWizard())
	if m.buildNetworkForm() == nil {
		t.Fatal("buildNetworkForm() returned nil")
	}
}

func TestBuildUserForm_ReturnsForm(t *testing.T) {
	m := New(newTestWizard())
	if m.buildUserForm() == nil {
		t.Fatal("buildUserForm() returned nil")
	}
}

func TestBuildTailscaleForm_ReturnsForm(t *testing.T) {
	m := New(newTestWizard())
	if m.buildTailscaleForm() == nil {
		t.Fatal("buildTailscaleForm() returned nil")
	}
}

func TestReviewSummary_StaticNetwork(t *testing.T) {
	w := newTestWizard()
	w.State.Config.Network = model.NetworkConfig{
		Mode:    model.NetworkStatic,
		Address: "10.0.0.5/24",
		Gateway: "10.0.0.1",
	}
	if s := New(w).reviewSummary(); !strings.Contains(s, "10.0.0.5") {
		t.Errorf("static summary missing address: %q", s)
	}
}

func TestReviewSummary_WithSysexts(t *testing.T) {
	w := newTestWizard()
	w.State.Config.Sysexts = []model.SysextEntry{
		{Name: "docker", Selected: true},
		{Name: "tailscale", Selected: true},
	}
	s := New(w).reviewSummary()
	if !strings.Contains(s, "docker") || !strings.Contains(s, "tailscale") {
		t.Errorf("summary missing sysext names: %q", s)
	}
}

func TestReviewSummary_SwapDisabled(t *testing.T) {
	w := newTestWizard()
	w.State.Config.Swap = model.SwapConfig{Enabled: false}
	if s := New(w).reviewSummary(); !strings.Contains(s, "disabled") {
		t.Errorf("expected 'disabled' in swap summary: %q", s)
	}
}

func TestReviewSummary_WithTailscale(t *testing.T) {
	w := newTestWizard()
	w.State.Config.Tailscale = model.TailscaleConfig{
		AuthKey: "tskey-auth-kSomeID1234567-SomeSecretThatIsLongEnough",
		Mode:    model.TailscaleModeConnect,
	}
	if s := New(w).reviewSummary(); !strings.Contains(s, "Tailscale") {
		t.Errorf("expected Tailscale in summary: %q", s)
	}
}

func TestLocalKeysSummary_DoesNotPanic(t *testing.T) {
	m := New(newTestWizard())
	s := m.localKeysSummary()
	if s == "" {
		t.Error("localKeysSummary() returned empty string")
	}
}

func TestGetChannelMeta_WithChannels(t *testing.T) {
	w := newTestWizard()
	w.State.Channels = []bakery.ChannelInfo{
		{Channel: "stable", Version: "3510.2.0"},
		{Channel: "beta", Version: "3520.0.0"},
	}
	if meta := New(w).getChannelMeta(); len(meta) == 0 {
		t.Fatal("getChannelMeta() returned empty slice when channels are set")
	}
}

func TestChannelList_ReturnsExpectedChannels(t *testing.T) {
	list := New(newTestWizard()).channelList()
	// channelList() always returns the fixed set: stable, lts, beta, alpha
	if len(list) == 0 {
		t.Fatal("channelList() returned empty slice")
	}
	found := make(map[string]bool)
	for _, ch := range list {
		found[ch] = true
	}
	for _, want := range []string{"stable", "beta", "alpha"} {
		if !found[want] {
			t.Errorf("channelList() missing %q", want)
		}
	}
}

func TestChannelCardCount_NonNegative(t *testing.T) {
	m := New(newTestWizard())
	if n := m.channelCardCount(); n < 0 {
		t.Errorf("channelCardCount() = %d, want >= 0", n)
	}
}

// ── form_logic.go: initForm per step ─────────────────────────────────────────

func TestInitForm_NetworkSetsActiveForm(t *testing.T) {
	w := newTestWizard()
	w.State.CurrentStep = model.StepNetwork
	m := New(w)
	m.initForm()
	if m.activeForm == nil {
		t.Error("initForm for StepNetwork should set activeForm")
	}
}

func TestInitForm_UserSetsActiveForm(t *testing.T) {
	w := newTestWizard()
	w.State.CurrentStep = model.StepUser
	m := New(w)
	m.initForm()
	if m.activeForm == nil {
		t.Error("initForm for StepUser should set activeForm")
	}
}

func TestInitForm_TailscaleSetsActiveForm(t *testing.T) {
	w := newTestWizard()
	w.State.CurrentStep = model.StepTailscale
	m := New(w)
	m.initForm()
	if m.activeForm == nil {
		t.Error("initForm for StepTailscale should set activeForm")
	}
}

func TestInitForm_ReviewSetsActiveForm(t *testing.T) {
	w := newTestWizard()
	w.State.CurrentStep = model.StepReview
	m := New(w)
	m.initForm()
	if m.activeForm == nil {
		t.Error("initForm for StepReview should set activeForm")
	}
}

func TestInitForm_StorageNilsActiveForm(t *testing.T) {
	w := newTestWizard()
	w.State.CurrentStep = model.StepStorage
	m := New(w)
	m.activeForm = m.buildNetworkForm() // force non-nil first
	m.initForm()
	if m.activeForm != nil {
		t.Error("initForm for StepStorage should set activeForm = nil")
	}
}

// ── form_logic.go: viewWithForm ───────────────────────────────────────────────

func TestViewWithForm_RendersContent(t *testing.T) {
	w := newTestWizard()
	w.State.CurrentStep = model.StepNetwork
	m := New(w)
	m.initForm()
	if m.activeForm != nil {
		m.activeForm.Init()
	}
	if out := m.viewWithForm(); len(out) == 0 {
		t.Error("viewWithForm() returned empty string")
	}
}

func TestViewWithForm_WithError_ShowsMessage(t *testing.T) {
	w := newTestWizard()
	w.State.CurrentStep = model.StepNetwork
	m := New(w)
	m.initForm()
	m.err = fmt.Errorf("unique-test-error-string")
	out := m.viewWithForm()
	if !strings.Contains(out, "unique-test-error-string") {
		t.Errorf("viewWithForm should render error: %q", out)
	}
}

func TestViewWithForm_FetchingShowsIndicator(t *testing.T) {
	w := newTestWizard()
	w.State.CurrentStep = model.StepUser
	m := New(w)
	m.initForm()
	m.fetching = true
	out := m.viewWithForm()
	if !strings.Contains(out, "Fetching") {
		t.Errorf("viewWithForm should show fetching indicator: %q", out)
	}
}

// ── tui.go: viewInstall branch not yet covered ───────────────────────────────

func TestViewInstall_InstallingBranch(t *testing.T) {
	m := New(newTestWizard())
	m.installing = true
	out := m.viewInstall()
	if !strings.Contains(out, "Working") {
		t.Errorf("installing state should show working indicator: %q", out)
	}
}

// ── sysext_list.go: Update no-op ─────────────────────────────────────────────

func TestSysextDelegateUpdate_IsNoop(t *testing.T) {
	d := newSysextDelegate(func(int) bool { return false })
	if cmd := d.Update(nil, nil); cmd != nil {
		t.Errorf("Update() = %v, want nil (no-op)", cmd)
	}
}
func TestViewSysext_EmptyFallback(t *testing.T) {
	w := newTestWizard()
	m := New(w)
	m.Wizard.State.Sysexts = nil
	out := m.viewSysext()
	if !strings.Contains(out, "No extensions available") {
		t.Errorf("empty sysext state should mention unavailability: %q", out)
	}
}

func TestViewSysext_FallbackRendering(t *testing.T) {
	w := newTestWizard()
	w.State.Sysexts = []model.SysextEntry{
		{Name: "docker", Version: "28.0.0", Category: "container", SupportTier: bakery.TierIntegrated, Selected: true},
		{Name: "tailscale", Version: "1.0.0", Category: "networking", SupportTier: bakery.TierMaintained},
	}
	m := New(w)
	m.sysextListReady = false
	m.cursor = 0
	out := m.viewSysext()
	if !strings.Contains(out, "docker") {
		t.Errorf("viewSysext fallback missing docker: %q", out)
	}
	if !strings.Contains(out, "[✓]") {
		t.Errorf("viewSysext fallback missing selected checkmark: %q", out)
	}
}

func TestViewSysext_SelectedCountCoverage(t *testing.T) {
	w := newTestWizard()
	w.State.Sysexts = []model.SysextEntry{
		{Name: "a", Selected: true},
		{Name: "b", Selected: true},
		{Name: "c", Selected: false},
	}
	m := New(w)
	m.sysextListReady = false
	out := m.viewSysext()
	if !strings.Contains(out, "2 selected") {
		t.Errorf("viewSysext should show selected count: %q", out)
	}
}

func TestViewSysext_NvidiaGPUBanner(t *testing.T) {
	w := newTestWizard()
	w.State.NvidiaGPUDetected = true
	w.State.Sysexts = []model.SysextEntry{{Name: "docker"}}
	m := New(w)
	m.sysextListReady = false
	out := m.viewSysext()
	if !strings.Contains(out, "NVIDIA") {
		t.Errorf("viewSysext should show NVIDIA banner when GPU detected: %q", out)
	}
}

func TestViewSysext_CursorHighlight(t *testing.T) {
	w := newTestWizard()
	w.State.Sysexts = []model.SysextEntry{
		{Name: "alpha", SupportTier: bakery.TierIntegrated},
		{Name: "beta", SupportTier: bakery.TierIntegrated},
	}
	m := New(w)
	m.sysextListReady = false
	m.cursor = 1
	out := m.viewSysext()
	if !strings.Contains(out, "beta") {
		t.Errorf("cursor item should appear in output: %q", out)
	}
}
