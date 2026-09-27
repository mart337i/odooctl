package selfupdate

import (
	"fmt"
	"strings"
)

const Repository = "mart337i/odooctl"

type Method string

const (
	MethodAuto    Method = "auto"
	MethodAPT     Method = "apt"
	MethodRelease Method = "release"
	MethodGo      Method = "go"
	MethodSource  Method = "source"
)

type Options struct {
	CurrentVersion string
	MethodOverride string
	SourceDir      string
	Force          bool
}

type PlanResult struct {
	CurrentVersion  string   `json:"current_version"`
	LatestVersion   string   `json:"latest_version"`
	UpdateAvailable bool     `json:"update_available"`
	Method          Method   `json:"method"`
	Executable      string   `json:"executable"`
	SourceDir       string   `json:"source_dir,omitempty"`
	Asset           string   `json:"asset,omitempty"`
	AssetURL        string   `json:"asset_url,omitempty"`
	ChecksumURL     string   `json:"checksum_url,omitempty"`
	Commands        []string `json:"commands,omitempty"`
	RequiresForce   bool     `json:"requires_force,omitempty"`
	Warning         string   `json:"warning,omitempty"`
}

func (p PlanResult) Summary() string {
	parts := []string{
		fmt.Sprintf("Current: %s", p.CurrentVersion),
		fmt.Sprintf("Latest:  %s", p.LatestVersion),
		fmt.Sprintf("Method:  %s", p.Method),
	}
	if p.Warning != "" {
		parts = append(parts, "Warning: "+p.Warning)
	}
	if !p.UpdateAvailable {
		parts = append(parts, "odooctl is already up to date")
		return strings.Join(parts, "\n")
	}
	if len(p.Commands) > 0 {
		parts = append(parts, "Commands:")
		for _, command := range p.Commands {
			parts = append(parts, "  "+command)
		}
	} else if p.Asset != "" {
		parts = append(parts, "Asset:   "+p.Asset)
	}
	return strings.Join(parts, "\n")
}
