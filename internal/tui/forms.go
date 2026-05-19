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
	).WithTheme(huh.ThemeDracula()).WithShowHelp(true)
}

// buildNetworkForm creates the huh form for the Network step.
func (m *Model) buildNetworkForm() *huh.Form {
	// Show detected interfaces in the description
	ifaceDesc := "Network interface for static config"
	if len(m.Wizard.State.Interfaces) > 0 {
		var names []string
		for _, iface := range m.Wizard.State.Interfaces {
			names = append(names, iface.Name)
		}
		ifaceDesc = fmt.Sprintf("Detected: %s", strings.Join(names, ", "))
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Network Configuration").
				Description("Configure networking for this machine.\nLeave all fields blank for DHCP (recommended for most setups).\n\nCommon static configurations:\n  • Home server: 192.168.1.100/24, gateway 192.168.1.1, DNS 1.1.1.1\n  • Office/VLAN: 10.0.1.50/24, gateway 10.0.1.1, DNS 10.0.1.1\n  • Lab network: 172.16.0.10/16, gateway 172.16.0.1, DNS 8.8.8.8"),
			huh.NewInput().
				Title("Interface").
				Description(ifaceDesc).
				Placeholder("eth0").
				Value(&m.Wizard.State.Config.Network.Interface),
			huh.NewInput().
				Title("IP Address (CIDR)").
				Description("Include subnet mask, e.g. /24 = 255.255.255.0").
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
				Description("Comma-separated. Common: 1.1.1.1 (Cloudflare), 8.8.8.8 (Google)").
				Placeholder("1.1.1.1,8.8.8.8").
				Value(&m.dnsInput),
		),
	).WithTheme(huh.ThemeDracula()).WithShowHelp(true)
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
				Placeholder("castrojo").
				Value(&m.githubUserInput),
			huh.NewInput().
				Title("SSH Public Key").
				Description("Or paste key directly (separate multiple with ;)").
				Value(&m.sshKeyInput),
		),
	).WithTheme(huh.ThemeDracula()).WithShowHelp(true)
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
	).WithTheme(huh.ThemeDracula()).WithShowHelp(true)
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

// renderProgressBar creates a visual step indicator for the wizard.
func (m *Model) renderProgressBar() string {
	steps := []string{"Channel", "Network", "Disk", "User", "Sysext", "Update", "Review", "Install"}
	current := int(m.Wizard.State.CurrentStep)

	var parts []string
	for i, name := range steps {
		if i < current {
			// Completed
			parts = append(parts, lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).
				Render("✓ "+name))
		} else if i == current {
			// Current
			parts = append(parts, lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("213")).
				Render("● "+name))
		} else {
			// Future
			parts = append(parts, lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Render("○ "+name))
		}
	}
	return strings.Join(parts, "  ") + "\n"
}

// viewWelcomeHeader renders system info above the form.
func (m *Model) viewWelcomeHeader() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("63")).
		MarginBottom(1)

	b.WriteString(headerStyle.Render("🔧 Knuckle — Flatcar Container Linux Installer"))
	b.WriteString("\n\n")

	// System checks
	if len(m.Wizard.State.SystemChecks) > 0 {
		for _, check := range m.Wizard.State.SystemChecks {
			icon := "✓"
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
			if check.Status == "warn" {
				icon = "⚠"
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
			} else if check.Status == "fail" {
				icon = "✗"
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
			}
			b.WriteString(style.Render(fmt.Sprintf("  %s %s: %s", icon, check.Name, check.Detail)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Channel versions
	if len(m.Wizard.State.Channels) > 0 {
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		verifiedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
		for _, ch := range m.Wizard.State.Channels {
			// Verification indicator
			verifyIcon := "⚠️" // unverified
			if ch.DigestVerified && ch.SignedDigest {
				verifyIcon = verifiedStyle.Render("🔒") // fully verified
			} else if ch.SBOMVerified {
				verifyIcon = "🔓" // SBOM parsed but not signature-verified
			}
			fmt.Fprintf(&b, "  %s %s — Flatcar %s\n", verifyIcon, ch.Channel, ch.Version)
			b.WriteString(dimStyle.Render(fmt.Sprintf("    kernel %s · systemd %s · docker %s",
				ch.Kernel, ch.Systemd, ch.Docker)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		// Legend
		b.WriteString(dimStyle.Render("  🔒 = SBOM verified + signed digest  🔓 = SBOM only  ⚠️ = unverified"))
		b.WriteString("\n\n")
	}

	return b.String()
}
