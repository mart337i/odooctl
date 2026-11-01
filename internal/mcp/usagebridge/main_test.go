package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The test executable acts as an SDK stdio server, without a CLI fixture or Docker.
func TestMain(m *testing.M) {
	mode := os.Getenv("ODOOCTL_USAGEBRIDGE_TEST_SERVER")
	if mode == "" || len(os.Args) < 3 || os.Args[1] != "mcp" || os.Args[2] != "serve" {
		os.Exit(m.Run())
	}
	flags := flag.NewFlagSet("fixture", flag.ExitOnError)
	project := flags.String("project", "", "")
	for _, name := range []string{"sql-role", "sql-tables", "odoo-user-id", "models", "company-ids"} {
		flags.String(name, "", "")
	}
	flags.Bool("browser", false, "")
	_ = flags.Parse(os.Args[3:])
	server := mcp.NewServer(&mcp.Implementation{Name: "usagebridge-fixture", Version: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "project_context"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, map[string]any, error) {
		version := "17.0"
		if mode == "wrong-version" {
			version = "18.0"
		}
		return nil, map[string]any{"project": *project, "environment": map[string]any{"odoo_version": version}}, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "probe", Description: "stdio test probe"}, func(context.Context, *mcp.CallToolRequest, struct {
		Name string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{IsError: mode == "tool-error", Content: []mcp.Content{&mcp.TextContent{Text: "probe reached " + os.Getenv("ODOOCTL_MCP_BROWSER_PASSWORD")}}, StructuredContent: map[string]any{"reached": true}}, nil, nil
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestStdioBridge(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ODOOCTL_MCP_BROWSER_LOGIN", "fixture-login")
	t.Setenv("ODOOCTL_MCP_BROWSER_PASSWORD", "fixture-password")
	for _, mode := range []string{"success", "wrong-version", "tool-error"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("ODOOCTL_USAGEBRIDGE_TEST_SERVER", mode)
			var output bytes.Buffer
			args := []string{"--project", "/fixture", "--expected-version", "17.0", "--artifacts", t.TempDir(), "--binary", executable}
			code := run(append(args, "call", "probe", `{"name":"agent"}`), &output)
			var entry struct {
				Status string      `json:"status"`
				Result *toolOutput `json:"result"`
			}
			if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), "fixture-password") || strings.Contains(output.String(), "fixture-login") {
				t.Fatal("stdio output leaked fixture credentials")
			}
			switch mode {
			case "wrong-version":
				if code != 1 || entry.Status != "setup_error" || entry.Result != nil || strings.Contains(output.String(), "probe reached") {
					t.Fatalf("binding did not block call: %s", output.String())
				}
			case "tool-error":
				if code != 1 || entry.Status != "tool_error" || entry.Result == nil || !entry.Result.IsError || entry.Result.Native["isError"] != true {
					t.Fatalf("tool error lost: %s", output.String())
				}
			case "success":
				if code != 0 || entry.Status != "ok" || entry.Result == nil || len(entry.Result.Text) != 1 {
					t.Fatalf("call failed: %s", output.String())
				}
				output.Reset()
				if code := run(append(args, "list"), &output); code != 0 {
					t.Fatalf("list failed: %s", output.String())
				}
				if !strings.Contains(output.String(), `"inputSchema"`) || !strings.Contains(output.String(), `"outputSchema"`) || !strings.Contains(output.String(), `"probe"`) {
					t.Fatal("tool schemas omitted")
				}
			}
		})
	}
}

func TestDevelopmentLoginRequiresOptIn(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ODOOCTL_USAGEBRIDGE_TEST_SERVER", "success")
	t.Setenv("ODOOCTL_MCP_BROWSER_LOGIN", "")
	t.Setenv("ODOOCTL_MCP_BROWSER_PASSWORD", "")
	args := []string{"--project", "/fixture", "--expected-version", "17.0", "--artifacts", t.TempDir(), "--binary", executable}
	var output bytes.Buffer
	if code := run(append(args, "list"), &output); code != 1 || !strings.Contains(output.String(), `"status":"setup_error"`) {
		t.Fatal("missing credentials did not block connection")
	}
	output.Reset()
	if code := run(append(args, "--development-login", "call", "probe", `{"name":"agent"}`), &output); code != 0 {
		t.Fatalf("explicit development login failed: %s", output.String())
	}
	if strings.Contains(output.String(), "admin") || !strings.Contains(output.String(), "[REDACTED]") {
		t.Fatal("development credentials were not redacted")
	}
}

func TestImageHandling(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	var payload bytes.Buffer
	if err := png.Encode(&payload, image.NewRGBA(image.Rect(0, 0, 7, 3))); err != nil {
		t.Fatal(err)
	}
	result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "screenshot"}, &mcp.ImageContent{MIMEType: "image/png", Data: payload.Bytes()}}, StructuredContent: map[string]any{"ok": true}, IsError: true}
	output, err := convertResult(root, "../../browser_screenshot", result)
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Images) != 1 || !output.IsError || output.Native["isError"] != true {
		t.Fatalf("unexpected output: %+v", output)
	}
	metadata := output.Images[0]
	sum := sha256.Sum256(payload.Bytes())
	if metadata.Width != 7 || metadata.Height != 3 || metadata.Size != payload.Len() || metadata.MIME != "image/png" || metadata.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("wrong metadata: %+v", metadata)
	}
	if filepath.Dir(metadata.Path) != dir {
		t.Fatal("image escaped artifact directory")
	}
	info, err := os.Stat(metadata.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("image permissions: %o", info.Mode().Perm())
	}
	data, err := os.ReadFile(metadata.Path)
	if err != nil || !bytes.Equal(data, payload.Bytes()) {
		t.Fatal("artifact differs from native PNG payload")
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(base64.StdEncoding.EncodeToString(payload.Bytes()))) || bytes.Contains(encoded, []byte(`"data":`)) {
		t.Fatal("raw image included in output")
	}
	for _, bad := range []*mcp.ImageContent{{MIMEType: "image/jpeg", Data: payload.Bytes()}, {MIMEType: "image/png", Data: []byte("not png")}, {MIMEType: "image/png", Data: make([]byte, (2<<20)+1)}} {
		if _, err := saveImage(root, "test", bad); err == nil {
			t.Fatal("invalid image accepted")
		}
	}
}

func TestBinding(t *testing.T) {
	for _, test := range []struct {
		project, version string
		valid            bool
	}{{"/fixture", "17.0", true}, {"/other", "17.0", false}, {"/fixture/", "17.0", false}, {"/fixture", "18.0", false}, {"/fixture", "", false}} {
		value := map[string]any{"project": test.project, "environment": map[string]any{"odoo_version": test.version}}
		if err := validateBinding(value, "/fixture", "17.0"); (err == nil) != test.valid {
			t.Fatalf("binding %+v: %v", test, err)
		}
	}
	if err := validateBinding(nil, "/fixture", "17.0"); err == nil {
		t.Fatal("missing binding accepted")
	}
}

func TestArguments(t *testing.T) {
	for _, raw := range []string{`{}`, `{"query":"SELECT id FROM ir_module_module"}`} {
		if err := validateArguments(raw); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{"", "null", "[]", "1", `{"x":}`, `{} {}`, `{"x":1} trailing`} {
		if err := validateArguments(raw); err == nil {
			t.Fatalf("invalid JSON accepted: %q", raw)
		}
	}
}

func TestFailureTranscriptRedaction(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "artifacts")
	t.Setenv("ODOOCTL_MCP_BROWSER_LOGIN", "fixture-login")
	t.Setenv("ODOOCTL_MCP_BROWSER_PASSWORD", "fixture-password")
	var output bytes.Buffer
	code := run([]string{"--project", "/fixture", "--expected-version", "17.0", "--artifacts", dir, "--binary", "/nonexistent/odooctl", "call", "test", `{"note":"fixture-password fixture-login password=hidden Authorization: Bearer hidden-bearer", "cookie":"hidden-cookie"}`}, &output)
	if code != 1 {
		t.Fatal("missing binary succeeded")
	}
	transcript, err := os.ReadFile(filepath.Join(dir, "transcript.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(transcript, output.Bytes()) {
		t.Fatal("output differs from transcript")
	}
	for _, secret := range []string{"fixture-login", "fixture-password", "hidden-bearer", "hidden-cookie", "hidden"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("secret leaked: %s", secret)
		}
	}
	for path, mode := range map[string]os.FileMode{dir: 0700, filepath.Join(dir, "transcript.jsonl"): 0600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("wrong permissions for %s", path)
		}
	}
	if code := run([]string{"--project", "/fixture", "--expected-version", "17.0", "--artifacts", dir, "--binary", "/nonexistent/odooctl", "list"}, &bytes.Buffer{}); code != 1 {
		t.Fatal("missing binary succeeded")
	}
	transcript, _ = os.ReadFile(filepath.Join(dir, "transcript.jsonl"))
	if bytes.Count(transcript, []byte{'\n'}) != 2 {
		t.Fatal("transcript was not appended")
	}
}

func TestTranscriptRejectsSymlinksWithoutWriting(t *testing.T) {
	for _, outside := range []bool{false, true} {
		name := "inside"
		if outside {
			name = "outside"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			targetDir := dir
			if outside {
				targetDir = t.TempDir()
			}
			target := filepath.Join(targetDir, "target.jsonl")
			original := []byte("target must remain unchanged\n")
			if err := os.WriteFile(target, original, 0644); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(target)
			if err != nil {
				t.Fatal(err)
			}
			transcript := filepath.Join(dir, "transcript.jsonl")
			if err := os.Symlink(target, transcript); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			code := run([]string{"--project", dir, "--expected-version", "17.0", "--artifacts", dir, "--binary", filepath.Join(dir, "unused-binary"), "call", "test", "invalid JSON"}, &output)
			if code != 1 || !strings.Contains(output.String(), `"status":"transcript_error"`) {
				t.Fatalf("symlink transcript not rejected: %s", output.String())
			}
			data, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(data, original) {
				t.Fatal("symlink target was modified")
			}
			after, err := os.Stat(target)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatal("symlink target identity or permissions changed")
			}
			info, err := os.Lstat(transcript)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatal("transcript symlink was replaced")
			}
		})
	}
}

func TestTranscriptRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	transcript := filepath.Join(dir, "transcript.jsonl")
	if err := os.Mkdir(transcript, 0755); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := run([]string{"--project", dir, "--expected-version", "17.0", "--artifacts", dir, "--binary", filepath.Join(dir, "unused-binary"), "call", "test", "invalid JSON"}, &output)
	if code != 1 || !strings.Contains(output.String(), `"status":"transcript_error"`) {
		t.Fatalf("directory transcript not rejected: %s", output.String())
	}
	info, err := os.Stat(transcript)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0755 {
		t.Fatal("transcript directory was modified")
	}
}
