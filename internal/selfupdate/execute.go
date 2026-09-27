package selfupdate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func Execute(plan PlanResult) error {
	if plan.RequiresForce {
		return fmt.Errorf("refusing to update non-release build without --force: %s", plan.Warning)
	}
	if !plan.UpdateAvailable {
		return nil
	}
	switch plan.Method {
	case MethodAPT:
		return runCommands("", [][]string{{"sudo", "apt", "update"}, {"sudo", "apt", "install", "--only-upgrade", "odooctl"}})
	case MethodGo:
		return runCommands("", [][]string{{"go", "install", "github.com/mart337i/odooctl@latest"}})
	case MethodSource:
		return runCommands(plan.SourceDir, [][]string{{"git", "pull", "--ff-only"}, {"make", "install"}})
	case MethodRelease:
		return updateFromRelease(plan)
	default:
		return fmt.Errorf("unsupported update method %q", plan.Method)
	}
}

func runCommands(dir string, commands [][]string) error {
	for _, command := range commands {
		if len(command) == 0 {
			continue
		}
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Dir = dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

func updateFromRelease(plan PlanResult) error {
	tmp, err := downloadAndVerify(plan)
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return replaceWindows(plan.Executable, tmp)
	}
	if err := os.Chmod(tmp, 0755); err != nil {
		return err
	}
	return replaceExecutable(plan.Executable, tmp)
}

func downloadAndVerify(plan PlanResult) (string, error) {
	if strings.TrimSpace(plan.AssetURL) == "" {
		return "", fmt.Errorf("release update plan is missing asset URL")
	}
	targetDir := filepath.Dir(plan.Executable)
	tmp, err := os.CreateTemp(targetDir, ".odooctl-update-*")
	if err != nil {
		return "", fmt.Errorf("create update file next to %s: %w", plan.Executable, err)
	}
	tmpPath := tmp.Name()
	defer tmp.Close()

	if err := downloadTo(tmp, plan.AssetURL); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if strings.TrimSpace(plan.ChecksumURL) != "" {
		if err := verifyChecksum(tmp, plan.ChecksumURL, plan.Asset); err != nil {
			_ = os.Remove(tmpPath)
			return "", err
		}
	}
	return tmpPath, nil
}

func downloadTo(dst io.Writer, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	_, err = io.Copy(dst, resp.Body)
	return err
}

func verifyChecksum(binary io.Reader, checksumURL, assetName string) error {
	resp, err := http.Get(checksumURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download checksums failed: %s", resp.Status)
	}
	checksums, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	expected, ok := checksumForAsset(checksums, assetName)
	if !ok {
		return fmt.Errorf("checksums.txt has no entry for %s", assetName)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, binary); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", assetName, expected, actual)
	}
	return nil
}

func checksumForAsset(data []byte, assetName string) (string, bool) {
	for _, line := range bytes.Split(data, []byte("\n")) {
		fields := strings.Fields(string(line))
		if len(fields) < 2 {
			continue
		}
		if fields[len(fields)-1] == assetName || filepath.Base(fields[len(fields)-1]) == assetName {
			return fields[0], true
		}
	}
	return "", false
}
