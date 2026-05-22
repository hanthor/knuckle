package ignition

import (
	"strings"
	"testing"

	"github.com/projectbluefin/knuckle/internal/model"
)

func TestBuilderEmptyDocumentIsValid(t *testing.T) {
	b := NewBuilder(&model.InstallConfig{})
	got := b.Build()
	wantPrefix := "variant: flatcar\nversion: 1.1.0\n"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("expected document to start with %q, got %q", wantPrefix, got)
	}
	if strings.Contains(got, "storage:") {
		t.Error("empty builder must not emit a storage: block")
	}
	if strings.Contains(got, "systemd:") {
		t.Error("empty builder must not emit a systemd: block")
	}
}

func TestBuilderStorageSection(t *testing.T) {
	b := NewBuilder(&model.InstallConfig{})
	b.AddStorageFile(`- path: /etc/hostname
  mode: 0644
  contents:
    inline: "demo"`)
	got := b.Build()
	if !strings.Contains(got, "storage:\n  files:\n    - path: /etc/hostname") {
		t.Errorf("expected storage.files block with /etc/hostname, got:\n%s", got)
	}
	if !strings.Contains(got, "      mode: 0644") {
		t.Errorf("expected mode key indented under file entry, got:\n%s", got)
	}
}

func TestBuilderSystemdSection(t *testing.T) {
	b := NewBuilder(&model.InstallConfig{})
	b.AddSystemdUnit(`- name: foo.service
  enabled: true`)
	got := b.Build()
	if !strings.Contains(got, "systemd:\n  units:\n    - name: foo.service") {
		t.Errorf("expected systemd.units block, got:\n%s", got)
	}
}

func TestBuilderPasswdSection(t *testing.T) {
	b := NewBuilder(&model.InstallConfig{})
	b.SetPasswdUsers(`- name: "core"
  ssh_authorized_keys:
    - "ssh-ed25519 AAAA"`)
	got := b.Build()
	if !strings.Contains(got, "passwd:\n  users:\n    - name: \"core\"") {
		t.Errorf("expected passwd.users block, got:\n%s", got)
	}
}

func TestBuilderRenderTemplate(t *testing.T) {
	out, err := renderTemplate("hostname", `- path: /etc/hostname
  contents:
    inline: "{{.Hostname | yamlEscape}}"`, struct{ Hostname string }{Hostname: `name"with\quotes`})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if !strings.Contains(out, `name\"with\\quotes`) {
		t.Errorf("expected escaped quotes/backslashes, got:\n%s", out)
	}
}

func TestBuilderEndToEndProducesValidButane(t *testing.T) {
	// Build a minimal document via the Builder and then run it through the
	// real CompileToIgnition path to prove the builder output is real
	// Butane that the compiler accepts. This is the regression guard for
	// future section migrations.
	b := NewBuilder(&model.InstallConfig{})
	b.AddStorageFile(`- path: /etc/hostname
  mode: 0644
  overwrite: true
  contents:
    inline: "builder-demo"`)
	b.SetPasswdUsers(`- name: "core"
  ssh_authorized_keys:
    - "ssh-ed25519 AAAA demo@key"`)
	doc := b.Build()

	ign, err := CompileToIgnition(doc)
	if err != nil {
		t.Fatalf("builder output failed butane compile:\n--- doc ---\n%s\n--- err ---\n%v", doc, err)
	}
	if !strings.Contains(ign, "/etc/hostname") {
		t.Errorf("compiled Ignition missing /etc/hostname entry, got:\n%s", ign)
	}
}
