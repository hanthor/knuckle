// Package ignition generates Butane YAML configs from InstallConfig.
// The generated YAML is Flatcar variant, spec 1.1.0, compiled to Ignition
// JSON via the coreos/butane Go library (not the CLI, which isn't on Flatcar).
package ignition

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/projectbluefin/knuckle/internal/model"
)

// Generator produces Butane YAML configs from InstallConfig.
type Generator struct{}

// NewGenerator returns a new Generator.
func NewGenerator() *Generator {
	return &Generator{}
}

// GenerateButane produces a Butane YAML config string from the given InstallConfig.
// The output is Flatcar variant, spec 1.1.0.
func (g *Generator) GenerateButane(cfg *model.InstallConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("config cannot be nil")
	}

	funcMap := template.FuncMap{
		"isStatic": func(n model.NetworkConfig) bool {
			return n.Mode == model.NetworkStatic
		},
		"yamlEscape": func(s string) string {
			s = strings.ReplaceAll(s, `\`, `\\`)
			s = strings.ReplaceAll(s, `"`, `\"`)
			s = strings.ReplaceAll(s, "\n", `\n`)
			s = strings.ReplaceAll(s, "\r", `\r`)
			s = strings.ReplaceAll(s, "\t", `\t`)
			return s
		},
	}

	tmpl, err := template.New("butane").Funcs(funcMap).Parse(butaneTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	channel := cfg.Channel
	if channel == "" {
		channel = "stable"
	}

	rebootStrategy := cfg.UpdateStrategy.RebootStrategy
	if rebootStrategy == "" {
		rebootStrategy = "reboot"
	}

	hasPassword := false
	for _, u := range cfg.Users {
		if u.PasswordHash != "" {
			hasPassword = true
			break
		}
	}

	data := templateData{
		Hostname:            cfg.Hostname,
		Timezone:            cfg.Timezone,
		Users:               cfg.Users,
		SSHKeys:             cfg.SSHKeys,
		Network:             cfg.Network,
		Sysexts:             filterSelected(cfg.Sysexts),
		Channel:             channel,
		RebootStrategy:      rebootStrategy,
		HasPassword:         hasPassword,
		NvidiaDriverVersion: cfg.NvidiaDriverVersion,
		Tailscale:           cfg.Tailscale,
		TailscaleEnabled:    cfg.Tailscale.AuthKey != "",
		TailscaleForwarding: cfg.Tailscale.AuthKey != "" && (cfg.Tailscale.Mode == model.TailscaleModeExitNode || cfg.Tailscale.Mode == model.TailscaleModeSubnetRouter),
		TailscaleExtraArgs:  tailscaleExtraArgs(cfg.Tailscale),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

type templateData struct {
	Hostname            string
	Timezone            string
	Users               []model.UserConfig
	SSHKeys             []string
	Network             model.NetworkConfig
	Sysexts             []model.SysextEntry
	Channel             string
	RebootStrategy      string
	HasPassword         bool
	NvidiaDriverVersion string // e.g. "570-open"; empty = no NVIDIA kernel driver setup
	Tailscale           model.TailscaleConfig
	TailscaleEnabled    bool   // AuthKey is set, i.e. user filled the step
	TailscaleForwarding bool   // exit-node or subnet-router → need sysctl ip_forward=1
	TailscaleExtraArgs  string // value of TS_EXTRA_ARGS in tailscale.env
}

// tailscaleExtraArgs builds the TS_EXTRA_ARGS value for /etc/tailscale/tailscale.env
// based on the selected mode.
func tailscaleExtraArgs(ts model.TailscaleConfig) string {
	switch ts.Mode {
	case model.TailscaleModeExitNode:
		return "--advertise-exit-node"
	case model.TailscaleModeSubnetRouter:
		routes := strings.ReplaceAll(strings.TrimSpace(ts.Routes), " ", "")
		if routes == "" {
			return ""
		}
		return "--advertise-routes=" + routes
	default:
		return ""
	}
}

func filterSelected(sysexts []model.SysextEntry) []model.SysextEntry {
	var selected []model.SysextEntry
	for _, s := range sysexts {
		if s.Selected {
			selected = append(selected, s)
		}
	}
	return selected
}

const butaneTemplate = `variant: flatcar
version: 1.1.0
storage:
  files:
    - path: /etc/hostname
      mode: 0644
      overwrite: true
      contents:
        inline: "{{.Hostname | yamlEscape}}"
    - path: /etc/flatcar/update.conf
      mode: 0644
      overwrite: true
      contents:
        inline: |
          REBOOT_STRATEGY={{.RebootStrategy}}
          GROUP={{.Channel}}
    - path: /etc/ssh/sshd_config.d/99-knuckle-hardening.conf
      mode: 0600
      overwrite: true
      contents:
        inline: |
          PasswordAuthentication {{if .HasPassword}}yes{{else}}no{{end}}
          PermitRootLogin no
          PubkeyAuthentication yes
{{- if isStatic .Network}}
    - path: /etc/systemd/network/10-static.network
      mode: 0644
      contents:
        inline: |
          [Match]
          Name={{.Network.Interface}}

          [Network]
          Address={{.Network.Address}}
          Gateway={{.Network.Gateway}}
{{- range .Network.DNS}}
          DNS={{.}}
{{- end}}
{{- end}}
{{- range .Sysexts}}
    - path: /etc/extensions/{{.Name}}.raw
      contents:
        source: "{{.URL | yamlEscape}}"
{{- if .Sha256}}
        verification:
          hash: "sha256-{{.Sha256}}"
{{- end}}
{{- end}}
{{- if .NvidiaDriverVersion}}
    - path: /etc/flatcar/enabled-sysext.conf
      mode: 0644
      overwrite: true
      contents:
        inline: |
          nvidia-drivers-{{.NvidiaDriverVersion | yamlEscape}}
{{- end}}
{{- if .TailscaleEnabled}}
    - path: /etc/tailscale/tailscale.env
      mode: 0600
      overwrite: true
      contents:
        inline: |
          TS_AUTHKEY={{.Tailscale.AuthKey | yamlEscape}}
          TS_AUTH_ONCE=true
{{- if .TailscaleExtraArgs}}
          TS_EXTRA_ARGS={{.TailscaleExtraArgs | yamlEscape}}
{{- end}}
{{- if .TailscaleForwarding}}
    - path: /etc/sysctl.d/99-tailscale.conf
      mode: 0644
      overwrite: true
      contents:
        inline: |
          net.ipv4.ip_forward = 1
          net.ipv6.conf.all.forwarding = 1
{{- end}}
{{- end}}
{{- if .Timezone}}
  links:
    - path: /etc/localtime
      target: "/usr/share/zoneinfo/{{.Timezone}}"
      overwrite: true
{{- end}}
systemd:
  units:
{{- if .Sysexts}}
    - name: systemd-sysext.service
      enabled: true
{{- end}}
    - name: update-engine.service
      enabled: true
{{- if .TailscaleEnabled}}
    - name: tailscaled.service
      enabled: true
    - name: knuckle-tailscale-up.service
      enabled: true
      contents: |
        [Unit]
        Description=Bring up Tailscale with the auth key provisioned by knuckle
        Requires=tailscaled.service network-online.target
        After=tailscaled.service network-online.target
        ConditionPathExists=/etc/tailscale/tailscale.env
        ConditionPathExists=!/var/lib/tailscale/.knuckle-up-done

        [Service]
        Type=oneshot
        EnvironmentFile=/etc/tailscale/tailscale.env
        ExecStart=/usr/bin/tailscale up --auth-key=$${TS_AUTHKEY} $${TS_EXTRA_ARGS}
        ExecStartPost=/bin/sh -c 'install -m 0600 /dev/null /var/lib/tailscale/.knuckle-up-done'
        RemainAfterExit=yes

        [Install]
        WantedBy=multi-user.target
{{- end}}
passwd:
  users:
{{- if .Users}}
{{- range .Users}}
    - name: "{{.Username | yamlEscape}}"
{{- if .Groups}}
      groups:
{{- range .Groups}}
        - "{{. | yamlEscape}}"
{{- end}}
{{- end}}
{{- if .SSHKeys}}
      ssh_authorized_keys:
{{- range .SSHKeys}}
        - "{{. | yamlEscape}}"
{{- end}}
{{- end}}
{{- if .PasswordHash}}
      password_hash: "{{.PasswordHash | yamlEscape}}"
{{- end}}
{{- end}}
{{- else}}
    - name: "core"
{{- if .SSHKeys}}
      ssh_authorized_keys:
{{- range .SSHKeys}}
        - "{{. | yamlEscape}}"
{{- end}}
{{- end}}
{{- end}}
`
