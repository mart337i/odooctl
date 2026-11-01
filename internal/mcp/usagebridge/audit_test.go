package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Audit saved agent evidence independently of the agents' final summaries.
func TestAgentUsageTranscripts(t *testing.T) {
	root := os.Getenv("ODOOCTL_MCP_MATRIX_ARTIFACTS")
	if root == "" {
		t.Skip("set ODOOCTL_MCP_MATRIX_ARTIFACTS to audit saved agent transcripts")
	}
	for _, version := range []string{"17", "18", "19", "20"} {
		t.Run(version, func(t *testing.T) {
			dir := filepath.Join(root, "v"+version, "round2")
			file, err := os.Open(filepath.Join(dir, "transcript.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			scanner := bufio.NewScanner(file)
			scanner.Buffer(make([]byte, 4096), 1<<20)
			calls := map[string]json.RawMessage{}
			invocations, elapsed := 0, int64(0)
			for scanner.Scan() {
				var entry struct {
					Action     string          `json:"action"`
					Tool       string          `json:"tool"`
					Version    string          `json:"version"`
					Status     string          `json:"status"`
					DurationMS int64           `json:"durationMs"`
					Result     json.RawMessage `json:"result"`
				}
				if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
					t.Fatal("invalid transcript JSON")
				}
				if entry.Version != version+".0" || entry.Status != "ok" {
					t.Fatal("transcript has a failed call or wrong version")
				}
				invocations++
				elapsed += entry.DurationMS
				if entry.Action == "call" {
					calls[entry.Tool] = entry.Result
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"odoo_search_read", "database_query", "module_info", "code_read", "browser_inspect", "browser_screenshot"} {
				if calls[name] == nil {
					t.Errorf("missing agent call: %s", name)
				}
			}
			base := func(name string) string {
				var result struct {
					Structured struct {
						Data []struct {
							ID    int    `json:"id"`
							Name  string `json:"name"`
							State string `json:"state"`
						} `json:"data"`
					} `json:"structuredContent"`
				}
				if err := json.Unmarshal(calls[name], &result); err != nil || len(result.Structured.Data) != 1 {
					t.Fatalf("%s did not return exactly one record", name)
				}
				row := result.Structured.Data[0]
				if row.ID <= 0 || row.Name != "base" || row.State != "installed" {
					t.Fatal("base is not installed")
				}
				return fmt.Sprintf("%d/%s/%s", row.ID, row.Name, row.State)
			}
			if base("odoo_search_read") != base("database_query") {
				t.Fatal("ORM and SQL disagree")
			}
			var source struct {
				Structured struct {
					Lines []struct {
						Path string `json:"path"`
						Line int    `json:"line"`
						Text string `json:"text"`
					} `json:"lines"`
				} `json:"structuredContent"`
			}
			if json.Unmarshal(calls["code_read"], &source) != nil {
				t.Fatal("invalid source result")
			}
			citation := false
			for _, line := range source.Structured.Lines {
				citation = citation || line.Path == "mcp_probe/__manifest__.py" && line.Line == 6 && line.Text == "    'depends': ['base'],"
			}
			if !citation {
				t.Fatal("dependency source citation missing")
			}
			var observation struct {
				Structured struct {
					Readiness  string            `json:"readiness"`
					Incomplete bool              `json:"incomplete"`
					Events     []json.RawMessage `json:"events"`
					Text       string            `json:"text"`
				} `json:"structuredContent"`
			}
			if json.Unmarshal(calls["browser_inspect"], &observation) != nil || observation.Structured.Readiness != "web_client_visible" || observation.Structured.Incomplete || len(observation.Structured.Events) != 0 || len(observation.Structured.Text) < 100 {
				t.Fatal("browser observation is incomplete or has diagnostic errors")
			}
			var screenshot struct {
				Images []imageMetadata `json:"images"`
			}
			if json.Unmarshal(calls["browser_screenshot"], &screenshot) != nil || len(screenshot.Images) != 1 {
				t.Fatal("missing native image metadata")
			}
			image := screenshot.Images[0]
			if filepath.Dir(image.Path) != dir {
				t.Fatal("image is outside the assigned artifact directory")
			}
			data, err := os.ReadFile(image.Path)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			config, err := png.DecodeConfig(bytes.NewReader(data))
			if err != nil || config.Width != 1280 || config.Height != 800 || len(data) != image.Size || hex.EncodeToString(sum[:]) != image.SHA256 || image.MIME != "image/png" {
				t.Fatal("PNG artifact does not match its reported metadata")
			}
			t.Logf("%d invocations, %d ms total client time, base=%s, PNG=%d bytes; evidence verified", invocations, elapsed, base("database_query"), len(data))
		})
	}
}
