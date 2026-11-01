package mcpsrv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// These native schemas mirror the 0.0.83 definitions, including optional
// parameters which the adapter must remove rather than merely hide in prose.
func playwrightFixtureTools() []*mcp.Tool {
	property := func(kind, description string) map[string]any {
		return map[string]any{"type": kind, "description": description}
	}
	tools := []*mcp.Tool{}
	for name, fields := range map[string][]string{
		"browser_navigate":         {"url"},
		"browser_snapshot":         {"target", "depth", "boxes"},
		"browser_take_screenshot":  {"type", "target", "element", "fullPage", "scale"},
		"browser_console_messages": {"level"},
		"browser_network_requests": {"static"},
		"browser_resize":           {"width", "height"},
		"browser_wait_for":         {"time", "text", "textGone"},
		"browser_close":            {},
	} {
		props := map[string]any{}
		for _, field := range fields {
			kind := "string"
			switch field {
			case "width", "height", "time", "depth":
				kind = "number"
			case "static", "boxes", "fullPage":
				kind = "boolean"
			}
			props[field] = property(kind, "Official "+field)
		}
		if name == "browser_network_requests" {
			props["filter"] = property("string", "URL regexp")
			props["static"].(map[string]any)["default"] = false
		}
		if name == "browser_console_messages" {
			props["all"] = property("boolean", "Whole session")
			props["level"].(map[string]any)["enum"] = []string{"error", "warning", "info", "debug"}
			props["level"].(map[string]any)["default"] = "info"
		}
		if name == "browser_snapshot" || name == "browser_take_screenshot" || name == "browser_console_messages" || name == "browser_network_requests" {
			props["filename"] = property("string", "Save to file")
		}
		schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
		if name == "browser_navigate" {
			schema["required"] = []string{"url"}
		}
		if name == "browser_resize" {
			schema["required"] = []string{"width", "height"}
		}
		tools = append(tools, &mcp.Tool{Name: name, Description: "Official " + name, Title: name, InputSchema: schema, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}})
	}
	for _, name := range []string{"browser_evaluate", "browser_run_code_unsafe", "browser_click", "browser_type", "browser_fill_form", "browser_file_upload", "browser_network_request", "browser_get_config", "browser_route", "browser_cookie_list", "browser_tabs", "browser_handle_dialog", "browser_page_webmcp", "future_tool"} {
		tools = append(tools, &mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}})
	}
	return tools
}

func playwrightRuntime(t *testing.T) *Runtime {
	t.Helper()
	return &Runtime{Project: t.TempDir(), PublicURL: "http://localhost:18069", InternalURL: "http://odoo:8069", ArtifactsDir: t.TempDir(), ProxyContainer: "private-proxy", BrowserNetwork: "private-net", BrowserImage: "pinned-image", AllowedOrigins: []string{"https://cdn.example.org"}}
}

func playwrightImage(t *testing.T, format string, width, height int) []byte {
	t.Helper()
	var data bytes.Buffer
	pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
	var err error
	if format == "jpeg" {
		err = jpeg.Encode(&data, pixels, nil)
	} else {
		err = png.Encode(&data, pixels)
	}
	if err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func playwrightBackend(t *testing.T, tools []*mcp.Tool, handler mcp.ToolHandler) *mcp.ClientSession {
	t.Helper()
	srv := mcp.NewServer(&mcp.Implementation{Name: "Playwright", Version: PlaywrightUpstreamVersion}, nil)
	for _, tool := range tools {
		srv.AddTool(tool, handler)
	}
	return testClient(t, srv)
}

func playwrightFrontend(t *testing.T, backend *mcp.ClientSession, r *Runtime) *mcp.ClientSession {
	t.Helper()
	srv, err := NewPlaywrightServer(context.Background(), backend, r, PlaywrightOptions{Login: "trusted-login", Password: "trusted-password"}, "adapter-test")
	if err != nil {
		t.Fatal(err)
	}
	return testClient(t, srv)
}

func TestPlaywrightProtocolAllowlist(t *testing.T) {
	var calls []string
	backend := playwrightBackend(t, playwrightFixtureTools(), func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		calls = append(calls, req.Params.Name)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "snapshot ref=e12"}}}, nil
	})
	r := playwrightRuntime(t)
	client := playwrightFrontend(t, backend, r)
	list, err := client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != len(playwrightFields)+2 {
		t.Fatalf("unexpected tools: %v", list.Tools)
	}
	for _, tool := range list.Tools {
		if tool.Annotations == nil || tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint {
			t.Fatalf("missing conservative annotations: %+v", tool)
		}
		if _, allowed := playwrightFields[tool.Name]; allowed {
			if tool.Annotations.ReadOnlyHint {
				t.Fatalf("trusted backend hint: %s", tool.Name)
			}
			schema := tool.InputSchema.(map[string]any)
			props := schema["properties"].(map[string]any)
			for _, banned := range []string{"filename", "path", "output", "filter", "all"} {
				if _, ok := props[banned]; ok {
					t.Fatalf("advertised %s on %s", banned, tool.Name)
				}
			}
			if tool.Name == "browser_console_messages" {
				p := props["level"].(map[string]any)
				if p["default"] != "info" || p["description"] != "Official level" || len(p["enum"].([]any)) != 4 {
					t.Fatalf("native schema lost: %v", p)
				}
			}
			if tool.Name == "browser_take_screenshot" {
				if !reflect.DeepEqual(props["fullPage"].(map[string]any)["enum"], []any{false}) || !reflect.DeepEqual(props["scale"].(map[string]any)["enum"], []any{"css"}) {
					t.Fatalf("unbounded screenshot advertised: %v", props)
				}
			}
			if tool.Name == "browser_snapshot" {
				depth := props["depth"].(map[string]any)
				if depth["maximum"] != float64(8) || depth["default"] != float64(8) {
					t.Fatalf("unbounded snapshot advertised: %v", depth)
				}
			}
		}
	}
	for _, tool := range playwrightFixtureTools() {
		if _, allowed := playwrightFields[tool.Name]; allowed {
			continue
		}
		if result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: tool.Name, Arguments: map[string]any{}}); err == nil && !result.IsError {
			t.Fatalf("direct unsafe call accepted: %s", tool.Name)
		}
	}
	if len(calls) != 0 {
		t.Fatalf("unsafe tools forwarded: %v", calls)
	}
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "browser_context", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("context: %v %v", result, err)
	}
	data, _ := json.Marshal(result)
	for _, expected := range []string{"adapter-test", PlaywrightUpstreamVersion, "private-proxy", "cdn.example.org", "not read-only", "never production"} {
		if !bytes.Contains(data, []byte(expected)) {
			t.Errorf("context missing %q: %s", expected, data)
		}
	}
	if bytes.Contains(data, []byte("trusted-password")) || bytes.Contains(data, []byte("trusted-login")) {
		t.Fatal("context leaked credentials")
	}
}

func TestPlaywrightArgumentConfinement(t *testing.T) {
	var forwarded []map[string]any
	backend := playwrightBackend(t, playwrightFixtureTools(), func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		_ = json.Unmarshal(req.Params.Arguments, &args)
		forwarded = append(forwarded, args)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
	})
	client := playwrightFrontend(t, backend, playwrightRuntime(t))
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"browser_snapshot", map[string]any{"filename": "../../secret"}},
		{"browser_take_screenshot", map[string]any{"path": "/tmp/output.png"}},
		{"browser_take_screenshot", map[string]any{"fullPage": true}},
		{"browser_take_screenshot", map[string]any{"scale": "device"}},
		{"browser_network_requests", map[string]any{"filter": "(a+)+$"}},
		{"browser_console_messages", map[string]any{"level": "log"}},
		{"browser_console_messages", map[string]any{"all": true}},
		{"browser_resize", map[string]any{"width": 319, "height": 800}},
		{"browser_resize", map[string]any{"width": 1921, "height": 800}},
		{"browser_resize", map[string]any{"width": 800, "height": 1201}},
		{"browser_resize", map[string]any{"width": 800.5, "height": 800}},
		{"browser_resize", map[string]any{"width": 800}},
		{"browser_wait_for", map[string]any{"time": 10.1}},
		{"browser_wait_for", map[string]any{"time": -1}},
		{"browser_wait_for", map[string]any{"text": strings.Repeat("x", 513)}},
		{"browser_wait_for", map[string]any{"script": "alert(1)"}},
		{"browser_snapshot", map[string]any{"depth": 9}},
		{"browser_snapshot", map[string]any{"depth": 1.5}},
		{"browser_snapshot", map[string]any{"boxes": "true"}},
		{"browser_snapshot", map[string]any{"target": strings.Repeat("x", 513)}},
		{"browser_close", map[string]any{"filename": "out"}},
		{"browser_navigate", map[string]any{"url": "https://cdn.example.org/"}},
	} {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
		if err == nil && !result.IsError {
			t.Errorf("accepted %s %v", tc.name, tc.args)
		}
	}
	if len(forwarded) != 0 {
		t.Fatalf("invalid arguments forwarded: %v", forwarded)
	}
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"browser_navigate", map[string]any{"url": "http://localhost:18069/web#action=12"}},
		{"browser_snapshot", map[string]any{"depth": 8, "boxes": true, "target": "e12"}},
		{"browser_resize", map[string]any{"width": 1920, "height": 240}},
		{"browser_wait_for", map[string]any{"time": 0}},
		{"browser_wait_for", map[string]any{"time": 0.5, "textGone": "Loading"}},
		{"browser_take_screenshot", map[string]any{"type": "png", "scale": "css"}},
		{"browser_network_requests", map[string]any{"static": true}},
	} {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
		if err != nil || result.IsError {
			t.Fatalf("valid %s rejected: %v %v", tc.name, result, err)
		}
	}
	if forwarded[0]["url"] != "http://odoo:8069/web#action=12" {
		t.Fatalf("wrong navigation mapping: %v", forwarded[0])
	}
	if forwarded[5]["scale"] != "css" {
		t.Fatalf("screenshot DPR not bounded: %v", forwarded[5])
	}
	if forwarded[5]["fullPage"] != false {
		t.Fatalf("viewport policy not sent to backend: %v", forwarded[5])
	}
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "browser_snapshot", Arguments: map[string]any{}})
	if err != nil || result.IsError || forwarded[len(forwarded)-1]["depth"] != float64(8) {
		t.Fatalf("snapshot default not bounded: %v %v %v", result, err, forwarded)
	}
}

func TestPlaywrightNavigationOrigins(t *testing.T) {
	r := playwrightRuntime(t)
	for raw, want := range map[string]string{
		"/web": "http://odoo:8069/web", "web": "http://odoo:8069/web", "/web?debug=1#action=2": "http://odoo:8069/web?debug=1#action=2",
		"http://localhost:18069/web": "http://odoo:8069/web", "http://odoo:8069/odoo": "http://odoo:8069/odoo",
	} {
		got, err := playwrightNavigateURL(raw, r)
		if err != nil || got != want {
			t.Errorf("%q: %q %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "//evil.org", "%2f%2fevil.org", "/%2fevil.org", "/%252fevil.org", "https://cdn.example.org", "http://localhost:18070/web", "http://127.0.0.1:18069/web", "http://odoo:80/", "https://odoo:8069/", "http://localhost:18069.evil.org/", "http://user:pass@odoo:8069/", "http://odoo:8069@evil.org/", "http://odoo:8069\\@evil.org/", "/web%5cfoo", "/web%255cfoo", "/web%0afoo", "/web\tfoo", "javascript:alert(1)", "data:text/html,x", "file:///etc/passwd"} {
		if got, err := playwrightNavigateURL(raw, r); err == nil {
			t.Errorf("accepted %q -> %q", raw, got)
		}
	}
}

func TestPlaywrightNativeRequiredDefaults(t *testing.T) {
	tools := playwrightFixtureTools()
	for _, tool := range tools {
		if tool.Name == "browser_console_messages" {
			tool.InputSchema.(map[string]any)["required"] = []string{"level"}
		}
	}
	calls := 0
	backend := playwrightBackend(t, tools, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		_ = json.Unmarshal(req.Params.Arguments, &args)
		if args["level"] != "info" {
			t.Error("native default not forwarded")
		}
		if _, exists := args["all"]; exists {
			t.Error("prohibited property default forwarded")
		}
		calls++
		return &mcp.CallToolResult{}, nil
	})
	client := playwrightFrontend(t, backend, playwrightRuntime(t))
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "browser_console_messages", Arguments: map[string]any{}})
	if err != nil || result.IsError || calls != 1 {
		t.Fatal("required native default rejected")
	}
}

func TestPlaywrightStartupFailsClosed(t *testing.T) {
	for _, mode := range []string{"missing", "rename", "required-filename", "required-unknown", "invalid-schema"} {
		t.Run(mode, func(t *testing.T) {
			tools := playwrightFixtureTools()
			for i, tool := range tools {
				if tool.Name == "browser_snapshot" {
					switch mode {
					case "missing":
						tools = append(tools[:i], tools[i+1:]...)
					case "rename":
						tool.Name = "browser_new_snapshot"
					case "required-filename":
						tool.InputSchema.(map[string]any)["required"] = []string{"filename"}
					case "required-unknown":
						tool.InputSchema.(map[string]any)["required"] = []string{"script"}
					case "invalid-schema":
						tool.InputSchema.(map[string]any)["properties"] = "invalid"
					}
					break
				}
			}
			backend := playwrightBackend(t, tools, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) { return nil, nil })
			if _, err := NewPlaywrightServer(context.Background(), backend, playwrightRuntime(t), PlaywrightOptions{Public: true}, "test"); err == nil {
				t.Fatal("incompatible backend accepted")
			}
		})
	}
	for _, opts := range []PlaywrightOptions{{}, {Login: "login"}, {Password: "password"}} {
		if err := playwrightOptions(opts); err == nil {
			t.Errorf("incomplete credentials accepted: %v", opts)
		}
	}
	if err := playwrightOptions(PlaywrightOptions{Public: true, Login: "ignored"}); err != nil {
		t.Fatal(err)
	}
}

func TestPlaywrightNativeResults(t *testing.T) {
	r := playwrightRuntime(t)
	image := playwrightImage(t, "png", 16, 12)
	backend := playwrightBackend(t, playwrightFixtureTools(), func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.TextContent{Text: "ref=e12 trusted-login trusted-password token=hide-me https://u:p@odoo:8069/web?private=query#secret /artifacts/page.png"},
			&mcp.ImageContent{MIMEType: "image/png", Data: image},
		}, StructuredContent: map[string]any{"ref": "e12", "password": "hide-me", "url": "http://u:p@odoo:8069/web?x=hide-me#hide-me", "nested": []any{"trusted-password"}}}, nil
	})
	client := playwrightFrontend(t, backend, r)
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "browser_take_screenshot", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("result: %v %v", result, err)
	}
	if !bytes.Equal(result.Content[1].(*mcp.ImageContent).Data, image) {
		t.Fatal("image bytes changed")
	}
	data, _ := json.Marshal(result)
	for _, secret := range []string{"trusted-login", "trusted-password", "hide-me", "private=query", "#secret", "u:p@"} {
		if bytes.Contains(data, []byte(secret)) {
			t.Errorf("leaked %q: %s", secret, data)
		}
	}
	if !strings.Contains(result.Content[0].(*mcp.TextContent).Text, r.ArtifactsDir) || !bytes.Contains(data, []byte("ref=e12")) {
		t.Fatalf("paths or refs lost: %s", data)
	}
	if !reflect.DeepEqual(result.StructuredContent.(map[string]any)["nested"], []any{"[REDACTED]"}) {
		t.Fatalf("structured content lost: %+v", result.StructuredContent)
	}
	var metadata struct {
		ImageArtifact struct {
			Path          string
			Exported      bool
			SizeBytes     int `json:"size_bytes"`
			SHA256        string
			Width, Height int
			MIMEType      string `json:"mime_type"`
		} `json:"image_artifact"`
	}
	if err := json.Unmarshal([]byte(result.Content[len(result.Content)-1].(*mcp.TextContent).Text), &metadata); err != nil {
		t.Fatal(err)
	}
	artifact := metadata.ImageArtifact
	stored, err := os.ReadFile(artifact.Path)
	if err != nil || !bytes.Equal(stored, image) {
		t.Fatalf("native image not exported intact: %v", err)
	}
	info, err := os.Stat(artifact.Path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("export permissions: %v %v", info, err)
	}
	if !artifact.Exported || artifact.SizeBytes != len(image) || artifact.Width != 16 || artifact.Height != 12 || artifact.MIMEType != "image/png" || artifact.SHA256 != fmt.Sprintf("%x", sha256.Sum256(image)) {
		t.Fatalf("incorrect image provenance: %+v", artifact)
	}
	if !strings.Contains(result.Content[0].(*mcp.TextContent).Text, artifact.Path) || filepath.Base(artifact.Path) == "page.png" {
		t.Fatal("backend filename was trusted or reference not rewritten")
	}
}

func TestPlaywrightResultLimits(t *testing.T) {
	for _, result := range []*mcp.CallToolResult{
		{Content: []mcp.Content{&mcp.TextContent{Text: strings.Repeat("x", (1<<20)+1)}}},
		{Content: []mcp.Content{&mcp.TextContent{Text: strings.Repeat("x", 600000)}, &mcp.TextContent{Text: strings.Repeat("y", 600000)}}},
		{Content: []mcp.Content{&mcp.ImageContent{MIMEType: "image/png", Data: make([]byte, browserImageLimit+1)}}},
		{Content: []mcp.Content{&mcp.ImageContent{MIMEType: "image/png", Data: make([]byte, browserImageLimit)}, &mcp.ImageContent{MIMEType: "image/png", Data: []byte{1}}}},
		{Content: []mcp.Content{&mcp.ImageContent{MIMEType: "text/html", Data: []byte("unsafe")}}},
		{StructuredContent: map[string]any{"body": strings.Repeat("x", 1<<20)}},
		{Content: []mcp.Content{&mcp.TextContent{Text: strings.Repeat("s", 100000)}, &mcp.TextContent{Text: strings.Repeat("s", 100000)}}},
	} {
		if _, err := playwrightResult(result, "", []string{"s"}); err == nil {
			t.Fatal("unbounded result accepted")
		}
	}
	result, err := playwrightResult(&mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "password=secret"}}}, "", nil)
	if err != nil || !result.IsError || strings.Contains(result.Content[0].(*mcp.TextContent).Text, "secret") {
		t.Fatalf("tool error mishandled: %v %v", result, err)
	}
}

func TestPlaywrightSessionArtifacts(t *testing.T) {
	r := playwrightRuntime(t)
	r.BrowserNetwork = "odooctl-browser-0123456789abcdef"
	token := strings.Repeat("a", 24)
	worker := r.BrowserNetwork + "-worker-" + token
	sessionDir := filepath.Join(r.ArtifactsDir, "session-"+token)
	if err := os.Mkdir(sessionDir, 0700); err != nil {
		t.Fatal(err)
	}
	session, err := playwrightWorkerRuntime(r, worker)
	if err != nil || session.ArtifactsDir != sessionDir || r.ArtifactsDir == session.ArtifactsDir {
		t.Fatalf("session directory: %+v %v", session, err)
	}
	for _, name := range []string{"", "../secret", "odooctl-browser-fedcba9876543210-worker-" + token, worker + "/../secret", worker + "-extra"} {
		if _, err := playwrightWorkerRuntime(r, name); err == nil {
			t.Fatalf("unmanaged worker accepted: %v", name)
		}
	}
	result, err := playwrightResult(&mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "[Snapshot](/artifacts/page.yaml) /artifacts/console.log /artifacts/network.txt /artifacts/page.text [Trace](file:///artifacts/trace.log) [Screenshot](/artifacts/page.png) http://odoo:8069/artifacts/page.png?secret=1 /artifacts/../secret"}}}, sessionDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if strings.Count(text, "[ephemeral text artifact not exported]") != 5 || !strings.Contains(text, "[ephemeral image artifact not exported]") || strings.Contains(text, sessionDir) || !strings.Contains(text, "http://odoo:8069/artifacts/page.png") || strings.Contains(text, "../secret") || strings.Contains(text, "secret=1") {
		t.Fatalf("artifact path or URL mapping: %s", text)
	}
	entries, err := os.ReadDir(sessionDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("text artifacts exported: %v %v", entries, err)
	}
}

func TestPlaywrightImageValidation(t *testing.T) {
	pngImage := playwrightImage(t, "png", 32, 24)
	jpegImage := playwrightImage(t, "jpeg", 32, 24)
	for _, tc := range []struct {
		name, mime string
		data       []byte
	}{
		{"fake-png", "image/png", []byte("not a screenshot")},
		{"png-header-only", "image/png", pngImage[:33]},
		{"jpeg-truncated", "image/jpeg", jpegImage[:len(jpegImage)/2]},
		{"mime-mismatch", "image/png", jpegImage},
		{"too-wide", "image/png", playwrightImage(t, "png", 1921, 1)},
		{"too-tall", "image/png", playwrightImage(t, "png", 1, 1201)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			_, err := playwrightResult(&mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{Data: tc.data, MIMEType: tc.mime}}}, dir, nil)
			if err == nil {
				t.Fatal("invalid native image accepted")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid image written to host: %v %v", entries, err)
			}
		})
	}
	for _, format := range []string{"png", "jpeg"} {
		t.Run("valid-"+format, func(t *testing.T) {
			dir := t.TempDir()
			data := pngImage
			if format == "jpeg" {
				data = jpegImage
			}
			for i := 0; i < 2; i++ {
				result, err := playwrightResult(&mcp.CallToolResult{Content: []mcp.Content{
					&mcp.TextContent{Text: "[Screenshot](/artifacts/page." + format + ") /artifacts/page." + format + " /artifacts/snapshot.yaml /artifacts/console.log"},
					&mcp.ImageContent{Data: data, MIMEType: "image/" + format},
				}}, dir, nil)
				if err != nil || !bytes.Equal(result.Content[1].(*mcp.ImageContent).Data, data) {
					t.Fatalf("native content: %v %v", result, err)
				}
				text := result.Content[0].(*mcp.TextContent).Text
				if strings.Count(text, dir) != 2 || strings.Count(text, "[ephemeral text artifact not exported]") != 2 {
					t.Fatalf("incorrect path rewriting: %s", text)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 2 {
				t.Fatalf("random exports: %v %v", entries, err)
			}
			for _, entry := range entries {
				if !strings.HasPrefix(entry.Name(), "screenshot-") || !strings.HasSuffix(entry.Name(), "."+format) {
					t.Fatalf("backend filename used: %s", entry.Name())
				}
				stored, err := os.ReadFile(filepath.Join(dir, entry.Name()))
				if err != nil || !bytes.Equal(stored, data) {
					t.Fatalf("export changed bytes: %v", err)
				}
			}
		})
	}
}

func TestPlaywrightImageExportFailClosed(t *testing.T) {
	data := playwrightImage(t, "png", 24, 16)
	dir := t.TempDir()
	for _, result := range []*mcp.CallToolResult{
		{Content: []mcp.Content{&mcp.ImageContent{Data: data, MIMEType: "image/png"}, &mcp.TextContent{Text: strings.Repeat("x", 1<<20)}}},
		{Content: []mcp.Content{&mcp.ImageContent{Data: data, MIMEType: "image/png"}}, StructuredContent: map[string]any{"log": strings.Repeat("x", 1<<20)}},
		{Content: []mcp.Content{&mcp.ImageContent{Data: data, MIMEType: "image/png"}, &mcp.ImageContent{Data: []byte("bad"), MIMEType: "image/png"}}},
	} {
		if _, err := playwrightResult(result, dir, nil); err == nil {
			t.Fatal("invalid/oversized result accepted")
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			t.Fatalf("partial result exported: %v %v", entries, err)
		}
	}
	result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{Data: data, MIMEType: "image/png"}}}
	if _, err := playwrightResult(result, filepath.Join(dir, "missing"), nil); err == nil {
		t.Fatal("missing export directory silently ignored")
	}
	if _, err := playwrightResult(result, "relative", nil); err == nil {
		t.Fatal("relative export directory accepted")
	}
	result.Content = append(result.Content, &mcp.TextContent{Text: "/artifacts/image.png /artifacts/snapshot.yaml"})
	native, err := playwrightResult(result, "", nil)
	if err != nil || !bytes.Equal(native.Content[0].(*mcp.ImageContent).Data, data) {
		t.Fatalf("in-memory native image: %v %v", native, err)
	}
	text := native.Content[1].(*mcp.TextContent).Text
	if !strings.Contains(text, "[ephemeral image artifact not exported]") || !strings.Contains(text, "[ephemeral text artifact not exported]") {
		t.Fatalf("in-memory result advertised inaccessible paths: %s", text)
	}
	metadata := native.Content[2].(*mcp.TextContent).Text
	if !strings.Contains(metadata, `"exported":false`) || !strings.Contains(metadata, `"path":""`) {
		t.Fatalf("in-memory result claimed export: %s", metadata)
	}
}

func TestPlaywrightActiveCallCancellation(t *testing.T) {
	entered, cancelled := make(chan struct{}), make(chan struct{})
	backend := playwrightBackend(t, playwrightFixtureTools(), func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		return nil, ctx.Err()
	})
	client := playwrightFrontend(t, backend, playwrightRuntime(t))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "browser_snapshot", Arguments: map[string]any{}})
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("backend call not entered")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("active call ignored cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("frontend call not cancelled")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("RPC cancellation did not reach backend")
	}
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "browser_snapshot", Arguments: map[string]any{}})
	if err != nil || !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "restart") {
		t.Fatalf("cancelled session reused: %v %v", result, err)
	}
}

func TestPlaywrightSessionFailureNoRetry(t *testing.T) {
	for _, transportFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "tool-error", true: "rpc-error"}[transportFailure], func(t *testing.T) {
			calls := 0
			backend := playwrightBackend(t, playwrightFixtureTools(), func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				calls++
				if transportFailure {
					return nil, errors.New("trusted-password backend private failure")
				}
				return playwrightError("trusted-password page navigation failed"), nil
			})
			client := playwrightFrontend(t, backend, playwrightRuntime(t))
			for i := 0; i < 2; i++ {
				result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "browser_navigate", Arguments: map[string]any{"url": "/web"}})
				if err != nil || !result.IsError {
					t.Fatalf("error: %v %v", result, err)
				}
				data, _ := json.Marshal(result)
				if bytes.Contains(data, []byte("trusted-password")) {
					t.Fatal("error leaked credential")
				}
			}
			if calls != 1 {
				t.Fatalf("uncertain operation retried: %d", calls)
			}
		})
	}
}

func TestPlaywrightSerializesSessionAndCancellation(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var mu sync.Mutex
	active, maxActive := 0, 0
	backend := playwrightBackend(t, playwrightFixtureTools(), func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		defer func() { mu.Lock(); active--; mu.Unlock() }()
		entered <- struct{}{}
		select {
		case <-release:
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	client := playwrightFrontend(t, backend, playwrightRuntime(t))
	done := make(chan error, 2)
	go func() {
		_, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "browser_snapshot", Arguments: map[string]any{}})
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("backend not entered")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "browser_snapshot", Arguments: map[string]any{}})
	if err == nil {
		t.Fatal("queued call ignored cancellation")
	}
	go func() {
		_, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "browser_snapshot", Arguments: map[string]any{}})
		done <- err
	}()
	close(release)
	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("calls did not finish")
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if maxActive != 1 {
		t.Fatalf("concurrent backend calls: %d", maxActive)
	}
}
