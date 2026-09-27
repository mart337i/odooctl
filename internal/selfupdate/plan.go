package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type plannerRuntime struct {
	GOOS       string
	GOARCH     string
	Executable string
	RunOutput  func(name string, args ...string) (string, error)
	Getenv     func(string) string
}

func Plan(opts Options) (PlanResult, error) {
	exe, err := os.Executable()
	if err != nil {
		return PlanResult{}, err
	}
	rt := plannerRuntime{
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		Executable: exe,
		RunOutput:  commandOutput,
		Getenv:     os.Getenv,
	}
	return planWithRuntime(opts, githubReleaseClient{URL: latestReleaseURL}, rt)
}

func planWithRuntime(opts Options, releases releaseClient, rt plannerRuntime) (PlanResult, error) {
	if rt.RunOutput == nil {
		rt.RunOutput = commandOutput
	}
	if rt.Getenv == nil {
		rt.Getenv = os.Getenv
	}
	if rt.GOOS == "" {
		rt.GOOS = runtime.GOOS
	}
	if rt.GOARCH == "" {
		rt.GOARCH = runtime.GOARCH
	}
	if rt.Executable == "" {
		return PlanResult{}, fmt.Errorf("executable path cannot be empty")
	}

	release, err := releases.LatestRelease()
	if err != nil {
		return PlanResult{}, err
	}
	method, err := detectMethod(rt, opts)
	if err != nil {
		return PlanResult{}, err
	}

	current := normalizeVersion(opts.CurrentVersion)
	latest := normalizeVersion(release.TagName)
	plan := PlanResult{
		CurrentVersion:  opts.CurrentVersion,
		LatestVersion:   release.TagName,
		UpdateAvailable: current != latest,
		Method:          method,
		Executable:      rt.Executable,
	}

	if !isReleaseVersion(opts.CurrentVersion) {
		if opts.Force {
			plan.Warning = fmt.Sprintf("current version %q is not a release version; updating because --force was provided", opts.CurrentVersion)
		} else {
			plan.RequiresForce = true
			plan.Warning = fmt.Sprintf("current version %q is not a release version; use --force to update anyway", opts.CurrentVersion)
		}
	}

	switch method {
	case MethodAPT:
		plan.Commands = []string{"sudo apt update", "sudo apt install --only-upgrade odooctl"}
	case MethodGo:
		plan.Commands = []string{"go install github.com/mart337i/odooctl@latest"}
	case MethodSource:
		sourceDir := opts.SourceDir
		if sourceDir == "" {
			sourceDir = sourceCheckoutDir(rt.Executable)
		}
		if sourceDir == "" {
			return PlanResult{}, fmt.Errorf("--source-dir is required for source updates when odooctl is not running from a checkout")
		}
		plan.SourceDir = sourceDir
		plan.Commands = []string{"git pull --ff-only", "make install"}
	case MethodRelease:
		asset, err := assetName(rt.GOOS, rt.GOARCH)
		if err != nil {
			return PlanResult{}, err
		}
		assetInfo, ok := release.Asset(asset)
		if !ok {
			return PlanResult{}, fmt.Errorf("release %s has no asset %q", release.TagName, asset)
		}
		checksum, ok := release.Asset("checksums.txt")
		plan.Asset = asset
		plan.AssetURL = assetInfo.URL
		if ok {
			plan.ChecksumURL = checksum.URL
		} else {
			plan.Warning = appendWarning(plan.Warning, fmt.Sprintf("release %s has no checksums.txt asset; checksum verification will be skipped", release.TagName))
		}
	default:
		return PlanResult{}, fmt.Errorf("unsupported update method %q", method)
	}

	return plan, nil
}

func appendWarning(existing, addition string) string {
	if existing == "" {
		return addition
	}
	return existing + "; " + addition
}

func detectMethod(rt plannerRuntime, opts Options) (Method, error) {
	if opts.MethodOverride != "" && opts.MethodOverride != string(MethodAuto) {
		method := Method(opts.MethodOverride)
		switch method {
		case MethodAPT, MethodRelease, MethodGo, MethodSource:
			return method, nil
		default:
			return "", fmt.Errorf("unsupported update method %q", opts.MethodOverride)
		}
	}
	if rt.GOOS == "linux" && isAPTManaged(rt) {
		return MethodAPT, nil
	}
	if opts.SourceDir != "" {
		return MethodSource, nil
	}
	if isSourceCheckout(rt.Executable) {
		return MethodSource, nil
	}
	if isGoInstallPath(rt) {
		return MethodGo, nil
	}
	return MethodRelease, nil
}

func isAPTManaged(rt plannerRuntime) bool {
	output, err := rt.RunOutput("dpkg-query", "-S", rt.Executable)
	return err == nil && strings.Contains(output, "odooctl:")
}

func isGoInstallPath(rt plannerRuntime) bool {
	exe, err := filepath.Abs(rt.Executable)
	if err != nil {
		return false
	}
	goBin := rt.Getenv("GOBIN")
	if goBin != "" && samePath(filepath.Dir(exe), goBin) {
		return true
	}
	goPath := rt.Getenv("GOPATH")
	if goPath != "" && samePath(filepath.Dir(exe), filepath.Join(goPath, "bin")) {
		return true
	}
	return false
}

func isSourceCheckout(executable string) bool {
	return sourceCheckoutDir(executable) != ""
}

func sourceCheckoutDir(executable string) string {
	dir := filepath.Dir(executable)
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, "Makefile")) && dirHasGit(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	return errA == nil && errB == nil && filepath.Clean(absA) == filepath.Clean(absB)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirHasGit(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

func commandOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}
