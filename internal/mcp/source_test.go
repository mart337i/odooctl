package mcpsrv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/mart337i/odooctl/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSourceRegistration(t *testing.T) {
	s := &Server{}
	s.registerSource(mcp.NewServer(&mcp.Implementation{Name: "source-test", Version: "test"}, nil))
}

func sourceFixture(t *testing.T, root, path, content string) {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSourcePathPolicy(t *testing.T) {
	for _, path := range []string{"../file.py", "a/../../file.py", "/file.py", "a\\file.py", "C:/file.py", "a//file.py", "a/./file.py", "a/.git/file.py", ".env", "nested/.env.py", "nested/credentials.json", "nested/secrets/data.py", "nested/config.json", "nested/odoo_config.py", "state.json", "a/key.pem", "a/key.key", "a/id_rsa", "a/private_key.txt", "a/auth.json", "file.go", "image.png", "a\x00.py"} {
		t.Run(path, func(t *testing.T) {
			if sourceSafePath(path, true) {
				t.Fatalf("allowed %q", path)
			}
		})
	}
	for _, path := range []string{"models/product.py", "__manifest__.py", "views/form.xml", "README.md", "static/src/main.ts", "a/deps.toml", "a/data.csv"} {
		if !sourceSafePath(path, true) {
			t.Errorf("denied %q", path)
		}
	}
	if !sourceSafePath(".", false) || sourceSafePath(".", true) {
		t.Fatal("incorrect root directory policy")
	}
}

func TestSourceReadConfinement(t *testing.T) {
	root := sourceRoot{ID: "project", Path: t.TempDir()}
	outside := t.TempDir()
	sourceFixture(t, outside, "outside.py", "outside\n")
	sourceFixture(t, root.Path, "safe.py", "safe\n")
	sourceFixture(t, root.Path, "nested/credentials.json", "secret\n")
	sourceFixture(t, root.Path, ".hidden/hidden.py", "secret\n")
	sourceFixture(t, root.Path, "safe-dir/file.py", "safe\n")
	for name, target := range map[string]string{
		"escape.py":    filepath.Join(outside, "outside.py"),
		"escape-dir":   outside,
		"alias.py":     filepath.Join(root.Path, "nested", "credentials.json"),
		"hidden.py":    filepath.Join(root.Path, ".hidden", "hidden.py"),
		"internal.py":  filepath.Join(root.Path, "safe.py"),
		"internal-dir": filepath.Join(root.Path, "safe-dir"),
		"secret-dir":   filepath.Join(root.Path, ".hidden"),
	} {
		if err := os.Symlink(target, filepath.Join(root.Path, name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"../outside.py", "escape.py", "escape-dir/outside.py", "alias.py", "hidden.py", "internal.py", "internal-dir/file.py", "secret-dir/hidden.py", "nested/credentials.json", ".hidden/hidden.py"} {
		if _, err := sourceRead(context.Background(), root, sourceReadInput{Path: path}); err == nil {
			t.Errorf("allowed %s", path)
		}
	}
	for _, path := range []string{"internal-dir", "secret-dir", "escape-dir"} {
		if _, err := sourceSearch(context.Background(), root, sourceSearchInput{Path: path, Query: "safe"}); err == nil {
			t.Errorf("searched symlink directory %s", path)
		}
	}
	result, err := sourceRead(context.Background(), root, sourceReadInput{Path: "safe-dir/file.py"})
	if err != nil || len(result.Lines) != 1 || result.Lines[0].Text != "safe" {
		t.Fatalf("regular nested file: %+v, %v", result, err)
	}
}

func TestSourceOpenPinsFile(t *testing.T) {
	path := t.TempDir()
	sourceFixture(t, path, "safe/file.py", "safe\n")
	sourceFixture(t, path, ".hidden/file.py", "secret\n")
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	f, err := sourceOpen(root, "safe/file.py")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.Rename(filepath.Join(path, "safe"), filepath.Join(path, "original")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(path, ".hidden"), filepath.Join(path, "safe")); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(f)
	if err != nil || string(data) != "safe\n" {
		t.Fatalf("opened descriptor changed: %q %v", data, err)
	}
	if _, err := sourceTextAt(root, "safe/file.py"); err == nil {
		t.Fatal("replacement directory symlink accepted")
	}
}

func TestSourceReadPages(t *testing.T) {
	root := sourceRoot{ID: "addons-1", Path: t.TempDir()}
	sourceFixture(t, root.Path, "file.py", "one\r\ntwo\nthree\nfour\n")
	result, err := sourceRead(context.Background(), root, sourceReadInput{Path: "file.py", StartLine: 2, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 2 || result.Lines[0].Line != 2 || result.Lines[0].Text != "two" || result.Lines[0].Root != "addons-1" || result.Lines[0].Path != "file.py" || !result.Truncated || result.NextLine != 4 {
		t.Fatalf("bad page: %+v", result)
	}
	last, err := sourceRead(context.Background(), root, sourceReadInput{Path: "file.py", StartLine: 4, Limit: 200})
	if err != nil || len(last.Lines) != 1 || last.Truncated || last.NextLine != 0 {
		t.Fatalf("bad final page: %+v %v", last, err)
	}
	beyond, err := sourceRead(context.Background(), root, sourceReadInput{Path: "file.py", StartLine: 1000})
	if err != nil || len(beyond.Lines) != 0 {
		t.Fatalf("past EOF: %+v %v", beyond, err)
	}
	first, err := sourceRead(context.Background(), root, sourceReadInput{Path: "file.py", Limit: 1})
	if err != nil || first.Lines[0].Text != "one" {
		t.Fatalf("CRLF: %+v %v", first, err)
	}
	sourceFixture(t, root.Path, "empty.txt", "")
	empty, err := sourceRead(context.Background(), root, sourceReadInput{Path: "empty.txt"})
	if err != nil || len(empty.Lines) != 0 {
		t.Fatalf("empty: %+v %v", empty, err)
	}
	for _, in := range []sourceReadInput{{Path: "file.py", Limit: 201}, {Path: "file.py", Limit: -1}, {Path: "file.py", StartLine: -1}} {
		if _, err := sourceRead(context.Background(), root, in); err == nil {
			t.Errorf("accepted invalid page: %+v", in)
		}
	}
}

func TestSourceReadBinaryAndSizeBounds(t *testing.T) {
	root := sourceRoot{ID: "project", Path: t.TempDir()}
	for _, content := range []string{"abc\x00def", "abc\x01def", "abc\xffdef", "abc\x7fdef", strings.Repeat("x", sourceFileBytes+1)} {
		sourceFixture(t, root.Path, "file.py", content)
		if _, err := sourceRead(context.Background(), root, sourceReadInput{Path: "file.py"}); err == nil {
			t.Errorf("allowed binary or oversized input (%d bytes)", len(content))
		}
	}
	sourceFixture(t, root.Path, "file.py", strings.Repeat("x", sourceFileBytes))
	result, err := sourceRead(context.Background(), root, sourceReadInput{Path: "file.py"})
	if err != nil || !result.Truncated || !result.Lines[0].Truncated || len(result.Lines[0].Text) != sourceLineBytes {
		t.Fatalf("bounded long line: %+v %v", result, err)
	}
	sourceFixture(t, root.Path, "file.py", strings.Repeat("a", 1999)+"\u00e9")
	result, err = sourceRead(context.Background(), root, sourceReadInput{Path: "file.py"})
	if err != nil || !result.Truncated || !utf8.ValidString(result.Lines[0].Text) {
		t.Fatalf("invalid UTF-8 clipping: %+v %v", result, err)
	}
}

func TestSourceSearchLiteralAndDeniedFiles(t *testing.T) {
	root := sourceRoot{ID: "project", Path: t.TempDir()}
	sourceFixture(t, root.Path, "models/a.py", "a.*b\nnot regex: axxb\na.*b again\n")
	for _, path := range []string{".git/a.py", ".env", "nested/secret.py", "nested/config.json", "nested/data.key", "binary.py", "image.png"} {
		content := "a.*b\n"
		if path == "binary.py" {
			content += "\x00"
		}
		sourceFixture(t, root.Path, path, content)
	}
	outside := t.TempDir()
	sourceFixture(t, outside, "a.py", "a.*b\n")
	if err := os.Symlink(outside, filepath.Join(root.Path, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "a.py"), filepath.Join(root.Path, "escape.py")); err != nil {
		t.Fatal(err)
	}
	result, err := sourceSearch(context.Background(), root, sourceSearchInput{Query: "a.*b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 2 || result.Truncated {
		t.Fatalf("search: %+v", result)
	}
	for _, line := range result.Lines {
		if line.Path != "models/a.py" || line.Root != "project" {
			t.Fatalf("unexpected provenance: %+v", line)
		}
	}
	single, err := sourceSearch(context.Background(), root, sourceSearchInput{Path: "models/a.py", Query: "a.*b", Limit: 1})
	if err != nil || len(single.Lines) != 1 || !single.Truncated {
		t.Fatalf("single-file cap: %+v %v", single, err)
	}
	for _, in := range []sourceSearchInput{{Query: ""}, {Query: "x", Limit: 101}, {Query: "x", Path: ".."}, {Query: strings.Repeat("x", 1025)}} {
		if _, err := sourceSearch(context.Background(), root, in); err == nil {
			t.Errorf("accepted invalid search %+v", in)
		}
	}
}

func TestSourceSearchCaps(t *testing.T) {
	t.Run("matches", func(t *testing.T) {
		root := sourceRoot{ID: "project", Path: t.TempDir()}
		sourceFixture(t, root.Path, "a.py", strings.Repeat("match\n", 101))
		result, err := sourceSearch(context.Background(), root, sourceSearchInput{Query: "match"})
		if err != nil || len(result.Lines) != 100 || !result.Truncated {
			t.Fatalf("match cap: %+v %v", result, err)
		}
	})
	t.Run("visited", func(t *testing.T) {
		root := sourceRoot{ID: "project", Path: t.TempDir()}
		for i := 0; i < sourceVisitLimit+1; i++ {
			sourceFixture(t, root.Path, fmt.Sprintf("f%05d.bin", i), "")
		}
		result, err := sourceSearch(context.Background(), root, sourceSearchInput{Query: "match"})
		if err != nil || result.Visited != sourceVisitLimit || !result.Truncated {
			t.Fatalf("visit cap: %+v %v", result, err)
		}
	})
	t.Run("bytes", func(t *testing.T) {
		root := sourceRoot{ID: "project", Path: t.TempDir()}
		for i := 0; i < 17; i++ {
			sourceFixture(t, root.Path, fmt.Sprintf("f%02d.py", i), strings.Repeat("x", sourceFileBytes))
		}
		result, err := sourceSearch(context.Background(), root, sourceSearchInput{Query: "absent"})
		if err != nil || !result.Truncated || len(result.Lines) != 0 {
			t.Fatalf("byte cap: %+v %v", result, err)
		}
	})
	t.Run("canceled", func(t *testing.T) {
		root := sourceRoot{ID: "project", Path: t.TempDir()}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result, err := sourceSearch(ctx, root, sourceSearchInput{Query: "match"})
		if err != nil || !result.Truncated || result.Visited != 0 {
			t.Fatalf("cancellation: %+v %v", result, err)
		}
		if _, err := sourceRead(ctx, root, sourceReadInput{Path: "a.py"}); err == nil {
			t.Fatal("read ignored cancellation")
		}
	})
}

func TestSourceRootsAndModules(t *testing.T) {
	project, external := t.TempDir(), t.TempDir()
	sourceFixture(t, project, "sale/__manifest__.py", "{'name': 'Sales', 'version': '18.0.1.0.0', 'depends': ['base', 'custom']}")
	sourceFixture(t, external, "sale/__manifest__.py", "{'name': 'Other Sales', 'depends': ['base']}")
	sourceFixture(t, external, "custom/__manifest__.py", "{'name': 'Custom', 'depends': []}")
	s := &Server{state: &config.State{ProjectRoot: project, AddonsPaths: []string{external, filepath.Join(project, "missing")}}}
	roots := s.sourceRoots()
	if len(roots) != 3 || roots[0].ID != "project" || roots[1].ID != "addons-1" || roots[1].Path != external {
		t.Fatalf("roots: %+v", roots)
	}
	if _, err := s.sourceRoot("odoo-core"); err == nil {
		t.Fatal("container source enabled by default")
	}
	s.options.ContainerSource = true
	if root, err := s.sourceRoot("odoo-enterprise"); err != nil || !root.Container {
		t.Fatalf("container root: %+v %v", root, err)
	}
	if _, err := s.sourceRoot("/tmp"); err == nil {
		t.Fatal("arbitrary root accepted")
	}
	read, err := sourceRead(context.Background(), roots[1], sourceReadInput{Path: "custom/__manifest__.py"})
	if err != nil || len(read.Lines) != 1 {
		t.Fatalf("external configured root: %+v %v", read, err)
	}
	value, err := s.sourceModules(context.Background(), "sale")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Modules  []sourceModule `json:"modules"`
		Warnings []string       `json:"warnings"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Modules) != 2 || len(result.Modules[0].Manifest.Depends) != 2 || result.Modules[0].Manifest.Path != "sale/__manifest__.py" {
		t.Fatalf("modules: %s", data)
	}
	warnings := strings.Join(result.Warnings, "\n")
	for _, expected := range []string{"Module collision: sale", "Unavailable module root: addons-2", "sale -> base"} {
		if !strings.Contains(warnings, expected) {
			t.Errorf("missing %q in %s", expected, warnings)
		}
	}
	if strings.Contains(warnings, "sale -> custom") {
		t.Fatal("configured dependency marked missing")
	}
}

func TestSourceModuleSymlinkManifestDenied(t *testing.T) {
	root := t.TempDir()
	sourceFixture(t, root, "addon/placeholder.py", "")
	sourceFixture(t, root, ".hidden/data.py", "{'name': 'Private manifest', 'depends': ['private_dep']}")
	if err := os.Symlink(filepath.Join(root, ".hidden", "data.py"), filepath.Join(root, "addon", "__manifest__.py")); err != nil {
		t.Fatal(err)
	}
	s := &Server{roots: []string{root}}
	value, err := s.sourceModules(context.Background(), "addon")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Private manifest") || strings.Contains(string(data), "private_dep") {
		t.Fatalf("read symlink manifest: %s", data)
	}
	if !strings.Contains(string(data), "Denied or unavailable manifest") {
		t.Fatalf("missing denial warning: %s", data)
	}
}

// Exercise the fixed Python implementation locally, never through Docker. Only
// its hard-coded container root is mapped to a temporary fixture directory.
func TestSourceContainerPolicy(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	root := t.TempDir()
	sourceFixture(t, root, "a.py", "match\nmatch\nthird\n")
	sourceFixture(t, root, "nested/secret.py", "match\n")
	sourceFixture(t, root, ".env", "match\n")
	sourceFixture(t, root, "binary.py", "match\x00\n")
	sourceFixture(t, root, "config.json", "match\n")
	outside := t.TempDir()
	sourceFixture(t, outside, "a.py", "match\n")
	if err := os.Symlink(filepath.Join(outside, "a.py"), filepath.Join(root, "escape.py")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "nested", "secret.py"), filepath.Join(root, "alias.py")); err != nil {
		t.Fatal(err)
	}
	script := strings.ReplaceAll(sourceContainerScript, "'/opt/odoo-src'", fmt.Sprintf("%q", root))
	if err := os.Symlink(filepath.Join(root, "nested"), filepath.Join(root, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "a.py"), filepath.Join(root, "internal.py")); err != nil {
		t.Fatal(err)
	}
	run := func(op string, in any) (sourceResult, error) {
		t.Helper()
		request, err := json.Marshal(map[string]any{"root": sourceRoot{ID: "odoo-core"}, "operation": op, "input": in})
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(context.Background(), python, "-c", script)
		cmd.Stdin = strings.NewReader(string(request))
		output, err := cmd.CombinedOutput()
		if err != nil {
			return sourceResult{}, fmt.Errorf("%w: %s", err, output)
		}
		var result sourceResult
		err = json.Unmarshal(output, &result)
		return result, err
	}
	for _, path := range []string{"../a.py", "escape.py", "alias.py", "internal.py", "linked-dir/secret.py", ".env", "nested/secret.py", "binary.py", "config.json"} {
		if _, err := run("read", sourceReadInput{Path: path}); err == nil {
			t.Errorf("container allowed %s", path)
		}
	}
	page, err := run("read", sourceReadInput{Path: "a.py", StartLine: 2, Limit: 1})
	if err != nil || len(page.Lines) != 1 || page.Lines[0].Line != 2 || page.NextLine != 3 || !page.Truncated {
		t.Fatalf("container page: %+v %v", page, err)
	}
	search, err := run("search", sourceSearchInput{Query: "match", Limit: 1})
	if err != nil || len(search.Lines) != 1 || search.Lines[0].Path != "a.py" || !search.Truncated {
		t.Fatalf("container search: %+v %v", search, err)
	}
	all, err := run("search", sourceSearchInput{Query: "match"})
	if err != nil || len(all.Lines) != 2 || all.Truncated {
		t.Fatalf("container filtering: %+v %v", all, err)
	}
	sourceFixture(t, root, "quoted'\".py", "quoted source\n")
	quoted, err := run("read", sourceReadInput{Path: "quoted'\".py"})
	if err != nil || len(quoted.Lines) != 1 || quoted.Lines[0].Text != "quoted source" {
		t.Fatalf("JSON path transport: %+v %v", quoted, err)
	}
	sourceFixture(t, root, "many.py", strings.Repeat("needle\n", 101))
	capped, err := run("search", sourceSearchInput{Query: "needle"})
	if err != nil || len(capped.Lines) != 100 || !capped.Truncated {
		t.Fatalf("container default match cap: %+v %v", capped, err)
	}
	sourceFixture(t, root, "oversized.py", strings.Repeat("x", sourceFileBytes+1))
	if _, err := run("read", sourceReadInput{Path: "oversized.py"}); err == nil {
		t.Fatal("container allowed oversized source")
	}
}

func TestSourceContainerOpenRace(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	for _, scenario := range []string{"file-internal", "file-external", "directory-internal", "directory-external", "directory-after-open"} {
		for _, operation := range []string{"read", "search", "search-directory"} {
			t.Run(scenario+"/"+operation, func(t *testing.T) {
				root, outside := t.TempDir(), t.TempDir()
				sourceFixture(t, root, "victim/file.py", "safe\n")
				sourceFixture(t, root, ".hidden/file.py", "DO_NOT_EXPOSE\n")
				sourceFixture(t, outside, "file.py", "DO_NOT_EXPOSE\n")
				target := filepath.Join(root, ".hidden")
				if strings.Contains(scenario, "external") {
					target = outside
				}
				component := "victim"
				replace := filepath.Join(root, "victim")
				if strings.HasPrefix(scenario, "file") {
					component = "file.py"
					replace = filepath.Join(replace, "file.py")
					target = filepath.Join(target, "file.py")
				}
				// Swap at the descriptor-open boundary; after-open directory
				// swaps must preserve the original pinned descriptor.
				hook := fmt.Sprintf(`import os, sys
real_open = os.open
swapped = False
def racing_open(path, flags, mode=0o777, *, dir_fd=None):
    global swapped
    if path != %q or dir_fd is None or swapped:
        return real_open(path, flags, mode, dir_fd=dir_fd)
    swapped = True
    after = %q == 'directory-after-open'
    if after: fd = real_open(path, flags, mode, dir_fd=dir_fd)
    os.rename(%q, %q)
    os.symlink(%q, %q)
    sys.stderr.write('SWAPPED\n')
    if after: return fd
    return real_open(path, flags, mode, dir_fd=dir_fd)
os.open = racing_open
`, component, scenario, replace, replace+"-original", target, replace)
				script := hook + strings.ReplaceAll(sourceContainerScript, "'/opt/odoo-src'", fmt.Sprintf("%q", root))
				path, op := "victim/file.py", operation
				if operation == "search-directory" {
					path, op = ".", "search"
				}
				request, err := json.Marshal(map[string]any{"root": sourceRoot{ID: "odoo-core"}, "operation": op, "input": map[string]any{"path": path, "query": "DO_NOT_EXPOSE"}})
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, python, "-c", script)
				cmd.Stdin = strings.NewReader(string(request))
				output, runErr := cmd.CombinedOutput()
				if !strings.Contains(string(output), "SWAPPED") {
					t.Fatalf("race hook not exercised: %s", output)
				}
				if strings.Contains(string(output), "DO_NOT_EXPOSE") {
					t.Fatalf("leaked race target: %s", output)
				}
				if operation == "search-directory" {
					if runErr != nil {
						t.Fatalf("directory search failed instead of skipping swapped path: %v %s", runErr, output)
					}
					var result sourceResult
					if err := json.Unmarshal([]byte(strings.ReplaceAll(string(output), "SWAPPED\n", "")), &result); err != nil {
						t.Fatal(err)
					}
					if len(result.Lines) != 0 {
						t.Fatalf("directory search read race target: %+v", result)
					}
				} else if scenario == "directory-after-open" {
					if runErr != nil {
						t.Fatalf("pinned directory lost: %v %s", runErr, output)
					}
					if operation == "read" && !strings.Contains(string(output), `"text": "safe"`) {
						t.Fatalf("lost original file: %s", output)
					}
				} else if runErr == nil {
					t.Fatalf("raced symlink accepted: %s", output)
				}
			})
		}
	}
}
