package tui

import (
	"strings"
	"testing"
)

func TestRenderTierHeaderEmptyString(t *testing.T) {
	header := renderTierHeader("")
	if !strings.Contains(header, "Other") {
		t.Errorf("empty tier should default to 'Other', got: %q", header)
	}
	if !strings.Contains(header, "──") {
		t.Error("header should contain dashes for styling")
	}
}

func TestRenderTierHeaderIntegrated(t *testing.T) {
	header := renderTierHeader("Integrated")
	if !strings.Contains(header, "Integrated") {
		t.Errorf("header should contain tier name, got: %q", header)
	}
	if !strings.Contains(header, "──") {
		t.Error("header should contain dashes for styling")
	}
}

func TestRenderTierHeaderMaintained(t *testing.T) {
	header := renderTierHeader("Maintained")
	if !strings.Contains(header, "Maintained") {
		t.Errorf("header should contain 'Maintained', got: %q", header)
	}
}

func TestRenderTierHeaderExperimental(t *testing.T) {
	header := renderTierHeader("Experimental")
	if !strings.Contains(header, "Experimental") {
		t.Errorf("header should contain 'Experimental', got: %q", header)
	}
}
