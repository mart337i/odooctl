package mcpsrv

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Uses an existing disposable development fixture; only controlled login is allowed.
// Run once per version with ODOOCTL_MCP_TEST_PROJECT and browser credentials set.
func TestLiveBrowserMatrix(t *testing.T) {
	project := os.Getenv("ODOOCTL_MCP_TEST_PROJECT")
	if project == "" || os.Getenv("ODOOCTL_MCP_TEST_BROWSER") != "true" {
		t.Skip("set ODOOCTL_MCP_TEST_PROJECT and ODOOCTL_MCP_TEST_BROWSER=true")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", "../..", "mcp", "serve", "--project", project, "--browser")
	cmd.Stderr = &cappedBuffer{limit: 64 << 10}
	client := mcp.NewClient(&mcp.Implementation{Name: "browser-matrix", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal("live browser handshake failed (details withheld)")
	}
	defer func() {
		if session.Close() != nil || cmd.ProcessState == nil || !cmd.ProcessState.Success() {
			t.Error("live browser CLI did not exit cleanly")
		}
	}()
	for _, name := range []string{"browser_inspect", "browser_screenshot"} {
		t.Run(name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{"path": "/web"}})
			if err != nil || result == nil || result.IsError {
				t.Fatal("browser observation failed (contents withheld)")
			}
			var report browserReport
			imageBytes := 0
			for _, content := range result.Content {
				switch content := content.(type) {
				case *mcp.TextContent:
					if json.Unmarshal([]byte(content.Text), &report) != nil {
						t.Fatal("invalid browser report")
					}
				case *mcp.ImageContent:
					imageBytes = len(content.Data)
					config, err := png.DecodeConfig(bytes.NewReader(content.Data))
					if err != nil || content.MIMEType != "image/png" || config.Width != 1280 || config.Height != 800 {
						t.Error("invalid fixed-viewport PNG")
					}
				}
			}
			if report.Incomplete || report.Readiness != "web_client_visible" || len(report.Text) < 100 || len(report.DOM) < 5 {
				t.Errorf("thin/incomplete observation: readiness=%s incomplete=%t text=%d dom=%d", report.Readiness, report.Incomplete, len(report.Text), len(report.DOM))
			}
			for _, event := range report.Events {
				if event.Kind == "request_failed" || event.Kind == "pageerror" {
					t.Errorf("unexpected diagnostic kind=%s interception=%t aborted=%t (contents withheld)", event.Kind, event.Text == "request interception failed", event.Text == "net::ERR_ABORTED")
				}
			}
			if name == "browser_screenshot" && imageBytes < 12000 {
				t.Errorf("thin screenshot: %d bytes", imageBytes)
			}
			t.Logf("readiness=%s incomplete=%t text=%d dom=%d events=%d png=%d (contents withheld)", report.Readiness, report.Incomplete, len(report.Text), len(report.DOM), len(report.Events), imageBytes)
		})
	}
}
