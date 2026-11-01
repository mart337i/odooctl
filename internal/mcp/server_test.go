package mcpsrv

import (
	"bytes"
	"context"
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

func testClient(t *testing.T, srv *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close(); server.Close() })
	return session
}

func TestLocalProtocolAndConfinement(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	sourceFixture(t, root, "demo/__manifest__.py", "{'name': 'Demo', 'depends': ['base']}")
	sourceFixture(t, root, "demo/models/main.py", "name = 'Demo'\napi_key = 'do-not-expose'\n")
	srv, err := New(Options{Project: root}, "test")
	if err != nil {
		t.Fatal(err)
	}
	client := testClient(t, srv)
	tools, err := client.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 5 {
		t.Fatalf("local tool discovery: %v, %v", tools, err)
	}
	for _, tool := range tools.Tools {
		if tool.InputSchema == nil || tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("missing schema or inspection annotation: %v", tool)
		}
	}
	for _, name := range []string{"project_context", "module_list"} {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
		if err != nil || result.IsError || result.StructuredContent == nil {
			t.Fatalf("%s result: %v %v", name, result, err)
		}
	}
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "code_read", Arguments: map[string]any{"root": "project", "path": "demo/models/main.py"}})
	if err != nil || result.IsError {
		t.Fatalf("read: %v %v", result, err)
	}
	encoded, _ := json.Marshal(result)
	if bytes.Contains(encoded, []byte("do-not-expose")) || !bytes.Contains(encoded, []byte("REDACTED")) {
		t.Fatalf("source secrets not redacted: %s", encoded)
	}
	result, err = client.CallTool(context.Background(), &mcp.CallToolParams{Name: "code_read", Arguments: map[string]any{"root": "project", "path": "../outside.py"}})
	if err != nil || !result.IsError {
		t.Fatalf("traversal must be a tool error: %v %v", result, err)
	}
	if _, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "code_read", Arguments: map[string]any{"root": "project"}}); err == nil {
		t.Fatal("missing required path accepted by schema")
	}
}

func TestRuntimeCapabilityDiscovery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	state := &config.State{ProjectName: "test", Branch: "main", ProjectRoot: root, OdooVersion: "18.0"}
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	srv, err := New(Options{Project: root, SQLRole: "reader", SQLTables: []string{"res_partner"}, OdooUserID: 7, Models: []string{"res.partner"}, BrowserEnabled: true, BrowserLogin: "dev", BrowserPassword: "very-private"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	client := testClient(t, srv)
	tools, err := client.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 14 {
		t.Fatalf("runtime tool discovery: %v %v", tools, err)
	}
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "project_context", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("context: %v %v", result, err)
	}
	data, _ := json.Marshal(result)
	if bytes.Contains(data, []byte("very-private")) || bytes.Contains(data, []byte("enterprise_github_token")) {
		t.Fatalf("context leaked raw configuration: %s", data)
	}
	link, _ := config.ProjectLinkPath(root)
	if _, err := os.Stat(link); !os.IsNotExist(err) {
		t.Fatalf("passive startup repaired project link: %v", err)
	}
	result, err = client.CallTool(context.Background(), &mcp.CallToolParams{Name: "database_query", Arguments: map[string]any{"query": "DELETE FROM res_partner"}})
	if err != nil || !result.IsError {
		t.Fatalf("mutation accepted: %v %v", result, err)
	}
}

func TestInvalidCapabilityConfiguration(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	for _, options := range []Options{
		{SQLRole: "odoo", SQLTables: []string{"res_partner"}},
		{SQLRole: "reader"}, {SQLTables: []string{"res_partner"}},
		{OdooUserID: 1, Models: []string{"res.partner"}},
		{OdooUserID: 2}, {Models: []string{"res.partner"}},
		{BrowserEnabled: true}, {ContainerSource: true},
	} {
		options.Project = root
		if _, err := New(options, "test"); err == nil {
			t.Fatalf("invalid configuration accepted: %#v", options)
		}
	}
}

func TestTextAndStructuredRedaction(t *testing.T) {
	for _, input := range []string{
		`{"password":"hidden-value"}`, `api_key='hidden-value'`, `Authorization: Bearer hidden-value`,
		`postgresql://user:hidden-value@localhost/db`, `password = hidden-value`,
	} {
		if strings.Contains(redactText(input), "hidden-value") {
			t.Errorf("secret retained: %s", input)
		}
	}
	input := map[string]any{"data": []any{map[string]any{"password": "hidden-value", "name": "Bearer hidden-value", "description": "known-password"}}}
	data, _ := json.Marshal(sanitize(input, []string{"known-password"}))
	if bytes.Contains(data, []byte("hidden-value")) || bytes.Contains(data, []byte("known-password")) {
		t.Fatalf("structured secret retained: %s", data)
	}
}

func TestCappedBuffer(t *testing.T) {
	cancelled := false
	b := &cappedBuffer{limit: 4, onLimit: func() { cancelled = true }}
	if n, err := b.Write([]byte("abcdef")); n != 6 || err != nil || b.String() != "abcd" || !b.full || !cancelled {
		t.Fatalf("incorrect output cap: %+v, %d %v", b, n, err)
	}
}

func TestRunnerStdinWatchdogAndEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix fake Docker fixture")
	}
	t.Setenv("HOME", t.TempDir())
	state := &config.State{ProjectName: "test", Branch: "main"}
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = compose ] && [ "$2" = version ]; then exit 0; fi
if [ "$5" != timeout ] || [ "$6" != --signal=TERM ] || [ "$8" != 80s ]; then exit 11; fi
if [ -n "${ODOOCTL_MCP_BROWSER_PASSWORD+x}" ] || [ -n "${GITHUB_TOKEN+x}" ]; then exit 12; fi
exec /bin/cat
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ODOOCTL_MCP_BROWSER_PASSWORD", "private")
	t.Setenv("GITHUB_TOKEN", "private")
	s := &Server{state: state, workers: make(chan struct{}, 2)}
	output, err := s.run(context.Background(), "safe input", "exec", "-T", "db", "psql")
	if err != nil || string(output) != "safe input" || len(s.workers) != 0 {
		t.Fatalf("runner: %q %v workers=%d", output, err, len(s.workers))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.run(ctx, "", "ps"); err == nil {
		t.Fatal("runner ignored cancellation")
	}
}

func TestWrapperResultBoundsAndErrors(t *testing.T) {
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	addTool(srv, "large", "test", func(context.Context, struct{}) (any, error) {
		return map[string]string{"text": strings.Repeat("x", (1<<20)+1)}, nil
	})
	client := testClient(t, srv)
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "large", Arguments: map[string]any{}})
	if err != nil || !result.IsError {
		t.Fatalf("oversized result accepted: %v %v", result, err)
	}
}

func TestCancelledRemoteRetainsConcurrencyReservation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix fake Docker fixture")
	}
	t.Setenv("HOME", t.TempDir())
	state := &config.State{ProjectName: "cancel", Branch: "main"}
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	marker := filepath.Join(bin, "started")
	script := "#!/bin/sh\nif [ \"$2\" = version ]; then exit 0; fi\nprintf started > \"$MCP_TEST_STARTED\"\nexec sleep 30\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MCP_TEST_STARTED", marker)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	s := &Server{state: state, workers: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := s.run(ctx, "", "exec", "-T", "odoo", "python3")
		finished <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fake remote process did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-finished:
		if err == nil || len(s.workers) != 1 {
			t.Fatalf("unconfirmed remote prematurely released: %v workers=%d", err, len(s.workers))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runner did not cancel")
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer waitCancel()
	if _, err := s.run(waitCtx, "", "ps"); err == nil {
		t.Fatal("reserved slot admitted new work")
	}
}

func TestStdioCLIProtocol(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "go", "run", "../..", "mcp", "serve", "--project", t.TempDir())
	cmd.Stderr = &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("stdio initialization failed: %v\n%s", err, stderr.String())
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 5 {
		session.Close()
		t.Fatalf("stdio tool discovery failed: %v %v\n%s", tools, err, stderr.String())
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "project_context", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		session.Close()
		t.Fatalf("stdio context failed: %v %v", result, err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("stdio EOF shutdown failed: %v", err)
	}
}
