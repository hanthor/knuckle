package main

import (
	"testing"

	"github.com/projectbluefin/knuckle/internal/model"
)

func TestCheckCatalog_AllCovered(t *testing.T) {
	// "docker" is a well-known entry in descriptions.go.
	entries := []model.SysextEntry{
		{Name: "docker", Version: "28.0.0", URL: "https://example.com/docker.raw"},
	}
	covered, missing := checkCatalog(entries)
	if covered != 1 {
		t.Errorf("covered = %d, want 1", covered)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v, want empty", missing)
	}
}

func TestCheckCatalog_NoneKnown(t *testing.T) {
	entries := []model.SysextEntry{
		{Name: "totally-unknown-sysext-xyz", Version: "1.0.0", URL: "https://example.com/x.raw"},
		{Name: "another-mystery-ext-abc", Version: "2.0.0", URL: "https://example.com/y.raw"},
	}
	covered, missing := checkCatalog(entries)
	if covered != 0 {
		t.Errorf("covered = %d, want 0", covered)
	}
	if len(missing) != 2 {
		t.Fatalf("missing count = %d, want 2", len(missing))
	}
	if missing[0].Name != "totally-unknown-sysext-xyz" {
		t.Errorf("missing[0].Name = %q, want totally-unknown-sysext-xyz", missing[0].Name)
	}
	if missing[0].URL != "https://example.com/x.raw" {
		t.Errorf("missing[0].URL = %q, want https://example.com/x.raw", missing[0].URL)
	}
}

func TestCheckCatalog_Mixed(t *testing.T) {
	entries := []model.SysextEntry{
		{Name: "docker", Version: "28.0.0", URL: "https://example.com/docker.raw"},
		{Name: "unknown-ext", Version: "1.0.0", URL: "https://example.com/unknown.raw"},
		{Name: "tailscale", Version: "1.56.1", URL: "https://example.com/tailscale.raw"},
	}
	covered, missing := checkCatalog(entries)
	if covered != 2 {
		t.Errorf("covered = %d, want 2 (docker + tailscale)", covered)
	}
	if len(missing) != 1 {
		t.Fatalf("missing count = %d, want 1", len(missing))
	}
	if missing[0].Name != "unknown-ext" {
		t.Errorf("missing[0].Name = %q, want unknown-ext", missing[0].Name)
	}
}

func TestCheckCatalog_Empty(t *testing.T) {
	covered, missing := checkCatalog(nil)
	if covered != 0 || len(missing) != 0 {
		t.Errorf("empty catalog: covered=%d missing=%d, want both 0", covered, len(missing))
	}
}
