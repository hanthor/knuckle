package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/crypto/bcrypt"

	"github.com/castrojo/knuckle/internal/github"
	"github.com/castrojo/knuckle/internal/model"
	"github.com/castrojo/knuckle/internal/validate"
	"github.com/castrojo/knuckle/internal/wizard"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")).MarginBottom(1)
	stepStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1)
)

// installProgressMsg carries a progress line from the install goroutine.
type installProgressMsg string

// installDoneMsg signals install completion (success or failure).
type installDoneMsg struct{ err error }

// fetchKeysMsg carries the result of an async GitHub key fetch.
type fetchKeysMsg struct {
	keys []string
	err  error
}

// Model is the top-level Bubble Tea model
type Model struct {
	Wizard      *wizard.Wizard
	width       int
	height      int
	err         error
	quitting    bool
	confirmQuit bool
	showButane  bool
	installing  bool
	fetching    bool
	cursor      int
	fields      []field
	fieldIdx    int

	// huh form state
	activeForm       *huh.Form
	dnsInput         string
	networkModeInput string
	usernameInput    string
	passwordInput    string
	githubUserInput  string
	sshKeyInput      string
	showAdvanced     bool

	// Install progress
	spinner    spinner.Model
	progress   progress.Model
	progressCh chan string
}

type field struct {
	label  string
	value  string
	key    string
	masked bool
}

// New creates a new TUI model
func New(w *wizard.Wizard) *Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	p := progress.New(
		progress.WithGradient("#50fa7b", "#ff79c6"),
		progress.WithWidth(40),
	)

	m := &Model{
		Wizard:   w,
		spinner:  s,
		progress: p,
	}
	if len(w.State.Config.Users) > 0 {
		m.usernameInput = w.State.Config.Users[0].Username
	}
	m.initStepFields()
	m.initForm()
	return m
}

func (m *Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.activeForm != nil {
		cmds = append(cmds, m.activeForm.Init())
	}
	cmds = append(cmds, m.spinner.Tick)
	return tea.Batch(cmds...)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global keys override form
		switch msg.String() {
		case "ctrl+c":
			if m.confirmQuit {
				m.quitting = true
				return m, tea.Quit
			}
			m.confirmQuit = true
			m.err = fmt.Errorf("press Ctrl+C again to quit, or any other key to continue")
			return m, nil
		case "ctrl+a":
			// Toggle advanced mode on Welcome step
			if m.Wizard.State.CurrentStep == model.StepWelcome {
				m.showAdvanced = !m.showAdvanced
				m.initForm()
				return m, m.activeForm.Init()
			}
		}
		m.confirmQuit = false
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Forward to active form so it knows its rendering width
		if m.activeForm != nil {
			form, cmd := m.activeForm.Update(msg)
			if f, ok := form.(*huh.Form); ok {
				m.activeForm = f
			}
			return m, cmd
		}
		return m, nil
	case installProgressMsg:
		m.Wizard.State.ProgressMessages = append(m.Wizard.State.ProgressMessages, string(msg))
		// Update progress bar + continue listening
		total := 5.0
		done := float64(len(m.Wizard.State.ProgressMessages))
		pCmd := m.progress.SetPercent(done / total)
		return m, tea.Batch(pCmd, m.waitForProgress())
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case progress.FrameMsg:
		newProgress, cmd := m.progress.Update(msg)
		m.progress = newProgress.(progress.Model)
		return m, cmd
	case installDoneMsg:
		m.installing = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.Wizard.State.CurrentStep = model.StepDone
		// Don't quit immediately — show Done screen, let user press q to exit
		return m, nil
	case fetchKeysMsg:
		m.fetching = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		cfg := &m.Wizard.State.Config
		cfg.SSHKeys = msg.keys
		if len(cfg.Users) > 0 {
			cfg.Users[0].SSHKeys = msg.keys
		}
		if err := m.Wizard.Next(); err != nil {
			m.err = err
			return m, nil
		}
		m.err = nil
		m.cursor = 0
		m.initStepFields()
		m.initForm()
		if m.activeForm != nil {
			return m, m.activeForm.Init()
		}
		return m, nil
	}

	// Delegate to huh form if active
	if m.activeForm != nil {
		form, cmd := m.activeForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.activeForm = f
		}
		if m.activeForm.State == huh.StateCompleted {
			return m, m.onFormComplete()
		}
		if m.activeForm.State == huh.StateAborted {
			m.Wizard.Previous()
			m.err = nil
			m.cursor = 0
			m.initStepFields()
			m.initForm()
			if m.activeForm != nil {
				return m, m.activeForm.Init()
			}
			return m, nil
		}
		return m, cmd
	}

	// Non-form steps: handle keys
	if msg, ok := msg.(tea.KeyMsg); ok {
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Only allow 'q' to quit when NOT editing text fields
	switch msg.String() {
	case "ctrl+c":
		if m.confirmQuit {
			m.quitting = true
			return m, tea.Quit
		}
		m.confirmQuit = true
		m.err = fmt.Errorf("press Ctrl+C again to quit, or any other key to continue")
		return m, nil
	case "q":
		if len(m.fields) > 0 {
			// In field-editing mode, treat as regular character
			m.fields[m.fieldIdx].value += "q"
			return m, nil
		}
		// On non-field steps, require confirmation (same as Ctrl+C)
		if m.confirmQuit {
			m.quitting = true
			return m, tea.Quit
		}
		m.confirmQuit = true
		m.err = fmt.Errorf("press q again to quit, or any other key to continue")
		return m, nil
	case "enter":
		m.confirmQuit = false
		return m.handleEnter()
	case "tab", "down":
		m.confirmQuit = false
		if len(m.fields) > 0 {
			m.fieldIdx = (m.fieldIdx + 1) % len(m.fields)
		} else {
			m.cursor++
			// Clamp cursor to list bounds
			maxCursor := m.maxCursor()
			if m.cursor >= maxCursor {
				m.cursor = maxCursor - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
		}
		return m, nil
	case "shift+tab", "up":
		if len(m.fields) > 0 {
			m.fieldIdx--
			if m.fieldIdx < 0 {
				m.fieldIdx = len(m.fields) - 1
			}
		} else if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "backspace":
		if len(m.fields) > 0 && len(m.fields[m.fieldIdx].value) > 0 {
			m.fields[m.fieldIdx].value = m.fields[m.fieldIdx].value[:len(m.fields[m.fieldIdx].value)-1]
		}
		return m, nil
	case "esc":
		m.Wizard.Previous()
		m.err = nil
		m.initStepFields()
		return m, nil
	case " ":
		if m.Wizard.State.CurrentStep == model.StepSysext && m.cursor < len(m.Wizard.State.Sysexts) {
			m.Wizard.State.Sysexts[m.cursor].Selected = !m.Wizard.State.Sysexts[m.cursor].Selected
			m.Wizard.State.Config.Sysexts = m.Wizard.State.Sysexts
		} else if len(m.fields) > 0 {
			m.fields[m.fieldIdx].value += " "
		}
		return m, nil
	case "ctrl+b":
		if m.Wizard.State.CurrentStep == model.StepReview {
			m.showButane = !m.showButane
		}
		return m, nil
	default:
		if len(m.fields) > 0 && len(msg.String()) == 1 {
			m.fields[m.fieldIdx].value += msg.String()
		}
		return m, nil
	}
}

// maxCursor returns the number of selectable items in list-based steps
func (m *Model) maxCursor() int {
	switch m.Wizard.State.CurrentStep {
	case model.StepStorage:
		return len(m.Wizard.State.Disks)
	case model.StepSysext:
		return len(m.Wizard.State.Sysexts)
	case model.StepUpdate:
		return 3
	default:
		return 1
	}
}

func (m *Model) handleEnter() (tea.Model, tea.Cmd) {
	step := m.Wizard.State.CurrentStep
	m.applyFields()

	switch step {
	case model.StepWelcome:
		// If IgnitionURL is set, skip directly to Storage
		if m.Wizard.State.Config.IgnitionURL != "" {
			m.Wizard.GoToStep(model.StepStorage)
			m.err = nil
			m.cursor = 0
			m.initStepFields()
			m.initForm()
			if m.activeForm != nil {
				return m, m.activeForm.Init()
			}
			return m, nil
		}
	case model.StepStorage:
		if m.cursor < len(m.Wizard.State.Disks) {
			m.Wizard.State.Config.Disk = m.Wizard.State.Disks[m.cursor]
		}
		// If IgnitionURL is set, skip to Review after Storage
		if m.Wizard.State.Config.IgnitionURL != "" {
			if err := m.Wizard.ValidateCurrentStep(); err != nil {
				m.err = err
				return m, nil
			}
			m.Wizard.GoToStep(model.StepReview)
			m.err = nil
			m.cursor = 0
			m.initStepFields()
			m.initForm()
			if m.activeForm != nil {
				return m, m.activeForm.Init()
			}
			return m, nil
		}
	case model.StepUpdate:
		strategies := []string{"reboot", "off", "etcd-lock"}
		if m.cursor >= 0 && m.cursor < len(strategies) {
			m.Wizard.State.Config.UpdateStrategy.RebootStrategy = strategies[m.cursor]
		}
	case model.StepUser:
		// Trigger async GitHub key fetch if username is provided
		for _, f := range m.fields {
			if f.key == "github_user" && f.value != "" && !m.fetching {
				m.fetching = true
				username := f.value
				return m, func() tea.Msg {
					keys, err := github.FetchKeys(username)
					return fetchKeysMsg{keys: keys, err: err}
				}
			}
		}
	case model.StepInstall:
		if !m.installing {
			m.installing = true
			return m, m.startInstall()
		}
		return m, nil
	}

	if err := m.Wizard.Next(); err != nil {
		m.err = err
		return m, nil
	}

	m.err = nil
	m.cursor = 0
	m.initStepFields()
	m.initForm()

	if m.Wizard.State.CurrentStep == model.StepDone {
		return m, tea.Quit
	}
	if m.activeForm != nil {
		return m, m.activeForm.Init()
	}
	return m, nil
}

func (m *Model) startInstall() tea.Cmd {
	// Use a channel to send progress messages back to the TUI
	progressCh := make(chan string, 10)
	m.progressCh = progressCh

	// Start install in background
	go func() {
		defer close(progressCh)
		defer func() {
			if r := recover(); r != nil {
				progressCh <- fmt.Sprintf("PANIC: %v", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		progress := func(msg string) {
			progressCh <- msg
		}
		if err := m.Wizard.ExecuteWithProgress(ctx, progress); err != nil {
			progressCh <- "ERROR:" + err.Error()
		}
	}()

	// Return a Cmd that polls the channel
	return m.waitForProgress()
}

func (m *Model) waitForProgress() tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-m.progressCh
		if !ok {
			// Channel closed — install finished
			return installDoneMsg{err: nil}
		}
		if strings.HasPrefix(msg, "ERROR:") {
			return installDoneMsg{err: fmt.Errorf("%s", strings.TrimPrefix(msg, "ERROR:"))}
		}
		if strings.HasPrefix(msg, "PANIC:") {
			return installDoneMsg{err: fmt.Errorf("%s", msg)}
		}
		return installProgressMsg(msg)
	}
}

func (m *Model) applyFields() {
	cfg := &m.Wizard.State.Config
	switch m.Wizard.State.CurrentStep {
	case model.StepWelcome:
		for _, f := range m.fields {
			switch f.key {
			case "channel":
				if f.value != "" {
					if err := validate.Channel(f.value); err != nil {
						m.err = err
						return
					}
					cfg.Channel = f.value
				}
			case "version":
				cfg.Version = f.value
			case "ignition_url":
				cfg.IgnitionURL = f.value
			}
		}
	case model.StepNetwork:
		for _, f := range m.fields {
			switch f.key {
			case "interface":
				cfg.Network.Interface = f.value
			case "address":
				cfg.Network.Address = f.value
			case "gateway":
				cfg.Network.Gateway = f.value
			case "dns":
				if f.value != "" {
					cfg.Network.DNS = strings.Split(f.value, ",")
				}
			}
		}
		// Switch to static mode if any static fields are filled in
		if cfg.Network.Address != "" || cfg.Network.Gateway != "" {
			cfg.Network.Mode = model.NetworkStatic
		} else {
			cfg.Network.Mode = model.NetworkDHCP
		}
	case model.StepUser:
		for _, f := range m.fields {
			switch f.key {
			case "hostname":
				cfg.Hostname = f.value
			case "timezone":
				if f.value != "" {
					cfg.Timezone = f.value
				} else {
					cfg.Timezone = "UTC"
				}
			case "username":
				if f.value != "" {
					if len(cfg.Users) == 0 {
						cfg.Users = append(cfg.Users, model.UserConfig{
							Username: f.value,
							Groups:   []string{"sudo", "docker"},
						})
					} else {
						cfg.Users[0].Username = f.value
					}
				}
			case "password":
				if f.value != "" && len(cfg.Users) > 0 {
					hash, err := hashPassword(f.value)
					if err != nil {
						m.err = err
						return
					}
					cfg.Users[0].PasswordHash = hash
				}
			case "github_user":
				// GitHub key fetch is handled async in handleEnter()
				// Nothing to do here — fetch triggers on step advance
			case "ssh_key":
				if f.value != "" {
					// Support multiple keys separated by semicolons
					keys := splitSSHKeys(f.value)
					cfg.SSHKeys = keys
					if len(cfg.Users) > 0 {
						cfg.Users[0].SSHKeys = keys
					}
				}
			}
		}
	case model.StepReview:
		for _, f := range m.fields {
			if f.key == "confirm" {
				m.Wizard.State.Confirmed = (strings.ToUpper(strings.TrimSpace(f.value)) == "YES")
			}
		}
	}
}

func (m *Model) initStepFields() {
	m.fields = nil
	m.fieldIdx = 0
	switch m.Wizard.State.CurrentStep {
	case model.StepWelcome:
		m.fields = []field{
			{label: "Channel (stable/beta/alpha/edge)", key: "channel", value: m.Wizard.State.Config.Channel},
			{label: "Version (blank = latest)", key: "version", value: m.Wizard.State.Config.Version},
			{label: "External Ignition URL (skip wizard)", key: "ignition_url", value: m.Wizard.State.Config.IgnitionURL},
		}
	case model.StepNetwork:
		m.fields = []field{
			{label: "Interface", key: "interface", value: m.Wizard.State.Config.Network.Interface},
			{label: "IP Address (CIDR)", key: "address", value: m.Wizard.State.Config.Network.Address},
			{label: "Gateway", key: "gateway", value: m.Wizard.State.Config.Network.Gateway},
			{label: "DNS (comma-separated)", key: "dns", value: strings.Join(m.Wizard.State.Config.Network.DNS, ",")},
		}
	case model.StepUser:
		username := ""
		if len(m.Wizard.State.Config.Users) > 0 {
			username = m.Wizard.State.Config.Users[0].Username
		}
		sshKey := ""
		if len(m.Wizard.State.Config.SSHKeys) > 0 {
			sshKey = m.Wizard.State.Config.SSHKeys[0]
		}
		m.fields = []field{
			{label: "Hostname", key: "hostname", value: m.Wizard.State.Config.Hostname},
			{label: "Timezone (e.g. UTC, America/New_York)", key: "timezone", value: m.Wizard.State.Config.Timezone},
			{label: "Username", key: "username", value: username},
			{label: "Password (optional, leave blank for key-only)", key: "password", value: "", masked: true},
			{label: "GitHub Username (fetches SSH keys)", key: "github_user", value: ""},
			{label: "— OR — SSH Public Key(s) (separate with ;)", key: "ssh_key", value: sshKey},
		}
	case model.StepReview:
		m.fields = []field{
			{label: "Type YES to confirm installation", key: "confirm", value: ""},
		}
	case model.StepUpdate:
		// No fields — cursor-select screen
	}
}

func (m *Model) View() string {
	if m.quitting {
		return "Installation cancelled.\n"
	}

	// Form-based steps use huh rendering
	if m.activeForm != nil {
		return m.viewWithForm()
	}

	// Non-form steps use manual rendering
	var b strings.Builder
	b.WriteString(titleStyle.Render("🔧 Knuckle — Flatcar Container Linux Installer"))
	b.WriteString("\n")
	b.WriteString(m.renderProgressBar())
	b.WriteString("\n")

	switch m.Wizard.State.CurrentStep {
	case model.StepStorage:
		b.WriteString(m.viewStorage())
	case model.StepSysext:
		b.WriteString(m.viewSysext())
	case model.StepUpdate:
		b.WriteString(m.viewUpdate())
	case model.StepInstall:
		b.WriteString(m.viewInstall())
	case model.StepDone:
		b.WriteString(m.viewDone())
	}

	if m.err != nil {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(fmt.Sprintf("⚠ %s", m.err.Error())))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ navigate • enter confirm • esc back • q quit"))
	return b.String()
}

func (m *Model) viewWelcome() string {
	var b strings.Builder
	b.WriteString(`Welcome to Knuckle!

This wizard will guide you through installing Flatcar Container Linux
on your system.

What this installer will do:
  • Detect your hardware (disks, network)
  • Configure networking (DHCP or static)
  • Set up user accounts and SSH keys
  • Optionally add system extensions
  • Write Flatcar to your selected disk

`)
	// Show system checks
	if len(m.Wizard.State.SystemChecks) > 0 {
		b.WriteString("System checks:\n")
		for _, check := range m.Wizard.State.SystemChecks {
			icon := "✓"
			if check.Status == "warn" {
				icon = "⚠"
			} else if check.Status == "fail" {
				icon = "✗"
			}
			fmt.Fprintf(&b, "  %s %s: %s\n", icon, check.Name, check.Detail)
		}
		b.WriteString("\n")
	}
	// Show channel version info if available
	if len(m.Wizard.State.Channels) > 0 {
		b.WriteString("Available channels:\n")
		for _, ch := range m.Wizard.State.Channels {
			fmt.Fprintf(&b, "  %s — Flatcar %s\n", ch.Channel, ch.Version)
			fmt.Fprintf(&b, "    kernel %s · systemd %s · docker %s · containerd %s\n",
				ch.Kernel, ch.Systemd, ch.Docker, ch.Containerd)
		}
		b.WriteString("\n")
	}
	for i, f := range m.fields {
		cursor := "  "
		if i == m.fieldIdx {
			cursor = "▸ "
		}
		fmt.Fprintf(&b, "%s%s: %s\n", cursor, f.label, f.value)
	}
	b.WriteString("\nPress Enter to continue...")
	return b.String()
}

func (m *Model) viewNetwork() string {
	var b strings.Builder
	b.WriteString("Network Configuration\n\n")
	if len(m.Wizard.State.Interfaces) > 0 {
		b.WriteString("Detected interfaces:\n")
		for _, iface := range m.Wizard.State.Interfaces {
			fmt.Fprintf(&b, "  • %s (%s) — %s\n", iface.Name, iface.MAC, iface.State)
		}
		b.WriteString("\n")
	}
	b.WriteString("Using DHCP by default. Fill in fields for static config:\n\n")
	for i, f := range m.fields {
		cursor := "  "
		if i == m.fieldIdx {
			cursor = "▸ "
		}
		fmt.Fprintf(&b, "%s%s: %s\n", cursor, f.label, f.value)
	}
	return b.String()
}

func (m *Model) viewStorage() string {
	var b strings.Builder
	b.WriteString("Select Target Disk\n\n")
	if len(m.Wizard.State.Disks) == 0 {
		b.WriteString("No disks detected!\n")
		return b.String()
	}
	for i, disk := range m.Wizard.State.Disks {
		cursor := "  "
		if i == m.cursor {
			cursor = "▸ "
		}
		line := fmt.Sprintf("%s%s — %s [%s] %s", cursor, disk.DevPath, disk.Model, disk.SizeHuman, disk.Transport)
		if disk.Removable {
			line += " (removable)"
		}
		if i == m.cursor {
			b.WriteString(selectedStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n⚠ WARNING: All data on the selected disk will be erased!\n")
	return b.String()
}

func (m *Model) viewUser() string {
	var b strings.Builder
	b.WriteString("User Configuration\n\n")
	b.WriteString("Enter a GitHub username to auto-fetch your SSH keys,\nor paste a key manually.\n\n")
	for i, f := range m.fields {
		cursor := "  "
		if i == m.fieldIdx {
			cursor = "▸ "
		}
		displayVal := f.value
		if f.masked && f.value != "" {
			displayVal = strings.Repeat("•", len(f.value))
		}
		fmt.Fprintf(&b, "%s%s: %s\n", cursor, f.label, displayVal)
	}
	if m.fetching {
		b.WriteString("\n  ⠋ Fetching SSH keys from GitHub...\n")
	} else if len(m.Wizard.State.Config.SSHKeys) > 0 {
		fmt.Fprintf(&b, "\n  ✓ %d SSH key(s) configured\n", len(m.Wizard.State.Config.SSHKeys))
	}
	return b.String()
}

func (m *Model) viewSysext() string {
	var b strings.Builder
	b.WriteString("System Extensions (optional)\n\nSpace to toggle, Enter to continue:\n\n")
	if len(m.Wizard.State.Sysexts) == 0 {
		b.WriteString("No extensions available (catalog fetch may have failed)\n")
		return b.String()
	}
	for i, ext := range m.Wizard.State.Sysexts {
		cursor := "  "
		if i == m.cursor {
			cursor = "▸ "
		}
		check := "[ ]"
		if ext.Selected {
			check = "[✓]"
		}
		line := fmt.Sprintf("%s%s %s v%s — %s", cursor, check, ext.Name, ext.Version, ext.Description)
		if i == m.cursor {
			b.WriteString(selectedStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m *Model) viewUpdate() string {
	var b strings.Builder
	b.WriteString("Update Strategy\n\nChoose how Flatcar will handle OS updates:\n\n")

	type option struct {
		name string
		desc []string
	}
	options := []option{
		{"reboot (Recommended)", []string{
			"Auto-update and reboot immediately when an update is applied.",
			"Best for: single nodes, dev environments",
		}},
		{"off", []string{
			"Updates are downloaded but never applied automatically.",
			"You must run 'update_engine_client -update' manually.",
			"Best for: manually managed infrastructure",
		}},
		{"etcd-lock", []string{
			"Coordinates reboots with other nodes via etcd distributed lock.",
			"Only one node reboots at a time in the cluster.",
			"Best for: multi-node clusters running etcd",
		}},
	}

	for i, opt := range options {
		cursor := "  "
		if i == m.cursor {
			cursor = "▸ "
		}
		line := fmt.Sprintf("%s%s", cursor, opt.name)
		if i == m.cursor {
			b.WriteString(selectedStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
		for _, d := range opt.desc {
			b.WriteString(fmt.Sprintf("    %s\n", d))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m *Model) viewReview() string {
	var b strings.Builder
	cfg := m.Wizard.State.Config
	b.WriteString("Review Configuration\n\n")

	if cfg.IgnitionURL != "" {
		fmt.Fprintf(&b, "  Mode:      External Ignition\n")
		fmt.Fprintf(&b, "  URL:       %s\n", cfg.IgnitionURL)
		fmt.Fprintf(&b, "  Disk:      %s (%s)\n", cfg.Disk.DevPath, cfg.Disk.SizeHuman)
	} else {
		fmt.Fprintf(&b, "  Channel:   %s\n", cfg.Channel)
		if cfg.Version != "" {
			fmt.Fprintf(&b, "  Version:   %s\n", cfg.Version)
		}
		fmt.Fprintf(&b, "  Hostname:  %s\n", cfg.Hostname)
		if cfg.Timezone != "" {
			fmt.Fprintf(&b, "  Timezone:  %s\n", cfg.Timezone)
		}
		fmt.Fprintf(&b, "  Disk:      %s (%s)\n", cfg.Disk.DevPath, cfg.Disk.SizeHuman)
		fmt.Fprintf(&b, "  Network:   %s\n", cfg.Network.Mode.String())
		if cfg.Network.Mode == model.NetworkStatic {
			fmt.Fprintf(&b, "  Address:   %s\n", cfg.Network.Address)
			fmt.Fprintf(&b, "  Gateway:   %s\n", cfg.Network.Gateway)
		}
		if len(cfg.Users) > 0 {
			fmt.Fprintf(&b, "  User:      %s\n", cfg.Users[0].Username)
			if cfg.Users[0].PasswordHash != "" {
				fmt.Fprintf(&b, "  Password:  ✓ set\n")
			}
		}
		if len(cfg.SSHKeys) > 0 {
			fmt.Fprintf(&b, "  SSH Keys:  %d configured\n", len(cfg.SSHKeys))
		}
		selected := 0
		for _, s := range cfg.Sysexts {
			if s.Selected {
				selected++
			}
		}
		if selected > 0 {
			fmt.Fprintf(&b, "  Sysexts:   %d selected\n", selected)
		}
		if cfg.UpdateStrategy.RebootStrategy != "" {
			fmt.Fprintf(&b, "  Update:    %s\n", cfg.UpdateStrategy.RebootStrategy)
		}

		// Butane preview
		if m.showButane {
			b.WriteString("\n─── Butane YAML Preview ───\n")
			butane, err := m.Wizard.GenerateButane()
			if err != nil {
				fmt.Fprintf(&b, "  Error: %v\n", err)
			} else {
				// Show first 30 lines
				lines := strings.Split(butane, "\n")
				max := 30
				if len(lines) < max {
					max = len(lines)
				}
				for _, line := range lines[:max] {
					fmt.Fprintf(&b, "  %s\n", line)
				}
				if len(lines) > max {
					fmt.Fprintf(&b, "  ... (%d more lines)\n", len(lines)-max)
				}
			}
			b.WriteString("───────────────────────────\n")
		} else {
			b.WriteString("\n  Press Ctrl+B to preview Butane YAML\n")
		}
	}

	b.WriteString("\n⚠ ALL DATA ON " + cfg.Disk.DevPath + " WILL BE DESTROYED!\n\n")
	for i, f := range m.fields {
		cursor := "  "
		if i == m.fieldIdx {
			cursor = "▸ "
		}
		fmt.Fprintf(&b, "%s%s: %s\n", cursor, f.label, f.value)
	}
	return b.String()
}

func (m *Model) viewInstall() string {
	var b strings.Builder
	b.WriteString("Installing Flatcar Container Linux...\n\n")

	// Animated progress bar
	b.WriteString("  " + m.progress.View() + "\n\n")

	// Completed phases
	for _, msg := range m.Wizard.State.ProgressMessages {
		fmt.Fprintf(&b, "  ✓ %s\n", msg)
	}

	// Current phase with spinner
	if m.installing {
		b.WriteString(fmt.Sprintf("\n  %s Working...\n", m.spinner.View()))
	}

	if !m.installing && len(m.Wizard.State.ProgressMessages) == 0 {
		b.WriteString("\nPress Enter to start installation...")
	}
	return b.String()
}

func (m *Model) viewDone() string {
	return `
✅ Installation Complete!

Flatcar Container Linux has been installed successfully.
Remove the installation media and reboot your system.

Press q to exit.
`
}

// Run starts the Bubble Tea program
func Run(w *wizard.Wizard) error {
	m := New(w)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// hashPassword generates a bcrypt hash suitable for Ignition passwd field.
func hashPassword(plain string) (string, error) {
	if len(plain) > 72 {
		return "", fmt.Errorf("password too long (max 72 bytes for bcrypt)")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(hash), nil
}

// splitSSHKeys splits SSH keys by semicolons and trims whitespace.
func splitSSHKeys(input string) []string {
	parts := strings.Split(input, ";")
	var keys []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			keys = append(keys, p)
		}
	}
	return keys
}
