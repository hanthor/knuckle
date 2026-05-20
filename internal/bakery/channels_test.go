package bakery

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

const mockVersionTxt = `FLATCAR_VERSION=4593.2.0
FLATCAR_BUILD_ID="2026-04-14-0806"
GROUP=stable
`

const mockPackageList = `app-misc/some-tool-1.2.3::portage-stable
sys-kernel/coreos-kernel-6.12.81::coreos-overlay
sys-apps/systemd-257.9::portage-stable
net-misc/curl-8.5.0::portage-stable
`

func TestFetchChannelInfo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/version.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockVersionTxt))
	})
	mux.HandleFunc("/packages.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockPackageList))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	info, err := fetchChannelInfoFromURLs(context.Background(), "stable", srv.URL+"/version.txt", srv.URL+"/packages.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Channel != "stable" {
		t.Errorf("channel: got %q, want %q", info.Channel, "stable")
	}
	if info.Version != "4593.2.0" {
		t.Errorf("version: got %q, want %q", info.Version, "4593.2.0")
	}
	if info.BuildDate != "2026-04-14" {
		t.Errorf("build date: got %q, want %q", info.BuildDate, "2026-04-14")
	}
	if info.Kernel != "6.12.81" {
		t.Errorf("kernel: got %q, want %q", info.Kernel, "6.12.81")
	}
	if info.Systemd != "257.9" {
		t.Errorf("systemd: got %q, want %q", info.Systemd, "257.9")
	}
}

func TestFetchAllChannels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/version.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockVersionTxt))
	})
	mux.HandleFunc("/packages.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockPackageList))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	results, err := fetchAllChannelsWithURLFn(context.Background(), func(channel string) (string, string) {
		return srv.URL + "/version.txt", srv.URL + "/packages.txt"
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	expected := []string{"stable", "beta", "alpha", "lts"}
	for i, ch := range expected {
		if results[i].Channel != ch {
			t.Errorf("results[%d].Channel: got %q, want %q", i, results[i].Channel, ch)
		}
		if results[i].Version != "4593.2.0" {
			t.Errorf("results[%d].Version: got %q, want %q", i, results[i].Version, "4593.2.0")
		}
	}
}

func TestFetchChannelInfoHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fetchChannelInfoFromURLs(context.Background(), "stable", srv.URL+"/version.txt", srv.URL+"/packages.txt")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestParseVersionTxt(t *testing.T) {
	info := &ChannelInfo{}
	parseVersionTxt(mockVersionTxt, info)

	if info.Version != "4593.2.0" {
		t.Errorf("version: got %q, want %q", info.Version, "4593.2.0")
	}
	if info.BuildDate != "2026-04-14" {
		t.Errorf("build date: got %q, want %q", info.BuildDate, "2026-04-14")
	}
}

func TestParsePackageList(t *testing.T) {
	info := &ChannelInfo{}
	parsePackageList(mockPackageList, info)

	if info.Kernel != "6.12.81" {
		t.Errorf("kernel: got %q, want %q", info.Kernel, "6.12.81")
	}
	if info.Systemd != "257.9" {
		t.Errorf("systemd: got %q, want %q", info.Systemd, "257.9")
	}
}

func TestFetchChannelInfo_CancelledContext(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockVersionTxt))
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := fetchChannelInfoFromURLs(ctx, "stable", ts.URL+"/version.txt", ts.URL+"/packages.txt")
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

const mockSBOMJSON = `{
  "spdxVersion": "SPDX-2.3",
  "packages": [
    {"name": "sys-kernel/coreos-kernel", "versionInfo": "6.12.87"},
    {"name": "sys-apps/systemd", "versionInfo": "257.9"},
    {"name": "sys-apps/ignition", "versionInfo": "2.24.0-r1"},
    {"name": "dev-db/etcd", "versionInfo": "3.5.18"},
    {"name": "app-misc/unrelated", "versionInfo": "1.0.0"}
  ]
}`

func TestParseSBOMJSON(t *testing.T) {
	info := &ChannelInfo{}
	parseSBOMJSON(mockSBOMJSON, info)

	if info.Kernel != "6.12.87" {
		t.Errorf("kernel: got %q, want %q", info.Kernel, "6.12.87")
	}
	if info.Systemd != "257.9" {
		t.Errorf("systemd: got %q, want %q", info.Systemd, "257.9")
	}
	if info.Ignition != "2.24.0" {
		t.Errorf("ignition: got %q, want %q (should strip -rN)", info.Ignition, "2.24.0")
	}
	if info.Etcd != "3.5.18" {
		t.Errorf("etcd: got %q, want %q", info.Etcd, "3.5.18")
	}
}

func TestSBOMPreferredOverPackageList(t *testing.T) {
	// When SBOM JSON is available, it should be used instead of text package list.
	// The SBOM has different (newer) versions than the text file to prove priority.
	mux := http.NewServeMux()
	mux.HandleFunc("/version.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockVersionTxt))
	})
	mux.HandleFunc("/flatcar_production_image_packages.txt", func(w http.ResponseWriter, r *http.Request) {
		// Text file has OLDER versions — should NOT be used when SBOM available
		_, _ = w.Write([]byte("sys-kernel/coreos-kernel-5.0.0::coreos-overlay\nsys-apps/systemd-200.0::portage-stable\n"))
	})
	mux.HandleFunc("/flatcar_production_image_sbom.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockSBOMJSON))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	info, err := fetchChannelInfoFromURLs(context.Background(), "stable",
		srv.URL+"/version.txt",
		srv.URL+"/flatcar_production_image_packages.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// SBOM version should win over text file
	if info.Kernel != "6.12.87" {
		t.Errorf("SBOM should be preferred: kernel got %q, want %q", info.Kernel, "6.12.87")
	}
	if info.Systemd != "257.9" {
		t.Errorf("SBOM should be preferred: systemd got %q, want %q", info.Systemd, "257.9")
	}
}

func TestFallbackToPackageListWhenNoSBOM(t *testing.T) {
	// When SBOM returns 404, fall back to text package list
	mux := http.NewServeMux()
	mux.HandleFunc("/version.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockVersionTxt))
	})
	mux.HandleFunc("/flatcar_production_image_packages.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockPackageList))
	})
	mux.HandleFunc("/flatcar_production_image_sbom.json", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	info, err := fetchChannelInfoFromURLs(context.Background(), "stable",
		srv.URL+"/version.txt",
		srv.URL+"/flatcar_production_image_packages.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should get versions from text file fallback
	if info.Kernel != "6.12.81" {
		t.Errorf("fallback failed: kernel got %q, want %q", info.Kernel, "6.12.81")
	}
}

func TestVerifySHA512(t *testing.T) {
	content := "hello world"
	// Pre-computed SHA512 of "hello world"
	digest := "# SHA512 HASH\n309ecc489c12d6eb4cc40f50c902f2b4d0ed77ee511a7c7a9bcd3ca86d4cd86f989dd35bc5ff499670da34255b45b0cfd830e81f605dcf7dc5542e93ae9cd76f  test.txt\n"

	if !verifySHA512(content, digest) {
		t.Error("expected SHA512 verification to pass for known hash")
	}

	if verifySHA512("wrong content", digest) {
		t.Error("expected SHA512 verification to fail for wrong content")
	}

	if verifySHA512(content, "# SHA512 HASH\ndeadbeef  test.txt\n") {
		t.Error("expected SHA512 verification to fail for wrong hash")
	}
}

func TestVerificationStatusInChannelInfo(t *testing.T) {
	// When SBOM + digest are both served, verification flags should be set
	mux := http.NewServeMux()
	mux.HandleFunc("/version.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockVersionTxt))
	})
	sbomContent := mockSBOMJSON
	mux.HandleFunc("/flatcar_production_image_sbom.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sbomContent))
	})
	mux.HandleFunc("/flatcar_production_image_sbom.json.DIGESTS", func(w http.ResponseWriter, r *http.Request) {
		// Compute real SHA512 of the SBOM content
		hash := sha512Hash(sbomContent)
		_, _ = fmt.Fprintf(w, "# SHA512 HASH\n%s  flatcar_production_image_sbom.json\n", hash)
	})
	mux.HandleFunc("/flatcar_production_image_sbom.json.DIGESTS.asc", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("-----BEGIN PGP SIGNED MESSAGE-----\nfake\n"))
	})
	mux.HandleFunc("/flatcar_production_image_packages.txt", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not used", 404)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	info, err := fetchChannelInfoFromURLs(context.Background(), "stable",
		srv.URL+"/version.txt",
		srv.URL+"/flatcar_production_image_packages.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !info.SBOMVerified {
		t.Error("SBOMVerified should be true")
	}
	if !info.DigestVerified {
		t.Error("DigestVerified should be true")
	}
	if !info.SignedDigest {
		t.Error("SignedDigest should be true")
	}
}

func sha512Hash(content string) string {
	h := sha512.Sum512([]byte(content))
	return hex.EncodeToString(h[:])
}
