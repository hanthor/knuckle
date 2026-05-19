package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/castrojo/knuckle/internal/model"
	"github.com/castrojo/knuckle/internal/validate"
)

// buildWelcomeForm creates the huh form for the Welcome step.
func (m *Model) buildWelcomeForm() *huh.Form {
	channels := []huh.Option[string]{
		huh.NewOption("stable — recommended for production", "stable"),
		huh.NewOption("lts — long-term support", "lts"),
		huh.NewOption("beta — next stable candidate", "beta"),
		huh.NewOption("alpha — bleeding edge", "alpha"),
		huh.NewOption("edge — nightly builds", "edge"),
	}

	helpText := "This wizard will install Flatcar Container Linux on your system.\nIt will configure networking, users, and write the OS to disk.\n\nPress Ctrl+A for advanced options."
	if m.showAdvanced {
		helpText = "Advanced mode enabled. Press Ctrl+A to hide.\n\nVersion pinning installs a specific Flatcar release.\nExternal Ignition URL skips the wizard entirely."
	}

	fields := []huh.Field{
		huh.NewNote().
			Title("Welcome to Knuckle").
			Description(helpText),
		huh.NewSelect[string]().
			Title("Release Channel").
			Description("Choose the update track for this machine").
			Options(channels...).
			Value(&m.Wizard.State.Config.Channel),
	}

	if m.showAdvanced {
		fields = append(fields,
			huh.NewInput().
				Title("Version Pin").
				Description("Install a specific version instead of latest").
				Placeholder("e.g. 4593.2.1 or current-2024 for LTS").
				Value(&m.Wizard.State.Config.Version),
			huh.NewInput().
				Title("External Ignition URL").
				Description("Skip wizard — pass this URL directly to flatcar-install").
				Placeholder("https://example.com/config.ign").
				Value(&m.Wizard.State.Config.IgnitionURL),
		)
	}

	return huh.NewForm(
		huh.NewGroup(fields...),
	).WithTheme(huh.ThemeDracula()).WithShowHelp(true).WithWidth(80)
}

// buildNetworkForm creates the huh form for the Network step.
func (m *Model) buildNetworkForm() *huh.Form {
	// Build interface options from detected interfaces
	ifaceOptions := []huh.Option[string]{
		huh.NewOption("Auto (DHCP on all interfaces)", ""),
	}
	for _, iface := range m.Wizard.State.Interfaces {
		label := fmt.Sprintf("%s — %s (%s)", iface.Name, iface.MAC, iface.State)
		ifaceOptions = append(ifaceOptions, huh.NewOption(label, iface.Name))
	}

	modeOptions := []huh.Option[string]{
		huh.NewOption("DHCP — automatic configuration (recommended)", "dhcp"),
		huh.NewOption("Static — manual IP configuration", "static"),
	}

	fields := []huh.Field{
		huh.NewNote().
			Title("Network Configuration").
			Description("How should this machine connect to the network?"),
		huh.NewSelect[string]().
			Title("Network Mode").
			Options(modeOptions...).
			Value(&m.networkModeInput),
	}

	// Only show static config fields if static mode is likely
	// (huh doesn't support conditional fields, so we always show them
	// but the Note explains they're only used for static)
	staticFields := []huh.Field{
		huh.NewSelect[string]().
			Title("Interface").
			Description("Which network interface to configure").
			Options(ifaceOptions...).
			Value(&m.Wizard.State.Config.Network.Interface),
		huh.NewInput().
			Title("IP Address").
			Description("With subnet mask, e.g. 192.168.1.100/24").
			Placeholder("192.168.1.100/24").
			Value(&m.Wizard.State.Config.Network.Address).
			Validate(func(s string) error {
				if s == "" {
					return nil
				}
				return validate.CIDR(s)
			}),
		huh.NewInput().
			Title("Gateway").
			Placeholder("192.168.1.1").
			Value(&m.Wizard.State.Config.Network.Gateway).
			Validate(func(s string) error {
				if s == "" {
					return nil
				}
				return validate.IPAddress(s)
			}),
		huh.NewInput().
			Title("DNS Servers").
			Description("Comma-separated, e.g. 1.1.1.1,8.8.8.8").
			Placeholder("1.1.1.1").
			Value(&m.dnsInput),
	}

	return huh.NewForm(
		huh.NewGroup(fields...),
		huh.NewGroup(staticFields...).Title("Static IP Configuration").
			Description("Only needed if you chose Static mode above"),
	).WithTheme(huh.ThemeDracula()).WithShowHelp(true).WithWidth(80)
}

// buildUserForm creates the huh form for the User step.
// Split into two groups so it feels like a wizard progression.
func (m *Model) buildUserForm() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("System Identity").
				Description("Configure the hostname and primary user account."),
			huh.NewInput().
				Title("Hostname").
				Placeholder("flatcar-node01").
				Value(&m.Wizard.State.Config.Hostname).
				Validate(func(s string) error {
					if s == "" {
						return nil
					}
					return validate.Hostname(s)
				}),
			huh.NewInput().
				Title("Timezone").
				Placeholder("UTC").
				Description("e.g. America/New_York, Europe/Berlin").
				Value(&m.Wizard.State.Config.Timezone).
				Validate(func(s string) error {
					return validate.Timezone(s)
				}),
			huh.NewInput().
				Title("Username").
				Description("Primary user account").
				Value(&m.usernameInput).
				Validate(func(s string) error {
					if s == "" {
						return nil
					}
					return validate.Username(s)
				}),
			huh.NewInput().
				Title("Password").
				Description("Optional — leave blank for key-only auth").
				EchoMode(huh.EchoModePassword).
				Value(&m.passwordInput),
		),
		huh.NewGroup(
			huh.NewNote().
				Title("Authentication").
				Description("Set up SSH access. Enter a GitHub username to fetch your\npublic keys automatically, or paste a key directly."),
			huh.NewInput().
				Title("GitHub Username").
				Description("Fetches your SSH public keys automatically").
				Placeholder("username or @username").
				Value(&m.githubUserInput),
			huh.NewInput().
				Title("SSH Public Key").
				Description("Or paste key directly (separate multiple with ;)").
				Value(&m.sshKeyInput),
		),
	).WithTheme(huh.ThemeDracula()).WithShowHelp(true).WithWidth(80)
}

// buildReviewForm creates the huh confirm for the Review step.
func (m *Model) buildReviewForm() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("⚠️  DESTRUCTIVE OPERATION — Install Flatcar to disk?").
				Description(m.reviewSummary()).
				Affirmative("Yes, install").
				Negative("Go back").
				Value(&m.Wizard.State.Confirmed),
		),
	).WithTheme(huh.ThemeDracula()).WithShowHelp(true).WithWidth(80)
}

func (m *Model) reviewSummary() string {
	cfg := &m.Wizard.State.Config
	var b strings.Builder
	fmt.Fprintf(&b, "Channel: %s", cfg.Channel)
	if cfg.Version != "" {
		fmt.Fprintf(&b, " (v%s)", cfg.Version)
	}
	fmt.Fprintf(&b, "\nDisk: %s", cfg.Disk.DevPath)
	if cfg.Disk.Model != "" {
		fmt.Fprintf(&b, " (%s, %s)", cfg.Disk.Model, cfg.Disk.SizeHuman)
	}
	fmt.Fprintf(&b, "\nNetwork: %s", cfg.Network.Mode)
	if cfg.Network.Mode == model.NetworkStatic {
		fmt.Fprintf(&b, " — %s via %s", cfg.Network.Address, cfg.Network.Gateway)
	}
	fmt.Fprintf(&b, "\nHostname: %s", cfg.Hostname)
	if len(cfg.Users) > 0 {
		fmt.Fprintf(&b, "\nUser: %s", cfg.Users[0].Username)
	}
	if len(cfg.SSHKeys) > 0 {
		fmt.Fprintf(&b, "\nSSH Keys: %d key(s)", len(cfg.SSHKeys))
	}
	if len(cfg.Sysexts) > 0 {
		names := make([]string, len(cfg.Sysexts))
		for i, s := range cfg.Sysexts {
			names[i] = s.Name
		}
		fmt.Fprintf(&b, "\nSysexts: %s", strings.Join(names, ", "))
	}
	return b.String()
}

// renderProgressBar returns the breadcrumb (used by non-form step views).
func (m *Model) renderProgressBar() string {
	return m.buildBreadcrumb()
}

// buildBreadcrumb creates a conversational breadcrumb showing decisions made.
// e.g. "knuckle › stable › Samsung 860 › core@flatcar"
func (m *Model) buildBreadcrumb() string {
	breadcrumbStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true)

	parts := []string{accentStyle.Render("knuckle")}

	cfg := &m.Wizard.State.Config
	step := m.Wizard.State.CurrentStep

	// Show prior decisions as breadcrumb trail
	if step > model.StepWelcome && cfg.Channel != "" {
		parts = append(parts, cfg.Channel)
	}
	if step > model.StepStorage && cfg.Disk.DevPath != "" {
		disk := cfg.Disk.Model
		if disk == "" {
			disk = cfg.Disk.DevPath
		}
		parts = append(parts, disk)
	}
	if step > model.StepUser && len(cfg.Users) > 0 {
		user := cfg.Users[0].Username
		if cfg.Hostname != "" {
			user = user + "@" + cfg.Hostname
		}
		parts = append(parts, user)
	}

	return breadcrumbStyle.Render(strings.Join(parts, " › ")) + "\n"
}

// renderSystemChecks returns system check output.
// Shows one-liner if all pass, detailed view if any warn/fail.
func (m *Model) renderSystemChecks() string {
	if len(m.Wizard.State.SystemChecks) == 0 {
		return ""
	}

	allOk := true
	for _, check := range m.Wizard.State.SystemChecks {
		if check.Status != "ok" {
			allOk = false
			break
		}
	}

	var b strings.Builder
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	failStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	if allOk {
		b.WriteString(okStyle.Render("  ✓ System ready"))
		details := make([]string, 0, len(m.Wizard.State.SystemChecks))
		for _, check := range m.Wizard.State.SystemChecks {
			details = append(details, check.Name)
		}
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(
			" (" + strings.Join(details, ", ") + ")"))
		b.WriteString("\n")
	} else {
		for _, check := range m.Wizard.State.SystemChecks {
			var style lipgloss.Style
			var icon string
			switch check.Status {
			case "ok":
				style = okStyle
				icon = "✓"
			case "warn":
				style = warnStyle
				icon = "⚠"
			default:
				style = failStyle
				icon = "✗"
			}
			b.WriteString(style.Render(fmt.Sprintf("  %s %s: %s", icon, check.Name, check.Detail)))
			b.WriteString("\n")
		}
	}

	return b.String()
}

// viewWelcomeHeader is kept for backward compatibility but now minimal.
// The heavy channel version display has been removed from the default view.
func (m *Model) viewWelcomeHeader() string {
	return m.buildBreadcrumb()
}
