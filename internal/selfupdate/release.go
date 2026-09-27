package selfupdate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const latestReleaseURL = "https://api.github.com/repos/" + Repository + "/releases/latest"

type Release struct {
	TagName string         `json:"tag_name"`
	Assets  []ReleaseAsset `json:"assets"`
}

type ReleaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type releaseClient interface {
	LatestRelease() (Release, error)
}

type githubReleaseClient struct {
	URL        string
	HTTPClient *http.Client
}

func LatestRelease() (Release, error) {
	return githubReleaseClient{URL: latestReleaseURL, HTTPClient: http.DefaultClient}.LatestRelease()
}

func (c githubReleaseClient) LatestRelease() (Release, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	url := c.URL
	if url == "" {
		url = latestReleaseURL
	}
	resp, err := client.Get(url)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub release lookup failed: %s", resp.Status)
	}
	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return Release{}, err
	}
	if strings.TrimSpace(release.TagName) == "" {
		return Release{}, fmt.Errorf("latest release has no tag_name")
	}
	return release, nil
}

func (r Release) Asset(name string) (ReleaseAsset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return ReleaseAsset{}, false
}

func assetName(goos, goarch string) (string, error) {
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("unsupported architecture %s", goarch)
	}
	switch goos {
	case "linux", "darwin":
		return fmt.Sprintf("odooctl-%s-%s", goos, goarch), nil
	case "windows":
		if goarch != "amd64" {
			return "", fmt.Errorf("unsupported Windows architecture %s", goarch)
		}
		return "odooctl-windows-amd64.exe", nil
	default:
		return "", fmt.Errorf("unsupported OS %s", goos)
	}
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "odooctl ")
	version = strings.TrimPrefix(version, "v")
	return version
}

func isReleaseVersion(version string) bool {
	version = normalizeVersion(version)
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}
