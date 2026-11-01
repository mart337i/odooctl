// mcp-usage is a disposable-fixture MCP client, not a production CLI command.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type imageMetadata struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int    `json:"size"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	MIME   string `json:"mime"`
}

type toolOutput struct {
	Structured any             `json:"structuredContent,omitempty"`
	Text       []string        `json:"toolText"`
	Images     []imageMetadata `json:"images"`
	Native     map[string]any  `json:"nativeResult"`
	IsError    bool            `json:"isError"`
}

type record struct {
	Timestamp  string      `json:"timestamp"`
	Action     string      `json:"action"`
	Tool       string      `json:"tool,omitempty"`
	Project    string      `json:"project"`
	Version    string      `json:"version"`
	Args       string      `json:"argsRawJSON,omitempty"`
	Binding    *toolOutput `json:"binding,omitempty"`
	Result     any         `json:"result,omitempty"`
	DurationMS int64       `json:"durationMs"`
	Status     string      `json:"status"`
	Error      string      `json:"error,omitempty"`
}

var filenamePattern = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
var assignmentPattern = regexp.MustCompile(`(?i)((?:["']?[a-z_]*(?:password|passwd|secret|token|api_key|apikey|cookie|session_id|private_key)[a-z_]*["']?)\s*[:=]\s*)(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;\}\]]+)`)
var authPattern = regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[a-z0-9._~+/=-]+`)
var urlPattern = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^\s/@]+:[^\s/@]+@`)
var tokenPattern = regexp.MustCompile(`(?:gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+)`)
var secretKeyPattern = regexp.MustCompile(`(?i)password|passwd|secret|token|api_key|apikey|cookie|session_id|private_key|authorization`)

func redact(text string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	text = authPattern.ReplaceAllString(text, "[REDACTED]")
	text = urlPattern.ReplaceAllString(text, "${1}[REDACTED]@")
	text = tokenPattern.ReplaceAllString(text, "[REDACTED]")
	return assignmentPattern.ReplaceAllString(text, "${1}[REDACTED]")
}

func clean(value any, secrets []string) any {
	switch v := value.(type) {
	case string:
		return redact(v, secrets)
	case []any:
		for i := range v {
			v[i] = clean(v[i], secrets)
		}
	case map[string]any:
		for key, item := range v {
			if secretKeyPattern.MatchString(key) {
				v[key] = "[REDACTED]"
			} else {
				v[key] = clean(item, secrets)
			}
		}
	}
	return value
}

func validateArguments(raw string) error {
	var args map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &args); err != nil || args == nil {
		return errors.New("tool arguments must be one valid JSON object")
	}
	return nil
}

// Validate the original response, before redaction can change fixture metadata.
func validateBinding(value any, project, version string) error {
	data, err := json.Marshal(value)
	if err != nil {
		return errors.New("invalid project_context response")
	}
	var binding struct {
		Project     string `json:"project"`
		Environment struct {
			Version string `json:"odoo_version"`
		} `json:"environment"`
	}
	if json.Unmarshal(data, &binding) != nil {
		return errors.New("invalid project_context response")
	}
	if binding.Project != project {
		return fmt.Errorf("project binding mismatch: got %q, expected %q", binding.Project, project)
	}
	if binding.Environment.Version != version {
		return fmt.Errorf("version binding mismatch: got %q, expected %q", binding.Environment.Version, version)
	}
	return nil
}

func saveImage(root *os.Root, tool string, content *mcp.ImageContent) (imageMetadata, error) {
	var result imageMetadata
	if content.MIMEType != "image/png" || len(content.Data) == 0 || len(content.Data) > 2<<20 {
		return result, errors.New("image must be a PNG payload of at most 2 MiB")
	}
	config, err := png.DecodeConfig(bytes.NewReader(content.Data))
	if err != nil {
		return result, errors.New("image payload is not a valid PNG")
	}
	prefix := filenamePattern.ReplaceAllString(tool, "_")
	if len(prefix) > 64 {
		prefix = prefix[:64]
	}
	// Rooted exclusive creation prevents tool-supplied paths and symlink escapes.
	name := prefix + "-" + rand.Text() + ".png"
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return result, err
	}
	if _, err = file.Write(content.Data); err != nil {
		file.Close()
		root.Remove(name)
		return result, err
	}
	if err = file.Close(); err != nil {
		root.Remove(name)
		return result, err
	}
	sum := sha256.Sum256(content.Data)
	return imageMetadata{Path: filepath.Join(root.Name(), name), SHA256: hex.EncodeToString(sum[:]), Size: len(content.Data), Width: config.Width, Height: config.Height, MIME: "image/png"}, nil
}

func convertResult(root *os.Root, tool string, result *mcp.CallToolResult) (*toolOutput, error) {
	output := &toolOutput{Structured: result.StructuredContent, Text: []string{}, Images: []imageMetadata{}, IsError: result.IsError}
	// Marshal native MCP fields only after removing binary image content.
	copyResult := *result
	copyResult.Content = nil
	data, err := json.Marshal(&copyResult)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &output.Native); err != nil {
		return nil, err
	}
	contents := []any{}
	for _, content := range result.Content {
		if image, ok := content.(*mcp.ImageContent); ok {
			metadata, err := saveImage(root, tool, image)
			if err != nil {
				return nil, err
			}
			output.Images = append(output.Images, metadata)
			contents = append(contents, map[string]any{"type": "image", "mimeType": image.MIMEType, "artifact": metadata})
			continue
		}
		if text, ok := content.(*mcp.TextContent); ok {
			output.Text = append(output.Text, text.Text)
		}
		contents = append(contents, content)
	}
	output.Native["content"] = contents
	output.Native["isError"] = result.IsError
	return output, nil
}

func defaultBinary() string {
	// The documented build installs both binaries in CLIroot/bin.
	if exe, err := os.Executable(); err == nil && filepath.Base(filepath.Dir(exe)) == "bin" {
		return filepath.Join(filepath.Dir(exe), "odooctl")
	}
	_, source, _, _ := runtime.Caller(0)
	path, _ := filepath.Abs(filepath.Join(filepath.Dir(source), "../../..", "bin", "odooctl"))
	return path
}

func run(argv []string, stdout io.Writer) int {
	started := time.Now()
	login, password := os.Getenv("ODOOCTL_MCP_BROWSER_LOGIN"), os.Getenv("ODOOCTL_MCP_BROWSER_PASSWORD")
	secrets := []string{login, password}
	entry := record{Timestamp: started.UTC().Format(time.RFC3339Nano), Status: "setup_error"}
	var root *os.Root
	finish := func(err error, code int) int {
		if err != nil {
			entry.Error = err.Error()
		}
		entry.DurationMS = time.Since(started).Milliseconds()
		data, marshalErr := json.Marshal(entry)
		if marshalErr != nil {
			data, _ = json.Marshal(map[string]any{"status": "error", "error": "cannot encode bridge result"})
			code = 1
		}
		var value any
		_ = json.Unmarshal(data, &value)
		data, _ = json.Marshal(clean(value, secrets))
		data = append(data, '\n')
		if root != nil {
			previous, logErr := root.Lstat("transcript.jsonl")
			openFlags := os.O_WRONLY | os.O_APPEND
			if errors.Is(logErr, os.ErrNotExist) {
				openFlags |= os.O_CREATE | os.O_EXCL
				logErr = nil
			} else if logErr == nil && !previous.Mode().IsRegular() {
				logErr = errors.New("transcript must be a regular file, not a symlink or special file")
			}
			var file *os.File
			if logErr == nil {
				file, logErr = root.OpenFile("transcript.jsonl", openFlags, 0600)
			}
			if logErr == nil {
				// Check the opened identity before changing permissions or appending data.
				opened, statErr := file.Stat()
				logErr = statErr
				if logErr == nil && (!opened.Mode().IsRegular() || (previous != nil && !os.SameFile(previous, opened))) {
					logErr = errors.New("transcript file changed during open")
				}
				if logErr == nil {
					logErr = file.Chmod(0600)
				}
				if logErr == nil {
					_, logErr = file.Write(data)
				}
				closeErr := file.Close()
				if logErr == nil {
					logErr = closeErr
				}
			}
			if logErr != nil {
				data, _ = json.Marshal(map[string]any{"status": "transcript_error", "error": redact(logErr.Error(), secrets), "result": value})
				data = append(data, '\n')
				code = 1
			}
		}
		if _, err := stdout.Write(data); err != nil {
			return 1
		}
		return code
	}
	flags := flag.NewFlagSet("mcp-usage", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	project := flags.String("project", "", "Absolute fixture project directory")
	version := flags.String("expected-version", "", "Expected Odoo version")
	artifacts := flags.String("artifacts", "", "Absolute artifact directory")
	binary := flags.String("binary", defaultBinary(), "Absolute pinned odooctl executable")
	role := flags.String("sql-role", "odooctl_inspect", "Fixture inspection role")
	user := flags.Int("odoo-user-id", 2, "Fixture Odoo user")
	companies := flags.String("company-ids", "1", "Fixture company IDs")
	development := flags.Bool("development-login", false, "Allow admin/admin defaults for disposable fixtures only")
	if err := flags.Parse(argv); err != nil {
		return finish(err, 1)
	}
	if *development {
		if login == "" {
			login = "admin"
		}
		if password == "" {
			password = "admin"
		}
	}
	secrets = []string{login, password}
	entry.Project, entry.Version = *project, *version
	args := flags.Args()
	if len(args) == 1 && args[0] == "list" {
		entry.Action = "list"
	} else if len(args) == 3 && args[0] == "call" {
		entry.Action, entry.Tool, entry.Args = "call", args[1], args[2]
	} else {
		return finish(errors.New("usage: mcp-usage --project ABS --expected-version VERSION --artifacts ABS [flags] list | call TOOL '{JSON}'"), 1)
	}
	if !filepath.IsAbs(*project) || !filepath.IsAbs(*artifacts) || !filepath.IsAbs(*binary) || *version == "" {
		return finish(errors.New("project, artifacts and binary must be absolute; expected-version is required"), 1)
	}
	if err := os.MkdirAll(*artifacts, 0700); err != nil {
		return finish(err, 1)
	}
	info, err := os.Lstat(*artifacts)
	if err != nil {
		return finish(err, 1)
	}
	if !info.IsDir() {
		return finish(errors.New("artifact directory must be a real directory, not a symlink"), 1)
	}
	root, err = os.OpenRoot(*artifacts)
	if err != nil {
		return finish(err, 1)
	}
	defer root.Close()
	if err := root.Chmod(".", 0700); err != nil {
		return finish(err, 1)
	}
	if entry.Action == "call" {
		if err := validateArguments(entry.Args); err != nil {
			return finish(err, 1)
		}
	}
	if login == "" || password == "" {
		return finish(errors.New("browser credentials must be set in the environment, or explicitly opt into --development-login for a disposable fixture"), 1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, *binary, "mcp", "serve", "--project", *project, "--sql-role", *role, "--sql-tables", "ir_module_module", "--odoo-user-id", fmt.Sprint(*user), "--models", "ir.module.module", "--company-ids", *companies, "--browser")
	cmd.Stderr = io.Discard
	cmd.Env = append(os.Environ(), "ODOOCTL_MCP_BROWSER_LOGIN="+login, "ODOOCTL_MCP_BROWSER_PASSWORD="+password)
	client := mcp.NewClient(&mcp.Implementation{Name: "mcp-usage", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
	if err != nil {
		return finish(fmt.Errorf("MCP stdio connection: %w", err), 1)
	}
	defer session.Close()
	binding, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "project_context", Arguments: json.RawMessage(`{}`)})
	if err != nil {
		return finish(fmt.Errorf("project_context: %w", err), 1)
	}
	entry.Binding, err = convertResult(root, "project_context", binding)
	if err != nil {
		return finish(err, 1)
	}
	if binding.IsError {
		return finish(errors.New("project_context returned a tool error"), 1)
	}
	if err := validateBinding(binding.StructuredContent, *project, *version); err != nil {
		return finish(err, 1)
	}
	entry.Status = "error"
	if entry.Action == "list" {
		tools := []*mcp.Tool{}
		for tool, err := range session.Tools(ctx, nil) {
			if err != nil {
				return finish(err, 1)
			}
			tools = append(tools, tool)
		}
		entry.Result = map[string]any{"tools": tools}
	} else {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: entry.Tool, Arguments: json.RawMessage(entry.Args)})
		if err != nil {
			return finish(err, 1)
		}
		entry.Result, err = convertResult(root, entry.Tool, result)
		if err != nil {
			return finish(err, 1)
		}
		if result.IsError {
			entry.Status = "tool_error"
			return finish(nil, 1)
		}
	}
	entry.Status = "ok"
	return finish(nil, 0)
}

func main() { os.Exit(run(os.Args[1:], os.Stdout)) }
