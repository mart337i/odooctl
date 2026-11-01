package mcpsrv

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Opt-in only: the project, grants, user and browser runtime must already exist.
// ORM inspection requires trusted development code; browser login creates sessions.
func TestLiveMCPRuntime(t *testing.T) {
	project := os.Getenv("ODOOCTL_MCP_TEST_PROJECT")
	if project == "" {
		t.Skip("set ODOOCTL_MCP_TEST_PROJECT to inspect an existing live runtime")
	}
	role := os.Getenv("ODOOCTL_MCP_TEST_SQL_ROLE")
	uid := os.Getenv("ODOOCTL_MCP_TEST_ORM_UID")
	browserEnabled := os.Getenv("ODOOCTL_MCP_TEST_BROWSER") == "true"
	if role == "" && uid == "" && !browserEnabled {
		t.Skip("enable SQL_ROLE, ORM_UID or BROWSER through ODOOCTL_MCP_TEST_* variables")
	}
	args := []string{"run", "../..", "mcp", "serve", "--project", project}
	if role != "" {
		args = append(args, "--sql-role", role, "--sql-tables", "ir_module_module")
	}
	userID := 0
	if uid != "" {
		var err error
		userID, err = strconv.Atoi(uid)
		if err != nil || userID <= 1 {
			t.Fatal("ODOOCTL_MCP_TEST_ORM_UID must be a non-superuser integer greater than 1")
		}
		args = append(args, "--odoo-user-id", uid, "--models", "ir.module.module,res.partner", "--company-ids", "1")
	}
	if browserEnabled {
		args = append(args, "--browser")
	}
	secrets := []string{os.Getenv("ODOOCTL_MCP_BROWSER_LOGIN"), os.Getenv("ODOOCTL_MCP_BROWSER_PASSWORD")}
	safeError := func(err error) string {
		text := []rune(redactText(err.Error(), secrets...))
		if len(text) > 300 {
			text = text[:300]
		}
		return string(text)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...)
	// Do not attach stderr: startup diagnostics may contain live configuration.
	cmd.Stderr = &cappedBuffer{limit: 64 << 10}
	client := mcp.NewClient(&mcp.Implementation{Name: "odooctl-runtime-live-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("live stdio handshake: %s", safeError(err))
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Errorf("live stdio EOF shutdown: %s", safeError(err))
		}
		if cmd.ProcessState == nil || !cmd.ProcessState.Success() {
			t.Error("live CLI did not exit cleanly after EOF")
		}
	}()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %s", safeError(err))
	}
	available := map[string]bool{}
	for _, tool := range tools.Tools {
		available[tool.Name] = true
	}
	t.Run("registration", func(t *testing.T) {
		for _, name := range []string{"project_context", "module_list", "module_info", "code_read", "code_search", "runtime_status", "logs_recent"} {
			if !available[name] {
				t.Errorf("expected tool %s is missing", name)
			}
		}
		for name, enabled := range map[string]bool{
			"database_schema": role != "", "database_query": role != "",
			"odoo_model_info": uid != "", "odoo_search_read": uid != "", "odoo_read_group": uid != "",
			"browser_inspect": browserEnabled, "browser_screenshot": browserEnabled,
		} {
			if available[name] != enabled {
				t.Errorf("%s registration does not match explicit opt-in", name)
			}
		}
	})
	call := func(t *testing.T, name string, args map[string]any, rejected bool) *mcp.CallToolResult {
		t.Helper()
		toolCtx, toolCancel := context.WithTimeout(ctx, 90*time.Second)
		defer toolCancel()
		result, err := session.CallTool(toolCtx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s protocol error: %s", name, safeError(err))
		}
		if result == nil || result.IsError != rejected {
			t.Fatalf("%s: expected tool rejection=%t (contents withheld)", name, rejected)
		}
		return result
	}
	decode := func(t *testing.T, name string, args map[string]any, target any) {
		t.Helper()
		result := call(t, name, args, false)
		data, err := json.Marshal(result.StructuredContent)
		if err != nil || string(data) == "null" {
			t.Fatalf("%s has no structured result", name)
		}
		if json.Unmarshal(data, target) != nil {
			t.Fatalf("%s result cannot be decoded (contents withheld)", name)
		}
	}
	t.Run("project_context", func(t *testing.T) {
		var result struct {
			Project              string          `json:"project"`
			EnvironmentAvailable bool            `json:"environment_available"`
			Capabilities         map[string]bool `json:"capabilities"`
		}
		decode(t, "project_context", map[string]any{}, &result)
		want, err := filepath.Abs(project)
		if err != nil {
			t.Fatal("cannot resolve configured project")
		}
		want, err = filepath.EvalSymlinks(want)
		if err != nil || result.Project != want || !result.EnvironmentAvailable {
			t.Fatal("project context is not bound to the existing configured environment")
		}
		for name, enabled := range map[string]bool{"sql": role != "", "orm": uid != "", "browser": browserEnabled} {
			if result.Capabilities[name] != enabled {
				t.Errorf("project capability %s does not match opt-in", name)
			}
		}
	})
	t.Run("sql", func(t *testing.T) {
		if role == "" {
			t.Skip("ODOOCTL_MCP_TEST_SQL_ROLE is not configured")
		}
		var schema struct {
			Data []struct {
				Table  string `json:"table_name"`
				Column string `json:"column_name"`
				Type   string `json:"data_type"`
			} `json:"data"`
		}
		decode(t, "database_schema", map[string]any{"table": "ir_module_module"}, &schema)
		found := map[string]bool{}
		if len(schema.Data) == 0 || len(schema.Data) > 200 {
			t.Fatal("schema must contain bounded nonempty column metadata")
		}
		for _, column := range schema.Data {
			if column.Table != "ir_module_module" || column.Column == "" || column.Type == "" || databaseSensitive(column.Column) {
				t.Fatal("schema contains invalid or sensitive metadata")
			}
			found[column.Column] = true
		}
		for _, name := range []string{"id", "name", "state"} {
			if !found[name] {
				t.Errorf("schema is missing granted column %s", name)
			}
		}
		var rows struct {
			Data []struct {
				ID    int    `json:"id"`
				Name  string `json:"name"`
				State string `json:"state"`
			} `json:"data"`
			Provenance struct {
				Role     string `json:"role"`
				ReadOnly bool   `json:"read_only"`
			} `json:"provenance"`
		}
		decode(t, "database_query", map[string]any{"query": "SELECT id,name,state FROM ir_module_module ORDER BY id"}, &rows)
		if len(rows.Data) == 0 || len(rows.Data) > 200 || rows.Provenance.Role != role || !rows.Provenance.ReadOnly {
			t.Fatal("SQL result is empty, unbounded or lacks read-only role provenance")
		}
		base := false
		previous := 0
		for _, row := range rows.Data {
			if row.ID <= previous || row.Name == "" || row.State == "" {
				t.Fatal("SQL rows lack ordered IDs or required fields")
			}
			previous = row.ID
			base = base || row.Name == "base" && row.State == "installed"
		}
		if !base {
			t.Fatal("bounded SQL result does not contain the installed base module")
		}
		t.Logf("SQL inspection passed: %d rows (contents withheld)", len(rows.Data))
		for name, query := range map[string]string{
			"update":   "UPDATE ir_module_module SET state='installed'",
			"secret":   "SELECT password FROM ir_module_module",
			"function": "SELECT pg_sleep(1) FROM ir_module_module",
			"begin":    "BEGIN", "commit": "COMMIT", "rollback": "ROLLBACK",
			"multi_statement":         "SELECT id FROM ir_module_module; COMMIT",
			"ungranted_scalar":        "SELECT latest_version FROM ir_module_module",
			"ungranted_summary":       "SELECT summary FROM ir_module_module",
			"unsupported_column_type": "SELECT description FROM ir_module_module",
		} {
			t.Run("reject_"+name, func(t *testing.T) {
				call(t, "database_query", map[string]any{"query": query}, true)
			})
		}
	})
	t.Run("orm", func(t *testing.T) {
		if uid == "" {
			t.Skip("ODOOCTL_MCP_TEST_ORM_UID is not configured")
		}
		var info struct {
			Data map[string]map[string]any `json:"data"`
		}
		decode(t, "odoo_model_info", map[string]any{"model": "ir.module.module", "fields": []string{"id", "name", "state"}}, &info)
		if len(info.Data) != 3 {
			t.Fatal("model metadata must contain exactly three requested fields")
		}
		for _, field := range []string{"id", "name", "state"} {
			if len(info.Data[field]) == 0 || info.Data[field]["type"] == nil {
				t.Errorf("model metadata is missing field %s", field)
			}
		}
		var records struct {
			Data       []map[string]any `json:"data"`
			Provenance struct {
				UserID     int   `json:"user_id"`
				CompanyIDs []int `json:"company_ids"`
				ReadOnly   bool  `json:"read_only"`
			} `json:"provenance"`
		}
		decode(t, "odoo_search_read", map[string]any{"model": "ir.module.module", "domain": []any{[]any{"name", "=", "base"}}, "fields": []string{"id", "name", "state"}, "limit": 3}, &records)
		if len(records.Data) != 1 || records.Data[0]["name"] != "base" || records.Data[0]["state"] != "installed" || records.Data[0]["id"] == nil {
			t.Fatal("ORM base domain did not return the installed base module")
		}
		if records.Provenance.UserID != userID || !records.Provenance.ReadOnly || len(records.Provenance.CompanyIDs) != 1 || records.Provenance.CompanyIDs[0] != 1 {
			t.Fatal("ORM provenance does not match configured user and company")
		}
		var groups struct {
			Data []map[string]any `json:"data"`
		}
		decode(t, "odoo_read_group", map[string]any{"model": "ir.module.module", "fields": []string{"id:count"}, "group_by": []string{"state"}}, &groups)
		if len(groups.Data) == 0 || len(groups.Data) > 200 {
			t.Fatal("read_group must return nonempty bounded groups")
		}
		for _, group := range groups.Data {
			// Explicit id:count aggregates use the requested field's name.
			count, ok := group["id"].(float64)
			if !ok || count <= 0 || group["state"] == nil {
				t.Fatal("read_group lacks state or positive count")
			}
			for _, key := range []string{"__domain", "__context", "__range"} {
				if _, ok := group[key]; ok {
					t.Fatal("read_group exposed excluded metadata")
				}
			}
		}
		records.Data = nil
		decode(t, "odoo_search_read", map[string]any{"model": "res.partner", "domain": []any{[]any{"id", ">", 0}, []any{"active", "=", true}}, "fields": []string{"id", "name"}, "limit": 3, "order": "id asc"}, &records)
		if len(records.Data) == 0 || len(records.Data) > 3 {
			t.Fatal("partner domain must return 1 to 3 records")
		}
		for _, record := range records.Data {
			id, ok := record["id"].(float64)
			if !ok || id <= 0 || record["name"] == nil || len(record) != 2 {
				t.Fatal("partner result does not match bounded requested fields")
			}
		}
		t.Logf("ORM inspection passed: %d groups, %d partner records (contents withheld)", len(groups.Data), len(records.Data))
		for name, args := range map[string]map[string]any{
			"unsupported_model": {"model": "res.users", "fields": []string{"id"}},
			"secret_field":      {"model": "res.partner", "fields": []string{"password"}},
			"secret_domain":     {"model": "res.partner", "fields": []string{"id"}, "domain": []any{[]any{"password", "=", "probe"}}},
			"company":           {"model": "res.partner", "fields": []string{"id"}, "company_ids": []int{2}},
		} {
			t.Run("reject_"+name, func(t *testing.T) { call(t, "odoo_search_read", args, true) })
		}
	})
	t.Run("browser", func(t *testing.T) {
		if !browserEnabled {
			t.Skip("ODOOCTL_MCP_TEST_BROWSER=true is not configured")
		}
		checkText := func(t *testing.T, text string) {
			t.Helper()
			for _, secret := range secrets {
				if secret != "" && strings.Contains(text, secret) {
					t.Fatal("browser textual content leaked configured credentials")
				}
			}
		}
		checkReport := func(t *testing.T, report browserReport, web bool) {
			t.Helper()
			u, err := url.Parse(report.URL)
			if err != nil || u.Scheme != "http" || u.Host != "127.0.0.1:8069" || u.Path == "/web/login" || u.RawQuery != "" || u.User != nil {
				t.Fatal("browser did not return an authenticated local observation")
			}
			if report.Incomplete || report.Readiness != "web_client_visible" && report.Readiness != "body_visible_settled" || web && report.Readiness != "web_client_visible" {
				t.Fatal("browser observation did not reach positive readiness")
			}
			if strings.TrimSpace(report.Text) == "" || utf8.RuneCountInString(report.Text) > 12000 || len(report.DOM) == 0 || len(report.DOM) > 100 || len(report.Events) > 100 {
				t.Fatalf("browser observation is empty or exceeds bounds: text_runes=%d dom=%d events=%d", utf8.RuneCountInString(report.Text), len(report.DOM), len(report.Events))
			}
			for _, entry := range report.DOM {
				if utf8.RuneCountInString(entry) > 200 {
					t.Fatal("browser DOM entry exceeds bound")
				}
			}
			for _, event := range report.Events {
				if event.Kind == "http_error" {
					t.Fatal("browser observation reported an HTTP error (contents withheld)")
				}
			}
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatal("cannot encode browser report")
			}
			checkText(t, string(data))
		}
		for _, path := range []string{"/web", "/"} {
			t.Run("inspect"+path, func(t *testing.T) {
				result := call(t, "browser_inspect", map[string]any{"path": path}, false)
				for _, content := range result.Content {
					if text, ok := content.(*mcp.TextContent); ok {
						checkText(t, text.Text)
					}
				}
				data, err := json.Marshal(result.StructuredContent)
				var report browserReport
				if err != nil || string(data) == "null" || json.Unmarshal(data, &report) != nil {
					t.Fatal("browser inspect lacks structured report")
				}
				checkReport(t, report, path == "/web")
			})
		}
		t.Run("screenshot", func(t *testing.T) {
			result := call(t, "browser_screenshot", map[string]any{"path": "/web"}, false)
			images, texts := 0, 0
			for _, content := range result.Content {
				switch content := content.(type) {
				case *mcp.ImageContent:
					images++
					if content.MIMEType != "image/png" || len(content.Data) == 0 || len(content.Data) > 2*1024*1024 {
						t.Fatal("screenshot must be a nonempty native PNG of at most 2 MiB")
					}
					config, err := png.DecodeConfig(bytes.NewReader(content.Data))
					if err != nil || config.Width != 1280 || config.Height != 800 {
						t.Fatal("screenshot does not have the fixed 1280x800 PNG viewport")
					}
				case *mcp.TextContent:
					texts++
					checkText(t, content.Text)
					var report browserReport
					if json.Unmarshal([]byte(content.Text), &report) != nil || report.Image != "" {
						t.Fatal("screenshot text must contain a report without embedded image data")
					}
					checkReport(t, report, true)
				default:
					t.Fatal("unexpected screenshot content type")
				}
			}
			if images != 1 || texts != 1 {
				t.Fatal("screenshot must contain one native image and one text report")
			}
		})
		t.Run("reject_off_origin", func(t *testing.T) {
			for _, name := range []string{"browser_inspect", "browser_screenshot"} {
				call(t, name, map[string]any{"path": "https://example.com/web"}, true)
				call(t, name, map[string]any{"path": "//example.com/web"}, true)
			}
		})
	})
}
