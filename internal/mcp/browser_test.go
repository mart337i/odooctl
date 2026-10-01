package mcpsrv

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mart337i/odooctl/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestObserveBrowserUsesIsolatedPython(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix fake Docker fixture")
	}
	t.Setenv("HOME", t.TempDir())
	state := &config.State{ProjectName: "browser", Branch: "main", OdooVersion: "19.0", BrowserEnabled: true}
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := `#!/bin/sh
case "$*" in
    "compose version") exit 0 ;;
    "compose exec -T odoo timeout --signal=TERM --kill-after=5s 80s /opt/odoo-browser-venv/bin/python3 -")
        /bin/cat >/dev/null
        printf '%s\n' '{"url":"http://127.0.0.1:8069/web"}' ;;
    *) exit 11 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	s := &Server{state: state, options: Options{BrowserLogin: "user", BrowserPassword: "password"}}
	for _, screenshot := range []bool{false, true} {
		report, err := s.observeBrowser(context.Background(), browserInput{}, screenshot)
		if err != nil || report.URL != "http://127.0.0.1:8069/web" {
			t.Fatalf("observeBrowser(screenshot=%v) = %+v, %v", screenshot, report, err)
		}
	}
}

func TestBrowserValidationBeforeExecution(t *testing.T) {
	for _, tc := range []struct {
		name    string
		state   *config.State
		options Options
		path    string
	}{
		{name: "missing state"},
		{name: "disabled", state: &config.State{OdooVersion: "18.0"}},
		{name: "unsupported version", state: &config.State{BrowserEnabled: true, OdooVersion: "14.0"}},
		{name: "unknown provider", state: &config.State{BrowserEnabled: true, OdooVersion: "18.0", BrowserProvider: "other"}},
		{name: "missing credentials", state: &config.State{BrowserEnabled: true, OdooVersion: "18.0"}},
		{name: "unsafe path", state: &config.State{BrowserEnabled: true, OdooVersion: "18.0"}, options: Options{BrowserLogin: "user", BrowserPassword: "password"}, path: "https://evil.example/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{state: tc.state, options: tc.options}
			if _, err := s.observeBrowser(context.Background(), browserInput{Path: tc.path}, false); err == nil {
				t.Fatal("invalid browser configuration accepted")
			}
		})
	}
}

func TestRegisterBrowserSchemas(t *testing.T) {
	srv := mcp.NewServer(&mcp.Implementation{Name: "browser-test", Version: "test"}, nil)
	(&Server{}).registerBrowser(srv)
}

func TestValidateBrowserPath(t *testing.T) {
	for _, path := range []string{"", "/web", "/odoo/apps", "/web?debug=assets", "/my/orders/1"} {
		got, err := validateBrowserPath(path)
		if err != nil || (path != "" && got != path) || (path == "" && got != "/web") {
			t.Errorf("validateBrowserPath(%q) = %q, %v", path, got, err)
		}
	}
	for _, path := range []string{"https://example.com/web", "http://user:password@localhost/web", "//evil/web", "web", "/\\evil", "/web#fragment", "/%2fevil", "/%5cevil", "/web%23fragment", "/web\n", "/web%0a", "/web%zz", "/" + strings.Repeat("x", 2048)} {
		if _, err := validateBrowserPath(path); err == nil {
			t.Errorf("accepted unsafe path %q", path)
		}
	}
}

func TestBrowserScriptCredentialsAreData(t *testing.T) {
	login := "user'\"\\\n\u2028"
	password := "'); __import__('os').system('bad') #"
	script, err := browserObservationScript("/web?debug=1", login, password, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(script, login) || strings.Contains(script, password) {
		t.Fatal("raw credentials embedded in Python")
	}
	const prefix = "settings = json.loads(base64.b64decode('"
	_, tail, found := strings.Cut(script, prefix)
	if !found {
		t.Fatal("missing safe JSON loader")
	}
	encoded, _, found := strings.Cut(tail, "'))\n")
	if !found {
		t.Fatal("invalid settings encoding")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Login      string `json:"login"`
		Password   string `json:"password"`
		Path       string `json:"path"`
		Screenshot bool   `json:"screenshot"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Login != login || settings.Password != password || settings.Path != "/web?debug=1" || !settings.Screenshot {
		t.Fatalf("settings did not round-trip: %#v", settings)
	}
}

func TestBrowserScriptSafetyGuards(t *testing.T) {
	for _, required := range []string{
		`await context.route("**/*", guard)`,
		`if not same_origin(target):`,
		`max_redirects=0`,
		`target = urljoin(target, response.headers["location"])`,
		`u.username is None and u.password is None`,
		`service_workers="block", accept_downloads=False`,
		`await context.route_web_socket("**/*", lambda ws: ws.close())`,
		`"width": 1280, "height": 800`,
		`full_page=False`,
		`mask=[page.locator('input,textarea,select,[contenteditable]')]`,
		`if len(image) > 2 * 1024 * 1024:`,
		`!e.closest('input,textarea,select,script,style,[hidden],[contenteditable]')`,
		`if len(events) < EVENT_LIMIT:`,
		`raise SystemExit("Browser observation failed")`,
		`await verify_session(context)`,
		`max_redirects=0, timeout=10000`,
		`type(uid) is not int or uid <= 0`,
		`document.querySelector('.o_web_client .o_main_navbar')`,
		`return snapshot.text.trim().length > 0 && snapshot.dom.length > 0;`,
		`await page.evaluate(DOM_SNAPSHOT)`,
		`return "readiness_timeout", True`,
	} {
		if !strings.Contains(browserPython, required) {
			t.Errorf("missing safety guard %q", required)
		}
	}
	if strings.Index(browserPython, "await verify_session(context)") > strings.Index(browserPython, `await navigate(page, settings["path"], "target_navigation")`) {
		t.Fatal("public target can be reached before authenticated session verification")
	}
	for _, forbidden := range []string{"route.continue_()", "response.body(", "storage_state", "screenshot(path=", "/browser-artifacts", "settings[\"selector\"]", "settings[\"javascript\"]", ".input_value(", "innerHTML"} {
		if strings.Contains(browserPython, forbidden) {
			t.Errorf("unsafe script feature %q", forbidden)
		}
	}
}

func TestBrowserNavigationFailureReports(t *testing.T) {
	for _, tc := range []struct{ output, want string }{
		{`{"failure":{"stage":"login_navigation","status":500}}`, "browser login navigation failed (HTTP 500); verify Odoo runtime health"},
		{`{"failure":{"stage":"target_navigation","status":404}}`, "browser target navigation failed (HTTP 404); verify Odoo runtime health"},
		{`{"failure":{"stage":"login_navigation","status":0}}`, "browser login navigation timed out; verify Odoo runtime health"},
		{`{"failure":{"stage":"password=private","status":500}}`, "invalid browser failure report"},
		{`{"failure":{"stage":"login_navigation","status":200}}`, "invalid browser failure report"},
	} {
		_, err := extractBrowserReport([]byte(tc.output))
		if err == nil || err.Error() != tc.want {
			t.Errorf("failure report: got %v, want %q", err, tc.want)
		}
	}
}

func TestExtractBrowserReport(t *testing.T) {
	source := browserReport{
		URL:   "http://user:password@127.0.0.1:8069/web?token=secret#private",
		Title: "password title", Text: strings.Repeat("x", 14000),
		DOM: make([]string, 120), Events: make([]browserEvent, 120),
		Warning:   "https://user:password@example.com/path?token=secret#private",
		Readiness: "readiness_timeout", Incomplete: true,
	}
	for i := range source.DOM {
		source.DOM[i] = strings.Repeat("x", 300)
	}
	for i := range source.Events {
		source.Events[i] = browserEvent{Kind: "http_error", URL: source.URL, Text: "password https://example.com/path?token=secret#private", Status: 403}
	}
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	report, err := extractBrowserReport(append([]byte("compose notice\n"), data...), "user", "password")
	if err != nil {
		t.Fatal(err)
	}
	if report.URL != "http://127.0.0.1:8069/web" || report.Title != "[REDACTED] title" || len(report.Text) != 12000 || len(report.DOM) != 100 || len(report.DOM[0]) != 200 || len(report.Events) != 100 || report.Events[0].Status != 403 {
		t.Fatalf("incorrect report extraction: URL=%q title=%q text=%d dom=%d events=%d", report.URL, report.Title, len(report.Text), len(report.DOM), len(report.Events))
	}
	if report.Readiness != "readiness_timeout" || !report.Incomplete {
		t.Fatal("incomplete readiness status lost during report extraction")
	}
	cleaned, _ := json.Marshal(report)
	for _, secret := range []string{"password", "?token", "#private", "user:"} {
		if strings.Contains(string(cleaned), secret) {
			t.Errorf("report leaks %q", secret)
		}
	}
	for _, invalid := range [][]byte{[]byte("not JSON"), []byte(`{"events":42}`), []byte("null"), []byte("{}"), []byte(strings.Repeat("x", 3*1024*1024+1))} {
		if _, err := extractBrowserReport(invalid); err == nil {
			t.Fatal("accepted invalid/oversized report")
		}
	}
}

func TestBrowserPythonOriginGuard(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable; no browser runtime is required")
	}
	script, err := browserObservationScript("/web", "user", "password", false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Parse the entire generated program, then execute only pure helpers and the
	// guard against fake routes. Never import Playwright or contact a service.
	cmd := exec.CommandContext(ctx, python, "-c", `import ast, asyncio, sys, json, shutil, subprocess
from urllib.parse import urlsplit, urlunsplit, urljoin
source = sys.stdin.read()
compile(source, '<browser>', 'exec')
tree = ast.parse(source)
helpers = [node for node in tree.body if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)) and node.name in ('same_origin', 'guard', 'verify_session', 'wait_for_observation', 'BrowserNavigationError', 'navigate')]
helpers += [node for node in tree.body if isinstance(node, ast.Assign) and any(isinstance(target, ast.Name) and target.id == 'DOM_SNAPSHOT' for target in node.targets)]
scope = dict(urlsplit=urlsplit, urljoin=urljoin, event=lambda *args, **kwargs: None, ORIGIN='http://127.0.0.1:8069', PlaywrightTimeoutError=TimeoutError)
exec(compile(ast.Module(body=helpers, type_ignores=[]), '<guard>', 'exec'), scope)
same_origin = scope['same_origin']
assert same_origin('http://127.0.0.1:8069/web')
for url in ['https://127.0.0.1:8069/web', 'http://localhost:8069/web', 'http://127.0.0.1/web', 'http://user:password@127.0.0.1:8069/web', 'http://evil/web', 'data:text/html,bad', 'http://127.0.0.1:bad/web']:
    assert not same_origin(url), url

class Response:
    def __init__(self, status, location=None):
        self.status = status
        self.headers = {'location': location} if location else {}
        self.disposed = False
    async def dispose(self):
        self.disposed = True

class Request:
    url = 'http://127.0.0.1:8069/web/login'
    method = 'POST'

class Route:
    request = Request()
    def __init__(self, responses):
        self.responses = list(responses)
        self.calls = []
        self.aborted = False
        self.fulfilled = False
    async def fetch(self, **kwargs):
        assert same_origin(kwargs['url']), 'off-origin fetch attempted'
        assert kwargs['max_redirects'] == 0
        self.calls.append(kwargs)
        return self.responses.pop(0)
    async def abort(self):
        self.aborted = True
    async def fulfill(self, **kwargs):
        self.fulfilled = True

class SessionResponse(Response):
    def __init__(self, payload, status=200, url='http://127.0.0.1:8069/web/session/get_session_info'):
        super().__init__(status)
        self.url = url
        self.payload = payload
        self.reads = 0
    async def json(self):
        self.reads += 1
        return self.payload

class SessionContext:
    def __init__(self, response):
        self.response = response
        self.request = self
        self.calls = []
    async def post(self, endpoint, **kwargs):
        assert endpoint == 'http://127.0.0.1:8069/web/session/get_session_info'
        assert kwargs == dict(data={'jsonrpc': '2.0', 'method': 'call', 'params': {}, 'id': 1}, max_redirects=0, timeout=10000)
        self.calls.append(endpoint)
        return self.response

class ReadyPage:
    def __init__(self, timeout=False, states=None):
        self.timeout = timeout
        self.calls = []
        self.states = states or [dict(nav=True, text='Rendered navigation', controls=True)]
        self.polls = []
    async def wait_for_selector(self, selector, **kwargs):
        self.calls.append((selector, kwargs))
        if self.timeout:
            raise TimeoutError()
    async def wait_for_function(self, expression, **kwargs):
        assert kwargs == {'timeout': 8000}
        assert "document.querySelector('.o_web_client .o_main_navbar')" in expression
        assert 'snapshot.text.trim().length > 0 && snapshot.dom.length > 0' in expression
        self.calls.append(('web_client_predicate', kwargs))
        if self.timeout:
            raise TimeoutError()
        node = shutil.which('node')
        if node:
            # Execute the actual fixed JS predicate and DOM snapshot against an
            # empty visible body and synthetic asynchronously-rendered fixtures.
            program = """const fs = require('fs');
const payload = JSON.parse(fs.readFileSync(0, 'utf8'));
let fixture;
const element = {getClientRects: () => [1], closest: () => null};
global.getComputedStyle = e => ({visibility: e.hidden ? 'hidden' : 'visible'});
global.NodeFilter = {SHOW_TEXT: 4};
global.document = {
    body: element,
    querySelector: () => fixture.nav ? {getClientRects: () => [1], hidden: fixture.hidden} : null,
    querySelectorAll: () => fixture.controls ? [{...element, tagName: 'BUTTON', innerText: fixture.text}] : [],
    createTreeWalker: () => {
        let visited = false;
        return {nextNode: () => {
            if (visited) return null;
            visited = true;
            return {parentElement: element, textContent: fixture.text};
        }};
    },
};
const ready = eval('(' + payload.expression + ')');
console.log(JSON.stringify(payload.states.map(state => {fixture = state; return ready();})));
"""
            completed = subprocess.run([node, '-e', program], input=json.dumps(dict(expression=expression, states=self.states)), text=True, capture_output=True, timeout=3, check=True)
            results = json.loads(completed.stdout)
        else:
            results = [bool(state['nav'] and not state.get('hidden') and state['text'].strip() and state['controls']) for state in self.states]
        for ready in results:
            self.polls.append(ready)
            if ready:
                return
        raise TimeoutError()

class Clock:
    def __init__(self):
        self.delays = []
    async def sleep(self, seconds):
        self.delays.append(seconds)

class NavigationPage:
    def __init__(self, status=200, timeout=False):
        self.status = status
        self.timeout = timeout
        self.calls = []
    async def goto(self, url, **kwargs):
        assert url == 'http://127.0.0.1:8069/web/login'
        assert kwargs == {'wait_until': 'domcontentloaded'}
        self.calls.append(url)
        if self.timeout:
            raise TimeoutError('private timeout detail must not escape')
        return Response(self.status)

async def test():
    # Error documents must fail immediately, never falling through to the
    # login selectors' 15-second timeout or exposing the response body.
    for status in [400, 404, 500, 503]:
        try:
            await scope['navigate'](NavigationPage(status), '/web/login', 'login_navigation')
            assert False, 'HTTP error document accepted'
        except scope['BrowserNavigationError'] as error:
            assert error.stage == 'login_navigation' and error.status == status
    try:
        await scope['navigate'](NavigationPage(timeout=True), '/web/login', 'login_navigation')
        assert False, 'navigation timeout accepted'
    except scope['BrowserNavigationError'] as error:
        assert error.status == 0 and error.stage == 'login_navigation'
        assert error.__suppress_context__
    assert await scope['navigate'](NavigationPage(), '/web/login', 'login_navigation') is None
    redirect = Response(302, '//evil.example/steal')
    route = Route([redirect])
    await scope['guard'](route)
    assert route.aborted and not route.fulfilled and len(route.calls) == 1 and redirect.disposed
    route = Route([Response(302, '/web'), Response(200)])
    await scope['guard'](route)
    assert route.fulfilled and not route.aborted and len(route.calls) == 2
    assert route.calls[1]['method'] == 'GET' and route.calls[1]['post_data'] == ''
    route = Route([Response(307, '/web'), Response(302, 'https://evil.example/')])
    await scope['guard'](route)
    assert route.aborted and len(route.calls) == 2 and 'method' not in route.calls[1]
    route = Route([Response(302, '/loop') for _ in range(10)])
    await scope['guard'](route)
    assert route.aborted and len(route.calls) == 10
    # A public target is irrelevant: missing or invalid authenticated UIDs must
    # fail before observation, including when the login form remains visible.
    for payload in [None, {}, {'result': {}}, {'result': {'uid': False}}, {'result': {'uid': True}}, {'result': {'uid': 0}}, {'result': {'uid': -1}}, {'result': {'uid': '2'}}, {'error': {'message': 'private session error'}, 'result': {'uid': 2}}]:
        response = SessionResponse(payload)
        public_page_observed = False
        try:
            await scope['verify_session'](SessionContext(response))
            public_page_observed = True
        except RuntimeError:
            pass
        assert not public_page_observed and response.disposed, payload
    for response in [SessionResponse({'result': {'uid': 2}}, status=302), SessionResponse({'result': {'uid': 2}}, url='https://evil.example/session')]:
        try:
            await scope['verify_session'](SessionContext(response))
            assert False, 'redirected or off-origin session accepted'
        except RuntimeError:
            pass
        assert response.disposed and response.reads == 0
    for uid in [1, 2, 42]:
        response = SessionResponse({'result': {'uid': uid, 'private_session_data': 'never report this'}})
        assert await scope['verify_session'](SessionContext(response)) is None
        assert response.disposed
    scope['asyncio'] = Clock()
    for path in ['/web', '/web?debug=1', '/odoo', '/odoo/apps']:
        page = ReadyPage()
        assert await scope['wait_for_observation'](page, path) == ('web_client_visible', False)
        assert page.calls == [('web_client_predicate', {'timeout': 8000})]
    empty_shell = dict(nav=False, text='', controls=False)
    empty_nav = dict(nav=True, text='', controls=False)
    hidden_nav = dict(nav=True, text='Navigation', controls=True, hidden=True)
    text_only = dict(nav=True, text='Loading', controls=False)
    controls_only = dict(nav=True, text='   ', controls=True)
    rendered = dict(nav=True, text='Rendered navigation', controls=True)
    page = ReadyPage(states=[empty_shell, empty_nav, hidden_nav, text_only, controls_only, rendered])
    assert await scope['wait_for_observation'](page, '/web') == ('web_client_visible', False)
    assert page.polls == [False, False, False, False, False, True]
    for shell in [empty_shell, empty_nav, text_only, controls_only]:
        page = ReadyPage(states=[shell])
        assert await scope['wait_for_observation'](page, '/web') == ('readiness_timeout', True)
        assert page.polls == [False]
    page = ReadyPage()
    assert await scope['wait_for_observation'](page, '/public') == ('body_visible_settled', False)
    assert page.calls == [('body', {'state': 'visible', 'timeout': 5000})]
    assert scope['asyncio'].delays == [0.75]
    for path in ['/web', '/public']:
        assert await scope['wait_for_observation'](ReadyPage(timeout=True), path) == ('readiness_timeout', True)
asyncio.run(test())
`)
	cmd.Stdin = strings.NewReader(script)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pure Python origin guard tests: %v\n%s", err, output)
	}
}

func TestBrowserScreenshotTextSanitization(t *testing.T) {
	report := browserReport{
		Title:  "password=visible-password",
		Text:   "Authorization: Bearer visible-bearer configured-secret",
		DOM:    []string{"token=visible-token"},
		Events: []browserEvent{{Kind: "console_warning", Text: "api_key=visible-api-key"}},
		Image:  base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nexample image")),
	}
	ctx := context.WithValue(context.Background(), secretsKey{}, []string{"configured-secret"})
	secrets, _ := ctx.Value(secretsKey{}).([]string)
	result, _, err := browserScreenshotResult(report, secrets...)
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	for _, secret := range []string{"visible-password", "visible-bearer", "configured-secret", "visible-token", "visible-api-key"} {
		if strings.Contains(text, secret) {
			t.Errorf("screenshot text leaks %q", secret)
		}
	}
	if !strings.Contains(text, "[REDACTED]") || !strings.Contains(text, "[REDACTED AUTHORIZATION]") {
		t.Fatalf("generic redaction not applied: %s", text)
	}
	// Standalone calls still apply generic patterns without configured secrets.
	result, _, err = browserScreenshotResult(report)
	if err != nil {
		t.Fatal(err)
	}
	if text := result.Content[0].(*mcp.TextContent).Text; strings.Contains(text, "visible-password") || strings.Contains(text, "visible-bearer") {
		t.Fatalf("standalone screenshot text bypasses generic redaction: %s", text)
	}
}

func TestBrowserScreenshotResult(t *testing.T) {
	// This tests transport encoding, not PNG rendering or a live browser.
	png := []byte("\x89PNG\r\n\x1a\nexample image")
	report := browserReport{Title: "Observed page", Image: base64.StdEncoding.EncodeToString(png)}
	result, structured, err := browserScreenshotResult(report)
	if err != nil || structured != nil || len(result.Content) != 2 {
		t.Fatalf("screenshot result: %v, %v, %v", result, structured, err)
	}
	image, ok := result.Content[1].(*mcp.ImageContent)
	if !ok || string(image.Data) != string(png) || image.MIMEType != "image/png" {
		t.Fatal("screenshot is not native MCP image content")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if strings.Contains(text, report.Image) || strings.Contains(text, `"image"`) {
		t.Fatal("binary included in JSON report")
	}
	wire, err := json.Marshal(image)
	if err != nil || !strings.Contains(string(wire), report.Image) {
		t.Fatalf("SDK does not encode image bytes as base64: %s, %v", wire, err)
	}
	for _, invalid := range []string{"", "not base64", base64.StdEncoding.EncodeToString([]byte("not PNG")), strings.Repeat("x", base64.StdEncoding.EncodedLen(browserImageLimit)+1)} {
		if _, _, err := browserScreenshotResult(browserReport{Image: invalid}); err == nil {
			t.Fatal("accepted invalid/oversized screenshot")
		}
	}
}
