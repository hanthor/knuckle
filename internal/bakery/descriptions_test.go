package bakery_test

import (
	"testing"

	"github.com/castrojo/knuckle/internal/bakery"
)

// allKnownExtensions is the authoritative list of 28 Flatcar Bakery extensions.
// Update this list when the bakery adds or removes an extension.
var allKnownExtensions = []string{
	"bird", "btop", "chrony", "cilium", "consul", "containerd", "crio",
	"docker", "docker-buildx", "docker-compose", "falco", "ig", "k3s",
	"keepalived", "kubernetes", "llamaedge", "nebula", "nerdctl", "nomad",
	"nvidia-runtime", "ollama", "opkssh", "rke2", "tailscale", "tilde",
	"wasmcloud", "wasmedge", "wasmtime",
}

func TestAllExtensionsHaveDescriptions(t *testing.T) {
	for _, name := range allKnownExtensions {
		meta, ok := bakery.Lookup(name)
		if !ok {
			t.Errorf("extension %q not found in catalog", name)
			continue
		}
		if meta.Short == "" {
			t.Errorf("extension %q has empty Short description", name)
		}
		if meta.Long == "" {
			t.Errorf("extension %q has empty Long description", name)
		}
		if meta.Category == "" {
			t.Errorf("extension %q has empty Category", name)
		}
		if meta.SupportTier == "" {
			t.Errorf("extension %q has empty SupportTier", name)
		}
	}
}

func TestAllExtensionsSupportTierIsValid(t *testing.T) {
	validTiers := map[string]bool{
		bakery.TierIntegrated:   true,
		bakery.TierMaintained:   true,
		bakery.TierExperimental: true,
	}
	for _, name := range allKnownExtensions {
		meta, ok := bakery.Lookup(name)
		if !ok {
			continue // already flagged above
		}
		if !validTiers[meta.SupportTier] {
			t.Errorf("extension %q has unrecognised SupportTier %q", name, meta.SupportTier)
		}
	}
}

func TestCatalogCount(t *testing.T) {
	// Verify all known extensions are present. If new extensions are added to the
	// bakery catalog, add them to allKnownExtensions above and implement entries.
	for _, name := range allKnownExtensions {
		if _, ok := bakery.Lookup(name); !ok {
			t.Errorf("extension %q missing from catalog — add an entry to descriptions.go", name)
		}
	}
}

func TestLookupUnknown(t *testing.T) {
	_, ok := bakery.Lookup("nonexistent-extension-xyz")
	if ok {
		t.Error("Lookup should return ok=false for unknown extension")
	}
}

func TestCaveatsForKnownExtensionsWithCaveats(t *testing.T) {
	// These extensions must have non-nil caveats per upstream docs.
	mustHaveCaveats := []string{
		"nvidia-runtime", // no kernel module, no host CUDA, x86-64 only
		"llamaedge",      // requires matching wasmedge, not auto-built, manual start
		"kubernetes",     // minor-version sysupdate only
		"k3s",            // minor-version sysupdate only
		"rke2",           // minor-version sysupdate only
		"ollama",         // public API by default
	}
	for _, name := range mustHaveCaveats {
		caveats := bakery.CaveatsFor(name)
		if len(caveats) == 0 {
			t.Errorf("extension %q should have caveats but CaveatsFor returned nil/empty", name)
		}
	}
}

func TestCaveatsForUnknownExtension(t *testing.T) {
	caveats := bakery.CaveatsFor("nonexistent-xyz")
	if caveats != nil {
		t.Errorf("expected nil caveats for unknown extension, got %v", caveats)
	}
}

func TestNvidiaRuntimeCaveatsArePrecise(t *testing.T) {
	caveats := bakery.CaveatsFor("nvidia-runtime")
	if len(caveats) < 3 {
		t.Fatalf("nvidia-runtime should have at least 3 caveats (kernel, CUDA, arch), got %d", len(caveats))
	}
	// Verify the three critical facts are present somewhere in the caveats.
	checks := []struct {
		keyword string
		desc    string
	}{
		{"kernel", "kernel module caveat"},
		{"CUDA", "CUDA caveat"},
		{"x86-64", "architecture caveat"},
	}
	for _, check := range checks {
		found := false
		for _, c := range caveats {
			if contains(c, check.keyword) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("nvidia-runtime caveats missing %s (keyword %q not found in any caveat)", check.desc, check.keyword)
		}
	}
}

func TestTierConstants(t *testing.T) {
	if bakery.TierIntegrated == "" || bakery.TierMaintained == "" || bakery.TierExperimental == "" {
		t.Error("tier constants must not be empty strings")
	}
	// Tiers must be distinct.
	tiers := []string{bakery.TierIntegrated, bakery.TierMaintained, bakery.TierExperimental}
	seen := map[string]bool{}
	for _, tier := range tiers {
		if seen[tier] {
			t.Errorf("duplicate tier constant value: %q", tier)
		}
		seen[tier] = true
	}
}

// contains is a simple substring check for test assertions.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
