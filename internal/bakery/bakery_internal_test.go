package bakery

import (
	"strings"
	"testing"
)

func TestTruncateDescription_Short(t *testing.T) {
	if got := truncateDescription("hello", 80); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestTruncateDescription_TooLong(t *testing.T) {
	long := strings.Repeat("x", 100)
	got := truncateDescription(long, 80)
	if len(got) != 80 {
		t.Errorf("truncated len = %d, want 80", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncated string should end with ...: %q", got)
	}
}

func TestTruncateDescription_MultilineUsesFirstLine(t *testing.T) {
	got := truncateDescription("first line\nsecond line\nthird", 80)
	if got != "first line" {
		t.Errorf("got %q, want %q", got, "first line")
	}
}

func TestTruncateDescription_Empty(t *testing.T) {
	if got := truncateDescription("", 80); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// ── parseLinkNext: missing branches ──────────────────────────────────────────

func TestParseLinkNext_NoSemicolon_Skipped(t *testing.T) {
	// A part with no semicolon has len(segments) < 2 → continue; the
	// function then exhausts all parts and returns ("", false).
	url, ok := parseLinkNext(`<https://api.github.com/page=2>`)
	if ok || url != "" {
		t.Errorf("parseLinkNext(no-semicolon) = (%q, %v), want (\"\", false)", url, ok)
	}
}

func TestParseLinkNext_NonEmpty_NoNext(t *testing.T) {
	// Non-empty header with rel="last" only — no rel="next" match → ("", false).
	url, ok := parseLinkNext(`<https://api.github.com/page=5>; rel="last"`)
	if ok || url != "" {
		t.Errorf("parseLinkNext(last-only) = (%q, %v), want (\"\", false)", url, ok)
	}
}
