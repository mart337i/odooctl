package selfupdate

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeReleaseClient struct{ release Release }

func (f fakeReleaseClient) LatestRelease() (Release, error) { return f.release, nil }

func testRelease() Release {
	return Release{
		TagName: "v0.2.6",
		Assets: []ReleaseAsset{
			{Name: "odooctl-linux-amd64", URL: "https://example.test/linux-amd64"},
			{Name: "odooctl-linux-arm64", URL: "https://example.test/linux-arm64"},
			{Name: "odooctl-darwin-amd64", URL: "https://example.test/darwin-amd64"},
			{Name: "odooctl-darwin-arm64", URL: "https://example.test/darwin-arm64"},
			{Name: "odooctl-windows-amd64.exe", URL: "https://example.test/windows-amd64.exe"},
			{Name: "checksums.txt", URL: "https://example.test/checksums.txt"},
		},
	}
}

func TestAssetName(t *testing.T) {
	tests := []struct {
		goos   string
		goarch string
		want   string
	}{
		{goos: "linux", goarch: "amd64", want: "odooctl-linux-amd64"},
		{goos: "linux", goarch: "arm64", want: "odooctl-linux-arm64"},
		{goos: "darwin", goarch: "amd64", want: "odooctl-darwin-amd64"},
		{goos: "darwin", goarch: "arm64", want: "odooctl-darwin-arm64"},
		{goos: "windows", goarch: "amd64", want: "odooctl-windows-amd64.exe"},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			got, err := assetName(tt.goos, tt.goarch)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("assetName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlanDetectsAPT(t *testing.T) {
	plan, err := planWithRuntime(Options{CurrentVersion: "0.2.5"}, fakeReleaseClient{release: testRelease()}, plannerRuntime{
		GOOS:       "linux",
		GOARCH:     "amd64",
		Executable: "/usr/bin/odooctl",
		RunOutput: func(name string, args ...string) (string, error) {
			if name == "dpkg-query" {
				return "odooctl: /usr/bin/odooctl", nil
			}
			return "", fmt.Errorf("unexpected command")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Method != MethodAPT {
		t.Fatalf("Method = %q, want apt", plan.Method)
	}
	if len(plan.Commands) != 2 || plan.Commands[1] != "sudo apt install --only-upgrade odooctl" {
		t.Fatalf("Commands = %v", plan.Commands)
	}
}

func TestPlanDetectsGoInstall(t *testing.T) {
	plan, err := planWithRuntime(Options{CurrentVersion: "0.2.5"}, fakeReleaseClient{release: testRelease()}, plannerRuntime{
		GOOS:       "linux",
		GOARCH:     "amd64",
		Executable: "/home/user/go/bin/odooctl",
		RunOutput:  func(name string, args ...string) (string, error) { return "", fmt.Errorf("not found") },
		Getenv: func(key string) string {
			if key == "GOPATH" {
				return "/home/user/go"
			}
			return ""
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Method != MethodGo {
		t.Fatalf("Method = %q, want go", plan.Method)
	}
	if len(plan.Commands) != 1 || plan.Commands[0] != "go install github.com/mart337i/odooctl@latest" {
		t.Fatalf("Commands = %v", plan.Commands)
	}
}

func TestPlanDetectsSourceCheckout(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"))
	writeTestFile(t, filepath.Join(root, "Makefile"))
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "bin", "odooctl")
	if err := os.MkdirAll(filepath.Dir(exe), 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, exe)

	plan, err := planWithRuntime(Options{CurrentVersion: "0.2.5"}, fakeReleaseClient{release: testRelease()}, plannerRuntime{
		GOOS:       "linux",
		GOARCH:     "amd64",
		Executable: exe,
		RunOutput:  func(name string, args ...string) (string, error) { return "", fmt.Errorf("not found") },
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Method != MethodSource || plan.SourceDir != root {
		t.Fatalf("Method/SourceDir = %q/%q, want source/%q", plan.Method, plan.SourceDir, root)
	}
}

func TestPlanReleaseOnWindows(t *testing.T) {
	plan, err := planWithRuntime(Options{CurrentVersion: "0.2.5"}, fakeReleaseClient{release: testRelease()}, plannerRuntime{
		GOOS:       "windows",
		GOARCH:     "amd64",
		Executable: `C:\Users\me\bin\odooctl.exe`,
		RunOutput:  func(name string, args ...string) (string, error) { return "", fmt.Errorf("not found") },
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Method != MethodRelease || plan.Asset != "odooctl-windows-amd64.exe" {
		t.Fatalf("Method/Asset = %q/%q", plan.Method, plan.Asset)
	}
}

func TestPlanReleaseWithoutChecksumWarns(t *testing.T) {
	release := testRelease()
	release.Assets = release.Assets[:len(release.Assets)-1]
	plan, err := planWithRuntime(Options{CurrentVersion: "0.2.5", MethodOverride: "release"}, fakeReleaseClient{release: release}, plannerRuntime{
		GOOS:       "linux",
		GOARCH:     "amd64",
		Executable: "/tmp/odooctl",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ChecksumURL != "" || !strings.Contains(plan.Warning, "checksum verification will be skipped") {
		t.Fatalf("ChecksumURL/Warning = %q/%q", plan.ChecksumURL, plan.Warning)
	}
}

func TestPlanRejectsUnknownMethod(t *testing.T) {
	_, err := planWithRuntime(Options{CurrentVersion: "0.2.5", MethodOverride: "brew"}, fakeReleaseClient{release: testRelease()}, plannerRuntime{GOOS: "linux", GOARCH: "amd64", Executable: "/tmp/odooctl"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPlanMarksNonReleaseVersionAsRequiringForce(t *testing.T) {
	plan, err := planWithRuntime(Options{CurrentVersion: "dev", MethodOverride: "release"}, fakeReleaseClient{release: testRelease()}, plannerRuntime{GOOS: "linux", GOARCH: "amd64", Executable: "/tmp/odooctl"})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.RequiresForce || plan.Warning == "" {
		t.Fatalf("RequiresForce/Warning = %v/%q", plan.RequiresForce, plan.Warning)
	}
}

func TestPlanForceAllowsNonReleaseVersion(t *testing.T) {
	plan, err := planWithRuntime(Options{CurrentVersion: "dev", MethodOverride: "release", Force: true}, fakeReleaseClient{release: testRelease()}, plannerRuntime{GOOS: "linux", GOARCH: "amd64", Executable: "/tmp/odooctl"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresForce || !strings.Contains(plan.Warning, "--force was provided") {
		t.Fatalf("RequiresForce/Warning = %v/%q", plan.RequiresForce, plan.Warning)
	}
}

func TestChecksumForAsset(t *testing.T) {
	data := []byte("abc123  odooctl-linux-amd64\n")
	checksum, ok := checksumForAsset(data, "odooctl-linux-amd64")
	if !ok || checksum != "abc123" {
		t.Fatalf("checksumForAsset() = %q/%v", checksum, ok)
	}
}

func TestVerifyChecksum(t *testing.T) {
	content := []byte("binary")
	sum := sha256.Sum256(content)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%x  odooctl-linux-amd64\n", sum)
	}))
	defer server.Close()

	if err := verifyChecksum(strings.NewReader(string(content)), server.URL, "odooctl-linux-amd64"); err != nil {
		t.Fatal(err)
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
}
