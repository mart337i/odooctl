package mcpsrv

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This test is deliberately opt-in: it inspects a real development project and
// running Compose services. It never starts containers or changes permissions.
func TestLiveMCPProject(t *testing.T) {
	project := os.Getenv("ODOOCTL_MCP_TEST_PROJECT")
	if project == "" {
		t.Skip("set ODOOCTL_MCP_TEST_PROJECT to run live project inspection")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", "../..", "mcp", "serve", "--project", project)
	cmd.Stderr = os.Stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "odooctl-live-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("live stdio handshake: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	available := map[string]bool{}
	for _, tool := range tools.Tools {
		available[tool.Name] = true
	}
	t.Logf("stdio initialization passed; %d tools registered", len(tools.Tools))
	t.Run("runtime_capabilities_require_opt_in", func(t *testing.T) {
		for _, name := range []string{"database_query", "database_schema", "odoo_search_read", "browser_inspect", "browser_screenshot"} {
			if available[name] {
				t.Fatalf("%s was unexpectedly enabled without configuration", name)
			}
		}
	})
	t.Run("privileged_sql_role_rejected", func(t *testing.T) {
		probe := exec.CommandContext(ctx, "go", "run", "../..", "mcp", "serve", "--project", project, "--sql-role", "odoo", "--sql-tables", "ir_module_module")
		output, err := probe.CombinedOutput()
		if err == nil || !strings.Contains(string(output), "SQLRole must not be a privileged or application role") {
			t.Fatal("CLI did not reject the privileged application SQL role")
		}
	})
	call := func(t *testing.T, name string, args map[string]any, target any) {
		t.Helper()
		if !available[name] {
			t.Skipf("%s is not available in this environment", name)
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s protocol error: %v", name, err)
		}
		if result.IsError {
			// Do not print potentially sensitive tool contents or live log data.
			t.Fatalf("%s returned a tool error", name)
		}
		data, err := json.Marshal(result.StructuredContent)
		if err != nil || string(data) == "null" {
			t.Fatalf("%s has no structured result", name)
		}
		if target != nil {
			if err := json.Unmarshal(data, target); err != nil {
				t.Fatalf("%s result decoding: %v", name, err)
			}
		}
		t.Logf("%s passed (%d bytes of structured output, contents withheld)", name, len(data))
	}
	t.Run("project_context", func(t *testing.T) {
		var result struct {
			Project string `json:"project"`
		}
		call(t, "project_context", map[string]any{}, &result)
		want, _ := filepath.Abs(project)
		want, _ = filepath.EvalSymlinks(want)
		if result.Project != want {
			t.Fatalf("wrong project binding: %q", result.Project)
		}
	})
	var modules struct {
		Modules []sourceModule `json:"modules"`
	}
	t.Run("module_list", func(t *testing.T) {
		call(t, "module_list", map[string]any{}, &modules)
		if len(modules.Modules) == 0 {
			t.Fatal("no modules discovered in live addon project")
		}
		t.Logf("discovered %d local module definitions", len(modules.Modules))
	})
	if len(modules.Modules) > 0 {
		module := modules.Modules[0]
		t.Run("module_info", func(t *testing.T) {
			call(t, "module_info", map[string]any{"name": module.Manifest.Module}, nil)
		})
		paths := []string{module.Path + "/__manifest__.py"}
		for _, path := range strings.Split(os.Getenv("ODOOCTL_MCP_TEST_FILES"), ",") {
			if path != "" {
				paths = append(paths, path)
			}
		}
		for _, path := range paths {
			t.Run("code_read/"+path, func(t *testing.T) {
				var result sourceResult
				call(t, "code_read", map[string]any{"root": module.Root, "path": path, "limit": 10}, &result)
				if len(result.Lines) == 0 || result.Lines[0].Line != 1 {
					t.Fatal("source read returned no numbered lines")
				}
			})
		}
		t.Run("code_search", func(t *testing.T) {
			var result sourceResult
			call(t, "code_search", map[string]any{"root": module.Root, "path": module.Path, "query": "name", "limit": 10}, &result)
			if len(result.Lines) == 0 {
				t.Fatal("source search did not find manifest name")
			}
		})
	}
	t.Run("runtime_status", func(t *testing.T) {
		var result struct {
			Services []struct {
				Name  string `json:"Service"`
				State string `json:"State"`
			} `json:"services"`
		}
		call(t, "runtime_status", map[string]any{}, &result)
		for _, service := range result.Services {
			t.Logf("service %s: %s", service.Name, service.State)
		}
	})
	t.Run("logs_recent", func(t *testing.T) {
		call(t, "logs_recent", map[string]any{"service": "odoo", "limit": 5}, nil)
		call(t, "logs_recent", map[string]any{"service": "db", "limit": 5}, nil)
	})
	for _, path := range []string{"../README.md", ".git/config", "tree2.glb"} {
		t.Run("deny/"+path, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "code_read", Arguments: map[string]any{"root": "project", "path": path}})
			if err != nil || !result.IsError {
				t.Fatalf("unsafe path was not rejected: protocol error=%v", err)
			}
		})
	}
	if err := session.Close(); err != nil {
		t.Fatalf("live stdio shutdown: %v", err)
	}
}
