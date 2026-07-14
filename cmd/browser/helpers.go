package browser

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	internalbrowser "github.com/mart337i/odooctl/internal/browser"
	"github.com/mart337i/odooctl/internal/config"
)

type pageOptions struct {
	PublicURL       string `json:"public_url"`
	InternalURL     string `json:"internal_url"`
	Login           string `json:"login,omitempty"`
	Password        string `json:"password,omitempty"`
	ScreenshotPath  string `json:"screenshot_path,omitempty"`
	TracePath       string `json:"trace_path,omitempty"`
	TimeoutMS       int    `json:"timeout_ms"`
	WaitMS          int    `json:"wait_ms"`
	ViewportWidth   int    `json:"viewport_width"`
	ViewportHeight  int    `json:"viewport_height"`
	InternalBaseURL string `json:"internal_base_url"`
}

type pageReport struct {
	URL            string            `json:"url"`
	InternalURL    string            `json:"internal_url"`
	FinalURL       string            `json:"final_url"`
	Title          string            `json:"title"`
	Screenshot     string            `json:"screenshot,omitempty"`
	Trace          string            `json:"trace,omitempty"`
	VisibleText    []string          `json:"visible_text"`
	ConsoleErrors  []consoleMessage  `json:"console_errors"`
	FailedRequests []failedRequest   `json:"failed_requests"`
	Metrics        map[string]string `json:"metrics,omitempty"`
}

type consoleMessage struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type failedRequest struct {
	URL    string `json:"url"`
	Method string `json:"method"`
	Error  string `json:"error"`
}

type browserFlags struct {
	Login    string
	Password string
	Output   string
	JSON     bool
	Timeout  int
	Wait     int
	Width    int
	Height   int
}

func defaultBrowserFlags() browserFlags {
	return browserFlags{Timeout: 30000, Wait: 1000, Width: 1440, Height: 1000}
}

func loadState() (*config.State, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	state, err := config.LoadFromDir(cwd)
	if err != nil {
		return nil, fmt.Errorf("no Docker environment found. Run 'odooctl docker create' first")
	}
	return state, nil
}

func ensureBrowserReady(state *config.State) error {
	if !state.BrowserEnabled {
		return fmt.Errorf("browser tooling is not enabled for this environment. Run 'odooctl docker reconfigure --browser --rebuild' first")
	}
	return internalbrowser.EnsureSupported(state)
}

func resolveURLs(state *config.State, target string) (string, string, error) {
	if target == "" {
		target = "/web"
	}
	publicBase := fmt.Sprintf("http://localhost:%d", state.Ports.Odoo)
	internalBase := "http://127.0.0.1:8069"
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		parsed, err := url.Parse(target)
		if err != nil {
			return "", "", err
		}
		public := target
		if parsed.Host == fmt.Sprintf("localhost:%d", state.Ports.Odoo) || parsed.Host == fmt.Sprintf("127.0.0.1:%d", state.Ports.Odoo) {
			parsed.Scheme = "http"
			parsed.Host = "127.0.0.1:8069"
			return public, parsed.String(), nil
		}
		return public, target, nil
	}
	if !strings.HasPrefix(target, "/") {
		target = "/" + target
	}
	return publicBase + target, internalBase + target, nil
}

func artifactPaths(state *config.State, requested, prefix, ext string) (string, string, error) {
	artifactsDir, err := internalbrowser.EnsureArtifactsDir(state)
	if err != nil {
		return "", "", err
	}
	name := filepath.Base(requested)
	if requested == "" || name == "." || name == string(os.PathSeparator) {
		name = fmt.Sprintf("%s-%s%s", prefix, time.Now().Format("20060102-150405"), ext)
	}
	if filepath.Ext(name) == "" {
		name += ext
	}
	containerPath := filepath.Join(internalbrowser.ContainerArtifactsDir, name)
	localPath := filepath.Join(artifactsDir, name)
	if requested != "" {
		abs, err := filepath.Abs(requested)
		if err != nil {
			return "", "", err
		}
		localPath = abs
	}
	return localPath, containerPath, nil
}

func copyArtifactIfNeeded(state *config.State, localPath, containerPath string) error {
	artifactsDir, err := internalbrowser.ArtifactsDir(state)
	if err != nil {
		return err
	}
	artifactPath := filepath.Join(artifactsDir, filepath.Base(containerPath))
	if filepath.Clean(localPath) == filepath.Clean(artifactPath) {
		return nil
	}
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(localPath, data, 0644)
}

func runPage(state *config.State, target string, flags browserFlags, screenshotPath, tracePath string) (pageReport, error) {
	if err := ensureBrowserReady(state); err != nil {
		return pageReport{}, err
	}
	publicURL, internalURL, err := resolveURLs(state, target)
	if err != nil {
		return pageReport{}, err
	}
	options := pageOptions{
		PublicURL:       publicURL,
		InternalURL:     internalURL,
		Login:           flags.Login,
		Password:        flags.Password,
		ScreenshotPath:  screenshotPath,
		TracePath:       tracePath,
		TimeoutMS:       flags.Timeout,
		WaitMS:          flags.Wait,
		ViewportWidth:   flags.Width,
		ViewportHeight:  flags.Height,
		InternalBaseURL: "http://127.0.0.1:8069",
	}
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return pageReport{}, err
	}
	if screenshotPath != "" || tracePath != "" {
		if err := internalbrowser.EnsureContainerArtifactsDir(state); err != nil {
			return pageReport{}, err
		}
	}
	output, err := internalbrowser.RunPythonScript(state, pageScript(string(optionsJSON)))
	if err != nil {
		return pageReport{}, fmt.Errorf("browser run failed: %w\n%s", err, strings.TrimSpace(output))
	}
	var report pageReport
	if err := json.Unmarshal([]byte(internalbrowser.ExtractJSONOutput(output)), &report); err != nil {
		return pageReport{}, fmt.Errorf("failed to parse browser output: %w\n%s", err, strings.TrimSpace(output))
	}
	return report, nil
}

func pageScript(optionsJSON string) string {
	return fmt.Sprintf(`import asyncio
import json

from playwright.async_api import async_playwright

OPTIONS = %s

async def maybe_login(page):
    if not OPTIONS.get("login"):
        return
    await page.goto(OPTIONS["internal_base_url"] + "/web/login", wait_until="domcontentloaded", timeout=OPTIONS["timeout_ms"])
    await page.fill('input[name="login"]', OPTIONS["login"])
    await page.fill('input[name="password"]', OPTIONS.get("password") or "")
    await page.click('button[type="submit"], input[type="submit"]')
    await page.wait_for_timeout(OPTIONS["wait_ms"])

async def main():
    console_errors = []
    failed_requests = []
    async with async_playwright() as p:
        browser = await p.chromium.launch(headless=True, args=["--no-sandbox", "--disable-dev-shm-usage"])
        context = await browser.new_context(viewport={"width": OPTIONS["viewport_width"], "height": OPTIONS["viewport_height"]})
        if OPTIONS.get("trace_path"):
            await context.tracing.start(screenshots=True, snapshots=True, sources=True)
        page = await context.new_page()
        page.on("console", lambda msg: console_errors.append({"type": msg.type, "text": msg.text}) if msg.type in ["error", "warning"] else None)
        page.on("requestfailed", lambda req: failed_requests.append({"url": req.url, "method": req.method, "error": req.failure or "request failed"}))
        await maybe_login(page)
        await page.goto(OPTIONS["internal_url"], wait_until="domcontentloaded", timeout=OPTIONS["timeout_ms"])
        await page.wait_for_timeout(OPTIONS["wait_ms"])
        title = await page.title()
        try:
            text = await page.locator("body").inner_text(timeout=5000)
        except Exception:
            text = ""
        if OPTIONS.get("screenshot_path"):
            await page.screenshot(path=OPTIONS["screenshot_path"], full_page=True)
        if OPTIONS.get("trace_path"):
            await context.tracing.stop(path=OPTIONS["trace_path"])
        result = {
            "url": OPTIONS["public_url"],
            "internal_url": OPTIONS["internal_url"],
            "final_url": page.url,
            "title": title,
            "screenshot": OPTIONS.get("screenshot_path", ""),
            "trace": OPTIONS.get("trace_path", ""),
            "visible_text": [line.strip() for line in text.splitlines() if line.strip()][:120],
            "console_errors": console_errors,
            "failed_requests": failed_requests,
        }
        await browser.close()
        print(json.dumps(result))

asyncio.run(main())
`, optionsJSON)
}
