package tui

import "testing"

func TestMergeKeysVariadic(t *testing.T) {
	a := []string{"ssh-ed25519 AAAA local@host"}
	b := []string{"ssh-rsa BBBB manual@paste"}
	c := []string{"ssh-ed25519 CCCC github@user", "ssh-ed25519 AAAA local@host"} // dup

	result := mergeKeys(a, b, c)

	if len(result) != 3 {
		t.Fatalf("expected 3 unique keys, got %d: %v", len(result), result)
	}
	// Order: local, manual, github (deduped)
	want := []string{
		"ssh-ed25519 AAAA local@host",
		"ssh-rsa BBBB manual@paste",
		"ssh-ed25519 CCCC github@user",
	}
	for i, k := range want {
		if result[i] != k {
			t.Errorf("result[%d] = %q, want %q", i, result[i], k)
		}
	}
}

func TestMergeKeysPreservesManualOnGitHubFetch(t *testing.T) {
	// Simulates what happens when fetchKeysMsg arrives:
	// local keys + manual pasted keys + GitHub keys should all be preserved
	localKeys := []string{"ssh-ed25519 AAAA local@host"}
	manualKeys := splitSSHKeys("ssh-rsa BBBB manual@paste;ssh-ed25519 DDDD second@manual")
	githubKeys := []string{"ssh-ed25519 CCCC github@user"}

	result := mergeKeys(localKeys, manualKeys, githubKeys)

	if len(result) != 4 {
		t.Fatalf("expected 4 keys, got %d: %v", len(result), result)
	}

	// Verify manual keys survived
	found := map[string]bool{}
	for _, k := range result {
		found[k] = true
	}
	for _, mk := range manualKeys {
		if !found[mk] {
			t.Errorf("manual key %q was dropped after GitHub fetch", mk)
		}
	}
}
