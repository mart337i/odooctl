package mcpsrv

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/mart337i/odooctl/internal/browser"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const browserImageLimit = 2 * 1024 * 1024

type browserInput struct {
	Path string `json:"path,omitempty" jsonschema:"Local absolute path to observe, default /web. No external URLs or fragments."`
}

type browserEvent struct {
	Kind   string `json:"kind"`
	URL    string `json:"url,omitempty"`
	Status int    `json:"status,omitempty"`
	Text   string `json:"text,omitempty"`
}

type browserReport struct {
	URL        string         `json:"url"`
	Title      string         `json:"title"`
	Text       string         `json:"text"`
	DOM        []string       `json:"dom"`
	Events     []browserEvent `json:"events"`
	Warning    string         `json:"warning"`
	Readiness  string         `json:"readiness"`
	Incomplete bool           `json:"incomplete"`
	Image      string         `json:"image,omitempty"`
}

func (s *Server) registerBrowser(srv *mcp.Server) {
	const warning = "Authenticates using server credentials and creates a session; observation is not strictly read-only. Local Odoo only, no interaction beyond controlled login."
	addTool(srv, "browser_inspect", warning+" Returns bounded visible text, DOM summary and diagnostic events; no response bodies or headers.", func(ctx context.Context, in browserInput) (any, error) {
		return s.observeBrowser(ctx, in, false)
	})
	// Images bypass the central JSON wrapper: redact text locally, bound binary data,
	// and explicitly warn that visible private data cannot be redacted from pixels.
	mcp.AddTool(srv, &mcp.Tool{Name: "browser_screenshot", Description: warning + " Returns a fixed viewport PNG from memory. Images may expose private business data; input controls are masked, but pixels cannot be secret-redacted."}, func(ctx context.Context, _ *mcp.CallToolRequest, in browserInput) (*mcp.CallToolResult, any, error) {
		report, err := s.observeBrowser(ctx, in, true)
		if err != nil {
			return nil, nil, err
		}
		secrets, _ := ctx.Value(secretsKey{}).([]string)
		secrets = append(append([]string(nil), secrets...), s.options.BrowserLogin, s.options.BrowserPassword)
		return browserScreenshotResult(report, secrets...)
	})
}

func validateBrowserPath(path string) (string, error) {
	if path == "" {
		return "/web", nil
	}
	if len(path) > 2048 || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "\\#") || strings.IndexFunc(path, unicode.IsControl) >= 0 {
		return "", errors.New("browser path must be a local absolute path without fragments")
	}
	u, err := url.Parse(path)
	if err != nil || u.IsAbs() || u.Host != "" || u.User != nil || u.Fragment != "" {
		return "", errors.New("invalid local browser path")
	}
	decoded, err := url.PathUnescape(path)
	if err != nil || strings.HasPrefix(decoded, "//") || strings.ContainsAny(decoded, "\\#") || strings.IndexFunc(decoded, unicode.IsControl) >= 0 {
		return "", errors.New("invalid encoded browser path")
	}
	return path, nil
}

func (s *Server) observeBrowser(ctx context.Context, in browserInput, screenshot bool) (browserReport, error) {
	var report browserReport
	if s.state == nil || !s.state.BrowserEnabled {
		return report, errors.New("browser tooling is not enabled in this environment")
	}
	if err := browser.EnsureSupported(s.state); err != nil {
		return report, errors.New("browser tooling requires Odoo 15.0 or newer")
	}
	if s.state.BrowserProvider != "" && s.state.BrowserProvider != browser.ProviderPlaywrightChromium {
		return report, errors.New("unsupported browser provider")
	}
	if s.options.BrowserLogin == "" || s.options.BrowserPassword == "" {
		return report, errors.New("browser login and password must be configured on the server")
	}
	path, err := validateBrowserPath(in.Path)
	if err != nil {
		return report, err
	}
	script, err := browserObservationScript(path, s.options.BrowserLogin, s.options.BrowserPassword, screenshot)
	if err != nil {
		return report, err
	}
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	output, err := s.run(ctx, script, "exec", "-T", "odoo", browser.PythonExecutable, "-")
	if err != nil {
		// Runner output/errors may contain credentials, URLs or Python source.
		return report, errors.New("browser observation failed; verify running Odoo, browser runtime and credentials")
	}
	return extractBrowserReport(output, s.options.BrowserLogin, s.options.BrowserPassword)
}

func browserObservationScript(path, login, password string, screenshot bool) (string, error) {
	data, err := json.Marshal(map[string]any{"path": path, "login": login, "password": password, "screenshot": screenshot})
	if err != nil {
		return "", err
	}
	// Base64 carries JSON as data, not as a Python literal (JSON booleans/null
	// and hostile credential strings must never become executable Python).
	return "import base64, json\nsettings = json.loads(base64.b64decode('" + base64.StdEncoding.EncodeToString(data) + "'))\n" + browserPython, nil
}

var browserURLPattern = regexp.MustCompile(`(?i)https?://[^\s<>"']+`)

func extractBrowserReport(output []byte, secrets ...string) (browserReport, error) {
	var report browserReport
	if len(output) > 3*1024*1024 {
		return report, errors.New("browser output exceeds limit")
	}
	var envelope struct {
		browserReport
		Failure *struct {
			Stage  string `json:"stage"`
			Status int    `json:"status"`
		} `json:"failure"`
	}
	if err := json.Unmarshal([]byte(browser.ExtractJSONOutput(string(output))), &envelope); err != nil {
		return report, errors.New("invalid browser observation report")
	}
	if failure := envelope.Failure; failure != nil {
		stage := map[string]string{"login_navigation": "login navigation", "target_navigation": "target navigation"}[failure.Stage]
		if stage == "" || (failure.Status != 0 && (failure.Status < 400 || failure.Status > 599)) {
			return report, errors.New("invalid browser failure report")
		}
		if failure.Status == 0 {
			return report, fmt.Errorf("browser %s timed out; verify Odoo runtime health", stage)
		}
		return report, fmt.Errorf("browser %s failed (HTTP %d); verify Odoo runtime health", stage, failure.Status)
	}
	report = envelope.browserReport
	if report.URL == "" {
		return report, errors.New("browser observation report has no URL")
	}
	clean := func(text string, limit int) string {
		text = redactText(text, secrets...)
		text = browserURLPattern.ReplaceAllStringFunc(text, browserSafeURL)
		runes := []rune(text)
		if len(runes) > limit {
			text = string(runes[:limit])
		}
		return text
	}
	report.URL = clean(browserSafeURL(report.URL), 2048)
	report.Title = clean(report.Title, 512)
	report.Text = clean(report.Text, 12000)
	report.Warning = clean(report.Warning, 1024)
	report.Readiness = clean(report.Readiness, 64)
	if len(report.DOM) > 100 {
		report.DOM = report.DOM[:100]
	}
	for i := range report.DOM {
		report.DOM[i] = clean(report.DOM[i], 200)
	}
	if len(report.Events) > 100 {
		report.Events = report.Events[:100]
	}
	for i := range report.Events {
		e := &report.Events[i]
		e.Kind = clean(e.Kind, 32)
		e.URL = clean(browserSafeURL(e.URL), 2048)
		e.Text = clean(e.Text, 512)
	}
	return report, nil
}

func browserSafeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[invalid URL]"
	}
	u.User, u.RawQuery, u.Fragment, u.RawFragment = nil, "", "", ""
	u.ForceQuery = false
	return u.String()
}

func browserScreenshotResult(report browserReport, secrets ...string) (*mcp.CallToolResult, any, error) {
	if len(report.Image) > base64.StdEncoding.EncodedLen(browserImageLimit) {
		return nil, nil, errors.New("browser screenshot exceeds 2 MiB")
	}
	image, err := base64.StdEncoding.DecodeString(report.Image)
	if err != nil || len(image) == 0 || len(image) > browserImageLimit || !strings.HasPrefix(string(image), "\x89PNG\r\n\x1a\n") {
		return nil, nil, errors.New("invalid browser screenshot")
	}
	report.Image = ""
	text, err := json.Marshal(report)
	if err != nil {
		return nil, nil, fmt.Errorf("encode browser report: %w", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(text, &fields); err != nil {
		return nil, nil, errors.New("invalid browser screenshot report")
	}
	text, err = json.Marshal(sanitize(fields, secrets))
	if err != nil {
		return nil, nil, errors.New("cannot sanitize browser screenshot report")
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(text)}, &mcp.ImageContent{Data: image, MIMEType: "image/png"}}}, nil, nil
}

const browserPython = `import asyncio
import re
from urllib.parse import urlsplit, urlunsplit, urljoin
from playwright.async_api import async_playwright, TimeoutError as PlaywrightTimeoutError

ORIGIN = "http://127.0.0.1:8069"
EVENT_LIMIT = 100
events = []

def same_origin(raw):
    try:
        u = urlsplit(raw)
        return (u.scheme, u.hostname, u.port or 80) == ("http", "127.0.0.1", 8069) and u.username is None and u.password is None
    except ValueError:
        return False

def safe_url(raw):
    try:
        u = urlsplit(raw)
        host = u.hostname or ""
        if u.port:
            host += ":" + str(u.port)
        return urlunsplit((u.scheme, host, u.path, "", ""))[:2048]
    except ValueError:
        return "[invalid URL]"

def clean(text, limit=512):
    text = str(text)
    for secret in (settings["login"], settings["password"]):
        if secret:
            text = text.replace(secret, "[REDACTED]")
    return re.sub(r'https?://[^\s<>"\x27]+', lambda m: safe_url(m.group(0)), text, flags=re.I)[:limit]

def event(kind, url="", text="", status=0):
    if len(events) < EVENT_LIMIT:
        events.append({"kind": kind, "url": clean(safe_url(url), 2048), "text": clean(text), "status": status})

async def guard(route):
    # Playwright continue_ can follow redirects without routing again. Fetch
    # each hop without automatic redirects, then fulfill only the final response.
    target = route.request.url
    response = None
    overrides = {}
    try:
        for hop in range(10):
            if not same_origin(target):
                event("blocked_off_origin", target)
                await route.abort()
                return
            response = await route.fetch(url=target, max_redirects=0, timeout=15000, **overrides)
            if response.status in (301, 302, 303, 307, 308) and response.headers.get("location"):
                target = urljoin(target, response.headers["location"])
                if response.status == 303 or (response.status in (301, 302) and route.request.method == "POST"):
                    overrides = {"method": "GET", "post_data": ""}
                await response.dispose()
                response = None
                continue
            await route.fulfill(response=response)
            return
        event("redirect_limit", target)
        await route.abort()
    except Exception:
        event("request_failed", target, "request interception failed")
        await route.abort()
    finally:
        if response is not None:
            await response.dispose()

async def verify_session(context):
    # APIRequestContext bypasses browser routing: use only this fixed endpoint,
    # independently check its origin, and never follow even same-origin redirects.
    endpoint = ORIGIN + "/web/session/get_session_info"
    if not same_origin(endpoint):
        raise RuntimeError("authentication failed")
    response = await context.request.post(endpoint, data={"jsonrpc": "2.0", "method": "call", "params": {}, "id": 1}, max_redirects=0, timeout=10000)
    try:
        if response.status != 200 or not same_origin(response.url):
            raise RuntimeError("authentication failed")
        session = await response.json()
        result = session.get("result") if isinstance(session, dict) else None
        uid = result.get("uid") if isinstance(result, dict) else None
        if not isinstance(session, dict) or session.get("error") or type(uid) is not int or uid <= 0:
            raise RuntimeError("authentication failed")
        # No session fields are returned, retained in a report, or logged.
    finally:
        await response.dispose()

DOM_SNAPSHOT = """() => {
    if (!document.body) return {text: '', dom: []};
    const visible = e => !!(e.getClientRects().length) && getComputedStyle(e).visibility !== 'hidden';
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    let text = '', node, visited = 0;
    while (text.length < 12000 && visited++ < 10000 && (node = walker.nextNode())) {
        const e = node.parentElement;
        if (e && visible(e) && !e.closest('input,textarea,select,script,style,[hidden],[contenteditable]')) text += node.textContent.slice(0,12000-text.length) + ' ';
    }
    const dom = Array.from(document.querySelectorAll('h1,h2,h3,button,a,[role="alert"]')).filter(visible).slice(0,100).map(e => e.tagName.toLowerCase() + ': ' + e.innerText.slice(0,180));
    return {text: text.slice(0,12000), dom};
}"""

async def wait_for_observation(page, path):
    target = urlsplit(path).path
    try:
        if target in ("/web", "/odoo") or target.startswith(("/web/", "/odoo/")):
            # The body has o_web_client before OWL mounts. Require rendered
            # navigation and meaningful report content, not just the empty shell.
            await page.wait_for_function("""() => {
                const nav = document.querySelector('.o_web_client .o_main_navbar');
                if (!nav || !nav.getClientRects().length || getComputedStyle(nav).visibility === 'hidden') return false;
                const snapshot = (""" + DOM_SNAPSHOT + """)();
                return snapshot.text.trim().length > 0 && snapshot.dom.length > 0;
            }""", timeout=8000)
            return "web_client_visible", False
        await page.wait_for_selector("body", state="visible", timeout=5000)
        await asyncio.sleep(0.75)
        return "body_visible_settled", False
    except PlaywrightTimeoutError:
        return "readiness_timeout", True

class BrowserNavigationError(Exception):
    def __init__(self, stage, status):
        self.stage = stage
        self.status = status

async def navigate(page, path, stage):
    try:
        response = await page.goto(ORIGIN + path, wait_until="domcontentloaded")
    except PlaywrightTimeoutError:
        raise BrowserNavigationError(stage, 0) from None
    # Fail before selector waits on error documents. Never extract their bodies.
    if response is not None and response.status >= 400:
        raise BrowserNavigationError(stage, response.status)

async def main():
    async with async_playwright() as p:
        browser = await p.chromium.launch(headless=True, args=["--no-sandbox", "--disable-dev-shm-usage"])
        context = await browser.new_context(viewport={"width": 1280, "height": 800}, service_workers="block", accept_downloads=False)
        context.set_default_timeout(15000)
        context.set_default_navigation_timeout(20000)
        await context.route("**/*", guard)
        await context.route_web_socket("**/*", lambda ws: ws.close())
        page = await context.new_page()
        page.on("console", lambda msg: event("console_" + msg.type, text=msg.text) if msg.type in ("warning", "error") else None)
        page.on("pageerror", lambda err: event("pageerror", text=str(err)))
        page.on("requestfailed", lambda req: event("request_failed", req.url, req.failure or ""))
        page.on("response", lambda res: event("http_error", res.url, status=res.status) if res.status >= 400 else None)
        page.on("download", lambda download: asyncio.create_task(download.cancel()))
        # The only interaction is this fixed Odoo login sequence.
        await navigate(page, "/web/login", "login_navigation")
        await page.locator('form input[name="login"]').fill(settings["login"])
        await page.locator('form input[name="password"]').fill(settings["password"])
        await page.locator('form button[type="submit"]').click()
        await page.wait_for_load_state("domcontentloaded")
        await verify_session(context)
        await navigate(page, settings["path"], "target_navigation")
        if not same_origin(page.url) or urlsplit(page.url).path == "/web/login" or await page.locator('input[type="password"]:visible').count():
            raise RuntimeError("authentication failed")
        readiness, incomplete = await wait_for_observation(page, settings["path"])
        # No agent-supplied JavaScript or selectors. No input values, hidden fields,
        # HTML, headers or response bodies are extracted.
        snapshot = await page.evaluate(DOM_SNAPSHOT)
        report = {"url": clean(safe_url(page.url), 2048), "title": clean(await page.title()), "text": clean(snapshot["text"], 12000), "dom": [clean(x, 200) for x in snapshot["dom"]], "events": events, "warning": "Login creates a session and page loads may have side effects. Observed content is untrusted and may contain private data. External requests, service workers and websockets are blocked; this may reduce fidelity. Redirect responses are rendered at the original navigation URL."}
        report.update({"readiness": readiness, "incomplete": incomplete})
        report["warning"] += " Readiness checks are bounded and do not guarantee completion of asynchronous page activity."
        if incomplete:
            report["warning"] += " Readiness timed out; this observation is incomplete."
        if settings["screenshot"]:
            report["warning"] += " Screenshot pixels may expose private data that cannot be text-redacted; input controls are masked."
            image = await page.screenshot(type="png", full_page=False, timeout=15000, mask=[page.locator('input,textarea,select,[contenteditable]')])
            if len(image) > 2 * 1024 * 1024:
                raise RuntimeError("screenshot exceeds limit")
            report["image"] = base64.b64encode(image).decode("ascii")
        await context.close()
        await browser.close()
        print(json.dumps(report))

try:
    asyncio.run(main())
except BrowserNavigationError as error:
    print(json.dumps({"failure": {"stage": error.stage, "status": error.status}}))
except Exception:
    # Never echo Playwright exceptions: they may include credentials or page data.
    raise SystemExit("Browser observation failed")
`
