package mcpsrv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Opt-in observation of an already prepared disposable fixture. Never prepares
// a runtime, starts services, or interacts with business records.
func TestLivePlaywright(t *testing.T) {
	project := os.Getenv("ODOOCTL_PLAYWRIGHT_TEST_PROJECT")
	if project == "" {
		t.Skip("set ODOOCTL_PLAYWRIGHT_TEST_PROJECT to a prepared disposable fixture")
	}
	login, password := os.Getenv("ODOOCTL_MCP_BROWSER_LOGIN"), os.Getenv("ODOOCTL_MCP_BROWSER_PASSWORD")
	if pair := os.Getenv("ODOOCTL_PLAYWRIGHT_TEST_LOGIN"); pair != "" {
		var ok bool
		login, password, ok = strings.Cut(pair, ":")
		if !ok || login == "" || password == "" {
			t.Fatal("test login must be a nonempty login:password pair")
		}
	}
	for _, mode := range []string{"authenticated", "public"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
			defer cancel()
			args := []string{"run", "../..", "mcp", "browser", "--project", project}
			if mode == "public" {
				args = append(args, "--public")
			}
			cmd := exec.CommandContext(ctx, "go", args...)
			cmd.Stderr = &cappedBuffer{limit: 64 << 10}
			for _, env := range os.Environ() {
				if !strings.HasPrefix(env, "ODOOCTL_MCP_BROWSER_LOGIN=") && !strings.HasPrefix(env, "ODOOCTL_MCP_BROWSER_PASSWORD=") && !strings.HasPrefix(env, "ODOOCTL_PLAYWRIGHT_TEST_LOGIN=") {
					cmd.Env = append(cmd.Env, env)
				}
			}
			if mode == "authenticated" {
				// Leave unset values unset so this exercises the CLI's defaults.
				if login != "" {
					cmd.Env = append(cmd.Env, "ODOOCTL_MCP_BROWSER_LOGIN="+login)
				}
				if password != "" {
					cmd.Env = append(cmd.Env, "ODOOCTL_MCP_BROWSER_PASSWORD="+password)
				}
			}
			client := mcp.NewClient(&mcp.Implementation{Name: "playwright-live-test", Version: "test"}, nil)
			session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: 15 * time.Second}, nil)
			if err != nil {
				t.Fatal("stdio handshake failed (details withheld)")
			}
			worker := ""
			defer func() {
				if session.Close() != nil || cmd.ProcessState == nil || !cmd.ProcessState.Success() {
					t.Error("stdio EOF cleanup failed (details withheld)")
				}
				if worker != "" {
					cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
					defer stop()
					out, err := exec.CommandContext(cleanup, "docker", "ps", "-a", "--filter", "name=^/"+worker+"$", "--format", "{{.ID}}").Output()
					if err != nil || len(bytes.TrimSpace(out)) != 0 {
						t.Error("session worker remains after stdio EOF")
					}
				}
			}()
			call := func(name string, arguments map[string]any, denied bool) *mcp.CallToolResult {
				t.Helper()
				deadline, stop := context.WithTimeout(ctx, 65*time.Second)
				defer stop()
				start := time.Now()
				result, err := session.CallTool(deadline, &mcp.CallToolParams{Name: name, Arguments: arguments})
				failed := err != nil || result == nil || result.IsError
				t.Logf("tool=%s error=%t elapsed_ms=%d", name, failed, time.Since(start).Milliseconds())
				if failed != denied {
					// Only fixed status classifications, never upstream errors or bodies.
					status := "unexpected_success"
					if failed {
						status = "rpc_error"
						if err == nil && result != nil {
							status = "tool_error"
							for _, c := range result.Content {
								if text, ok := c.(*mcp.TextContent); ok && strings.Contains(text.Text, "official Playwright schema") {
									status = "native_schema_error"
								}
								if text, ok := c.(*mcp.TextContent); ok && strings.Contains(text.Text, "Timeout") {
									status = "backend_timeout"
								}
								if text, ok := c.(*mcp.TextContent); ok && strings.Contains(text.Text, "Odoo browser login failed") {
									status = "runtime_login_failed"
								}
							}
						}
					}
					t.Fatalf("tool=%s status=%s", name, status)
				}
				return result
			}
			text := func(result *mcp.CallToolResult) string {
				var out strings.Builder
				for _, c := range result.Content {
					if c, ok := c.(*mcp.TextContent); ok {
						out.WriteString(c.Text)
					}
				}
				data, _ := json.Marshal(result.StructuredContent)
				out.Write(data)
				return out.String()
			}
			list, err := session.ListTools(ctx, nil)
			if err != nil || list.NextCursor != "" || len(list.Tools) != 10 {
				t.Fatal("tool registry mismatch")
			}
			seen := map[string]bool{}
			for _, tool := range list.Tools {
				_, approved := playwrightFields[tool.Name]
				if seen[tool.Name] || (!approved && tool.Name != "project_context" && tool.Name != "browser_context") {
					t.Fatal("unexpected or duplicate tool")
				}
				seen[tool.Name] = true
			}
			config := call("browser_context", map[string]any{}, false)
			data, _ := json.Marshal(config.StructuredContent)
			var provenance struct {
				Origins   []string `json:"allowed_asset_origins"`
				Artifacts string   `json:"artifacts_directory"`
				Public    bool     `json:"public"`
				Deadline  int      `json:"tool_deadline_seconds"`
				Network   string   `json:"browser_network"`
			}
			if json.Unmarshal(data, &provenance) != nil || provenance.Public != (mode == "public") || provenance.Deadline != 60 || !filepath.IsAbs(provenance.Artifacts) {
				t.Fatal("invalid structured runtime provenance")
			}
			worker = provenance.Network + "-worker-" + strings.TrimPrefix(filepath.Base(provenance.Artifacts), "session-")
			for _, origin := range []string{"https://cdn.jsdelivr.net", "https://www.openstreetmap.org", "https://tile.openstreetmap.org", "https://127.0.0.1.nip.io"} {
				if !strings.Contains(strings.Join(provenance.Origins, " "), origin) {
					t.Fatal("fixture approved origin missing")
				}
			}
			defer func() {
				entries, err := os.ReadDir(provenance.Artifacts)
				if err != nil {
					t.Error("cannot inspect managed exports")
					return
				}
				for _, entry := range entries {
					if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".png") && !strings.HasSuffix(entry.Name(), ".jpeg")) {
						t.Error("non-image host artifact exported")
					}
				}
				t.Logf("exported_images=%d", len(entries))
			}()
			info, err := os.Stat(provenance.Artifacts)
			if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
				t.Fatal("managed session directory is not private")
			}
			for _, name := range []string{"browser_evaluate", "browser_run_code_unsafe", "browser_click", "browser_type", "browser_fill_form", "browser_file_upload", "browser_network_request", "browser_get_config", "browser_route", "browser_cookie_list", "browser_tabs", "browser_handle_dialog", "browser_page_webmcp", "bodyReadModel.Table.Meta", "../browser_snapshot"} {
				call(name, map[string]any{}, true)
			}
			for _, tc := range []struct {
				name string
				args map[string]any
			}{
				{"browser_snapshot", map[string]any{"filename": "out.txt"}},
				{"browser_take_screenshot", map[string]any{"filename": "out.png"}},
				{"browser_network_requests", map[string]any{"filename": "out.txt"}},
				{"browser_console_messages", map[string]any{"filename": "out.txt"}},
				{"browser_resize", map[string]any{"width": 1921, "height": 800}},
				{"browser_resize", map[string]any{"width": 390, "height": 1201}},
				{"browser_snapshot", map[string]any{"depth": 9}},
				{"browser_wait_for", map[string]any{"time": 11}},
				{"browser_take_screenshot", map[string]any{"fullPage": true}},
			} {
				call(tc.name, tc.args, true)
			}
			for _, destination := range []string{"https://cdn.jsdelivr.net/", "//www.openstreetmap.org/", "http://127.0.0.1/", "http://10.0.0.1/", "https://127.0.0.1.nip.io/", "file:///etc/passwd", "javascript:void(0)"} {
				call("browser_navigate", map[string]any{"url": destination}, true)
			}
			if mode == "authenticated" {
				call("browser_navigate", map[string]any{"url": "/web"}, false)
				call("browser_wait_for", map[string]any{"text": "Official Apps"}, false)
				snapshot := text(call("browser_snapshot", map[string]any{"boxes": true}, false))
				// The pinned backend uses frame-scoped references such as f1e1.
				if len(snapshot) < 2000 || !strings.Contains(snapshot, "Official Apps") || !regexp.MustCompile(`ref=(?:f\d+)?e\d+`).MatchString(snapshot) || !strings.Contains(snapshot, "box=") {
					t.Error("authenticated snapshot lacks meaningful refs/boxes (contents withheld)")
				}
				t.Logf("snapshot_bytes=%d", len(snapshot))
				return
			}
			call("browser_navigate", map[string]any{"url": "/mcp_probe/static/browser-fidelity.html"}, false)
			call("browser_wait_for", map[string]any{"time": 10}, false)
			call("browser_wait_for", map[string]any{"text": "Three.js scene rendered"}, false)
			snapshot := text(call("browser_snapshot", map[string]any{"boxes": true}, false))
			if !regexp.MustCompile(`(?i)region[^\n]*map`).MatchString(snapshot) {
				t.Error("approved iframe child Map role missing")
			}
			for index, marker := range []string{"MCP Browser Fidelity Fixture", "Three.js scene rendered", "OpenStreetMap", "Map"} {
				if !strings.Contains(snapshot, marker) {
					t.Errorf("fidelity snapshot marker_index=%d missing (contents withheld)", index)
				}
			}
			network := text(call("browser_network_requests", map[string]any{"static": true}, false))
			// Native ARIA snapshots expose iframe children, not iframe src attributes.
			// Verify the frame source independently in the native network summary.
			if !strings.Contains(network, "www.openstreetmap.org/export/embed.html") {
				t.Error("approved iframe source missing from network summary")
			}
			for _, host := range []string{"cdn.jsdelivr.net", "www.openstreetmap.org", "tile.openstreetmap.org"} {
				matched := false
				for _, line := range strings.Split(network, "\n") {
					if strings.Contains(line, host) && strings.Contains(line, "200") {
						matched = true
					}
				}
				if !matched {
					t.Error("approved asset origin lacks successful network evidence (contents withheld)")
				}
			}
			for _, size := range [][2]int{{1280, 800}, {390, 844}} {
				call("browser_resize", map[string]any{"width": size[0], "height": size[1]}, false)
				result := call("browser_take_screenshot", map[string]any{"type": "png", "scale": "css", "fullPage": false}, false)
				images := 0
				for _, c := range result.Content {
					c, ok := c.(*mcp.ImageContent)
					if !ok {
						continue
					}
					images++
					pixels, format, err := image.Decode(bytes.NewReader(c.Data))
					if err != nil || c.MIMEType != "image/"+format || pixels.Bounds().Dx() != size[0] || pixels.Bounds().Dy() != size[1] {
						t.Fatal("native image MIME/dimensions mismatch")
					}
					green := 0
					for y := 0; y < size[1]; y++ {
						for x := 0; x < size[0]; x++ {
							r, g, b, _ := pixels.At(x, y).RGBA()
							if g > 45000 && r < 16000 && b < 16000 {
								green++
							}
						}
					}
					if green < 500 {
						t.Error("rendered Three.js cube missing from native screenshot")
					}
					verified := false
					for _, content := range result.Content {
						msg, ok := content.(*mcp.TextContent)
						if !ok {
							continue
						}
						var meta struct {
							Artifact struct {
								Path, SHA256 string
								Exported     bool
							} `json:"image_artifact"`
						}
						if json.Unmarshal([]byte(msg.Text), &meta) != nil || meta.Artifact.Path == "" {
							continue
						}
						stored, err := os.ReadFile(meta.Artifact.Path)
						info, statErr := os.Stat(meta.Artifact.Path)
						if err != nil || statErr != nil || !meta.Artifact.Exported || filepath.Dir(meta.Artifact.Path) != provenance.Artifacts || info.Mode().Perm() != 0600 || !bytes.Equal(stored, c.Data) || meta.Artifact.SHA256 != fmt.Sprintf("%x", sha256.Sum256(c.Data)) {
							t.Fatal("managed native export provenance mismatch")
						}
						verified = true
					}
					if !verified {
						t.Error("managed image export metadata missing")
					}
					t.Logf("image_width=%d image_height=%d image_bytes=%d green_pixels=%d", size[0], size[1], len(c.Data), green)
				}
				if images != 1 {
					t.Error("expected exactly one native MCP image")
				}
			}
			// No screenshots of privacy controls: pixels are not redacted.
			for index, result := range []*mcp.CallToolResult{
				call("browser_navigate", map[string]any{"url": "/mcp_probe/static/browser-privacy.html"}, false),
				call("browser_wait_for", map[string]any{"text": "Privacy fixture ready"}, false),
				call("browser_snapshot", map[string]any{"boxes": true}, false),
			} {
				observation := text(result)
				if index == 2 && !strings.Contains(observation, "Sensitive frame ready") {
					t.Error("privacy snapshot did not observe nested frame")
				}
				for _, sentinel := range []string{"fixture-private-password-74291", "fixture-private-input-85302", "fixture-private-textarea-96413", "fixture-private-frame-17524"} {
					if strings.Contains(observation, sentinel) {
						t.Error("runtime privacy producer leaked a control sentinel (contents withheld)")
					}
				}
			}
			call("browser_navigate", map[string]any{"url": "/mcp_probe/static/browser-denied.html"}, false)
			call("browser_wait_for", map[string]any{"text": "Private destination blocked"}, false)
			deniedNetwork := text(call("browser_network_requests", map[string]any{"static": true}, false))
			if !strings.Contains(deniedNetwork, "127.0.0.1.nip.io") || (!strings.Contains(deniedNetwork, "ERR_") && !strings.Contains(deniedNetwork, "403")) {
				t.Error("private destination lacks blocked network evidence")
			}
			call("browser_console_messages", map[string]any{}, false)
		})
	}
}
