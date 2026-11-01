package mcpsrv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mart337i/odooctl/internal/module"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	sourceFileBytes   = 1 << 20
	sourceSearchBytes = 16 << 20
	sourceVisitLimit  = 10000
	sourceLineBytes   = 2000
)

type sourceRoot struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Container bool   `json:"container"`
}

type sourceReadInput struct {
	Root      string `json:"root" jsonschema:"Approved root ID: project, addons-1, etc.; optional odoo-core or odoo-enterprise"`
	Path      string `json:"path" jsonschema:"Relative source file path within the root"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"First line, one-based; default 1"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Maximum lines, 1 to 200; default 100"`
}

type sourceSearchInput struct {
	Root  string `json:"root" jsonschema:"Approved root ID: project, addons-1, etc.; optional odoo-core or odoo-enterprise"`
	Path  string `json:"path,omitempty" jsonschema:"Relative file or directory; default root"`
	Query string `json:"query" jsonschema:"Literal case-sensitive substring, not a regular expression"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum matching lines, 1 to 100; default 100"`
}

type sourceLine struct {
	Root      string `json:"root"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated,omitempty"`
}

type sourceResult struct {
	Root      string       `json:"root"`
	Path      string       `json:"path"`
	Lines     []sourceLine `json:"lines"`
	Truncated bool         `json:"truncated"`
	NextLine  int          `json:"next_line,omitempty"`
	Visited   int          `json:"visited,omitempty"`
	Warnings  []string     `json:"warnings,omitempty"`
}

func (s *Server) registerSource(srv *mcp.Server) {
	addTool(srv, "module_list", "List local Odoo modules, direct dependencies, root provenance and lookup warnings.", func(ctx context.Context, _ struct{}) (any, error) {
		return s.sourceModules(ctx, "")
	})
	addTool(srv, "module_info", "Find all local definitions of an Odoo module and its direct dependencies; does not query the database.", func(ctx context.Context, in struct {
		Name string `json:"name" jsonschema:"Technical module name"`
	}) (any, error) {
		if in.Name == "" || strings.IndexFunc(in.Name, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_')
		}) >= 0 || !sourceSafePath(in.Name, false) {
			return nil, errors.New("invalid module name")
		}
		return s.sourceModules(ctx, in.Name)
	})
	addTool(srv, "code_read", "Read a bounded page of text source from an approved root; symlinks, secret and binary files are denied.", func(ctx context.Context, in sourceReadInput) (any, error) {
		if err := sourceReadDefaults(&in); err != nil {
			return nil, err
		}
		root, err := s.sourceRoot(in.Root)
		if err != nil {
			return nil, err
		}
		if root.Container {
			return s.sourceContainer(ctx, root, "read", in)
		}
		return sourceRead(ctx, root, in)
	})
	addTool(srv, "code_search", "Search literal substrings in approved text source without following symlinks, bounded to 100 matching lines and 10000 visited entries; reports truncation.", func(ctx context.Context, in sourceSearchInput) (any, error) {
		if err := sourceSearchDefaults(&in); err != nil {
			return nil, err
		}
		root, err := s.sourceRoot(in.Root)
		if err != nil {
			return nil, err
		}
		if root.Container {
			return s.sourceContainer(ctx, root, "search", in)
		}
		return sourceSearch(ctx, root, in)
	})
}

// Root IDs follow the ordered local roots supplied by core; configured external
// addon directories are independently approved, not confined to the project.
func (s *Server) sourceRoots() []sourceRoot {
	paths := s.roots
	if len(paths) == 0 && s.state != nil {
		paths = s.state.AllAddonsPaths()
	}
	if len(paths) == 0 && s.options.Project != "" {
		paths = []string{s.options.Project}
	}
	result := make([]sourceRoot, 0, len(paths)+2)
	for i, path := range paths {
		id := "project"
		if i > 0 {
			id = fmt.Sprintf("addons-%d", i)
		}
		if !filepath.IsAbs(path) && i > 0 && len(paths) > 0 {
			path = filepath.Join(paths[0], path)
		}
		result = append(result, sourceRoot{ID: id, Path: path})
	}
	if s.options.ContainerSource {
		result = append(result, sourceRoot{ID: "odoo-core", Path: "/opt/odoo-src", Container: true}, sourceRoot{ID: "odoo-enterprise", Path: "/opt/odoo-enterprise", Container: true})
	}
	return result
}

func (s *Server) sourceRoot(id string) (sourceRoot, error) {
	for _, root := range s.sourceRoots() {
		if root.ID == id {
			return root, nil
		}
	}
	return sourceRoot{}, errors.New("unknown or disabled source root")
}

func sourceSafePath(path string, file bool) bool {
	if path == "" || path == "." {
		return !file
	}
	if filepath.IsAbs(path) || strings.ContainsAny(path, "\\\x00:") {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		p := strings.ToLower(part)
		if p == "" || strings.HasPrefix(p, ".") {
			return false
		}
		for _, secret := range []string{"credential", "secret", "password", "token", "config", "state", "private", "api_key", "access_key", ".pem", ".key"} {
			if strings.Contains(p, secret) {
				return false
			}
		}
		if p == "id_rsa" || p == "id_ed25519" || p == "id_dsa" || p == "id_ecdsa" || p == "auth.json" {
			return false
		}
	}
	if !file {
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py", ".xml", ".js", ".ts", ".scss", ".css", ".csv", ".md", ".txt", ".json", ".html", ".yaml", ".yml", ".toml", ".rst":
		return true
	}
	return false
}

// Pin each directory before traversing further. Identity checks reject swaps to
// in-root secret symlinks; os.Root also prevents races escaping the pinned root.
func sourceOpen(root *os.Root, path string) (*os.File, error) {
	if !sourceSafePath(path, false) {
		return nil, errors.New("source path is not allowed")
	}
	current := root
	defer func() {
		if current != root {
			current.Close()
		}
	}()
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, part := range parts[:len(parts)-1] {
		expected, err := current.Lstat(part)
		if err != nil {
			return nil, fmt.Errorf("cannot inspect source directory: %w", err)
		}
		if !expected.IsDir() || expected.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("source symlinks and non-directories are denied")
		}
		next, err := current.OpenRoot(part)
		if err != nil {
			return nil, fmt.Errorf("cannot open source directory: %w", err)
		}
		actual, err := next.Stat(".")
		if err != nil || !os.SameFile(expected, actual) {
			next.Close()
			return nil, errors.New("source directory changed during open")
		}
		if current != root {
			current.Close()
		}
		current = next
	}
	name := parts[len(parts)-1]
	expected, err := current.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("cannot inspect source path: %w", err)
	}
	if expected.Mode()&os.ModeSymlink != 0 || !(expected.IsDir() || expected.Mode().IsRegular()) {
		return nil, errors.New("source symlinks and special files are denied")
	}
	f, err := current.Open(name)
	if err != nil {
		return nil, fmt.Errorf("cannot open source path: %w", err)
	}
	actual, err := f.Stat()
	if err != nil || !os.SameFile(expected, actual) {
		f.Close()
		return nil, errors.New("source path changed during open")
	}
	return f, nil
}

type sourceFS struct{ root *os.Root }

func (s sourceFS) Open(path string) (fs.File, error) { return sourceOpen(s.root, path) }

func sourceText(root sourceRoot, path string) ([]byte, error) {
	base, err := os.OpenRoot(root.Path)
	if err != nil {
		return nil, errors.New("source root is unavailable")
	}
	defer base.Close()
	return sourceTextAt(base, path)
}

func sourceTextAt(root *os.Root, path string) ([]byte, error) {
	if !sourceSafePath(path, true) {
		return nil, errors.New("source path is not allowed")
	}
	f, err := sourceOpen(root, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > sourceFileBytes {
		return nil, errors.New("source file is not regular or exceeds 1 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(f, sourceFileBytes+1))
	if err != nil {
		return nil, errors.New("cannot read source file")
	}
	if len(data) > sourceFileBytes {
		return nil, errors.New("source file exceeds 1 MiB")
	}
	if !utf8.Valid(data) {
		return nil, errors.New("binary source file is denied")
	}
	for _, b := range data {
		if b < 32 && b != '\n' && b != '\r' && b != '\t' || b == 127 {
			return nil, errors.New("binary source file is denied")
		}
	}
	return data, nil
}

func sourceReadDefaults(in *sourceReadInput) error {
	if in.StartLine == 0 {
		in.StartLine = 1
	}
	if in.Limit == 0 {
		in.Limit = 100
	}
	if in.StartLine < 1 || in.Limit < 1 || in.Limit > 200 {
		return errors.New("start_line must be positive and limit must be 1..200")
	}
	if !sourceSafePath(in.Path, true) {
		return errors.New("source path is not allowed")
	}
	return nil
}

func sourceSearchDefaults(in *sourceSearchInput) error {
	if in.Path == "" {
		in.Path = "."
	}
	if in.Limit == 0 {
		in.Limit = 100
	}
	if in.Limit < 1 || in.Limit > 100 || in.Query == "" || len(in.Query) > 1024 || !utf8.ValidString(in.Query) {
		return errors.New("query must be 1..1024 UTF-8 bytes and limit must be 1..100")
	}
	if !sourceSafePath(in.Path, false) {
		return errors.New("source path is not allowed")
	}
	return nil
}

func sourceLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func sourceNumbered(root, path string, n int, text string) sourceLine {
	line := sourceLine{Root: root, Path: filepath.ToSlash(path), Line: n, Text: strings.TrimSuffix(text, "\r")}
	if len(line.Text) > sourceLineBytes {
		line.Text = line.Text[:sourceLineBytes]
		for !utf8.ValidString(line.Text) {
			line.Text = line.Text[:len(line.Text)-1]
		}
		line.Truncated = true
	}
	return line
}

func sourceRead(ctx context.Context, root sourceRoot, in sourceReadInput) (sourceResult, error) {
	result := sourceResult{Root: root.ID, Path: in.Path, Lines: []sourceLine{}}
	if err := sourceReadDefaults(&in); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	data, err := sourceText(root, in.Path)
	if err != nil {
		return result, err
	}
	lines := sourceLines(data)
	for n := in.StartLine; n <= len(lines) && len(result.Lines) < in.Limit; n++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		line := sourceNumbered(root.ID, in.Path, n, lines[n-1])
		result.Truncated = result.Truncated || line.Truncated
		result.Lines = append(result.Lines, line)
	}
	if len(result.Lines) > 0 && in.StartLine+len(result.Lines) <= len(lines) {
		result.NextLine = in.StartLine + len(result.Lines)
		result.Truncated = true
	}
	return result, nil
}

func sourceSearch(ctx context.Context, root sourceRoot, in sourceSearchInput) (sourceResult, error) {
	result := sourceResult{Root: root.ID, Path: in.Path, Lines: []sourceLine{}}
	if err := sourceSearchDefaults(&in); err != nil {
		return result, err
	}
	result.Path = in.Path
	base, err := os.OpenRoot(root.Path)
	if err != nil {
		return result, errors.New("source root is unavailable")
	}
	defer base.Close()
	start, err := sourceOpen(base, in.Path)
	if err != nil {
		return result, err
	}
	start.Close()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	bytesRead := 0
	stop := errors.New("source search budget exhausted")
	err = fs.WalkDir(sourceFS{base}, in.Path, func(rel string, entry fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			result.Truncated = true
			return stop
		}
		if result.Visited >= sourceVisitLimit {
			result.Truncated = true
			return stop
		}
		result.Visited++
		if walkErr != nil {
			result.Warnings = []string{"Some source paths were unavailable"}
			return nil
		}
		if !sourceSafePath(rel, false) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !sourceSafePath(rel, true) {
			return nil
		}
		// WalkDir never follows links; guarded opens also reject directory swaps.
		data, err := sourceTextAt(base, rel)
		if err != nil {
			result.Warnings = []string{"Some source files were denied, binary, oversized or unavailable"}
			return nil
		}
		bytesRead += len(data)
		if bytesRead > sourceSearchBytes {
			result.Truncated = true
			return stop
		}
		for n, text := range sourceLines(data) {
			if !strings.Contains(text, in.Query) {
				continue
			}
			if len(result.Lines) >= in.Limit {
				result.Truncated = true
				return stop
			}
			line := sourceNumbered(root.ID, rel, n+1, text)
			result.Truncated = result.Truncated || line.Truncated
			result.Lines = append(result.Lines, line)
		}
		return nil
	})
	if err != nil && !errors.Is(err, stop) {
		return result, err
	}
	if ctx.Err() != nil {
		result.Warnings = append(result.Warnings, "Source search deadline or cancellation reached")
	}
	return result, nil
}

type sourceModule struct {
	Root     string              `json:"root"`
	Path     string              `json:"path"`
	Manifest module.ManifestInfo `json:"manifest"`
}

func (s *Server) sourceModules(ctx context.Context, name string) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result := struct {
		Modules   []sourceModule `json:"modules"`
		Warnings  []string       `json:"warnings"`
		Truncated bool           `json:"truncated"`
	}{Modules: []sourceModule{}, Warnings: []string{}}
	seen := map[string]string{}
	count := 0
	for _, root := range s.sourceRoots() {
		if root.Container {
			continue
		}
		base, err := os.OpenRoot(root.Path)
		if err != nil {
			result.Warnings = append(result.Warnings, "Unavailable module root: "+root.ID)
			continue
		}
		defer base.Close()
		dir, err := sourceOpen(base, ".")
		if err != nil {
			result.Warnings = append(result.Warnings, "Unavailable module root: "+root.ID)
			continue
		}
		entries, err := dir.ReadDir(-1)
		dir.Close()
		if err != nil {
			result.Warnings = append(result.Warnings, "Unavailable module root: "+root.ID)
			continue
		}
		// File.ReadDir follows filesystem order; keep module results deterministic.
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			candidate := entry.Name()
			count++
			if count > sourceVisitLimit || ctx.Err() != nil {
				result.Truncated = true
				return result, nil
			}
			if !sourceSafePath(candidate, false) {
				continue
			}
			manifestPath := filepath.Join(candidate, "__manifest__.py")
			data, err := sourceTextAt(base, manifestPath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				result.Warnings = append(result.Warnings, "Denied or unavailable manifest: "+root.ID+"/"+candidate)
				continue
			}
			info := module.ParseManifestContent(candidate, data)
			info.Module = candidate
			info.Path = filepath.ToSlash(manifestPath)
			if previous, ok := seen[candidate]; ok {
				result.Warnings = append(result.Warnings, "Module collision: "+candidate+" in "+previous+" and "+root.ID)
			} else {
				seen[candidate] = root.ID
			}
			if name == "" || candidate == name {
				result.Modules = append(result.Modules, sourceModule{Root: root.ID, Path: candidate, Manifest: info})
			}
		}
	}
	if name != "" && len(result.Modules) == 0 {
		result.Warnings = append(result.Warnings, "Module not found in local roots: "+name)
	}
	for _, item := range result.Modules {
		for _, dep := range item.Manifest.Depends {
			if _, ok := seen[dep]; !ok {
				result.Warnings = append(result.Warnings, "Dependency not found in local roots: "+item.Manifest.Module+" -> "+dep+" (may be container-provided)")
			}
		}
	}
	return result, nil
}

func (s *Server) sourceContainer(ctx context.Context, root sourceRoot, operation string, input any) (sourceResult, error) {
	request, err := json.Marshal(struct {
		Root      sourceRoot `json:"root"`
		Operation string     `json:"operation"`
		Input     any        `json:"input"`
	}{root, operation, input})
	if err != nil {
		return sourceResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	data, err := s.run(ctx, string(request), "exec", "-T", "odoo", "/opt/odoo-venv/bin/python3", "-c", sourceContainerScript)
	if err != nil {
		return sourceResult{}, errors.New("container source lookup failed; verify the source root and running Odoo container")
	}
	var result sourceResult
	if err := json.Unmarshal(data, &result); err != nil {
		return result, errors.New("invalid container source response")
	}
	return result, nil
}

// User strings travel only as JSON stdin, never as executable Python or shell.
const sourceContainerScript = `import json, os, pathlib, stat, sys, time
r = json.load(sys.stdin)
root, a = r['root'], r['input']
extensions = {'.py','.xml','.js','.ts','.scss','.css','.csv','.md','.txt','.json','.html','.yaml','.yml','.toml','.rst'}
def safe(p, file=False):
    if p in ('', '.'): return not file
    if p.startswith('/') or any(c in p for c in ('\\','\x00',':')): return False
    for part in p.split('/'):
        v = part.lower()
        if not v or v.startswith('.') or v in ('id_rsa','id_ed25519','id_dsa','id_ecdsa','auth.json'): return False
        if any(s in v for s in ('credential','secret','password','token','config','state','private','api_key','access_key','.pem','.key')): return False
    return not file or pathlib.Path(p).suffix.lower() in extensions
if root['id'] not in ('odoo-core','odoo-enterprise'): raise ValueError('invalid root')
expected = {'odoo-core':'/opt/odoo-src','odoo-enterprise':'/opt/odoo-enterprise'}[root['id']]
directory_flags = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW
base_fd = os.open(expected, directory_flags)
def open_path(p):
    if not safe(p): raise ValueError('source path is not allowed')
    current = os.dup(base_fd)
    try:
        if p in ('', '.'): return os.dup(current)
        parts = p.split('/')
        for part in parts[:-1]:
            child = os.open(part, directory_flags, dir_fd=current)
            os.close(current)
            current = child
        return os.open(parts[-1], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=current)
    finally: os.close(current)
def text(p):
    if not safe(p, True): raise ValueError('source path is not allowed')
    with os.fdopen(open_path(p), 'rb') as f:
        info = os.fstat(f.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_size > 1048576: raise ValueError('file size or type denied')
        data = f.read(1048577)
    if len(data) > 1048576 or any(b < 32 and b not in (9,10,13) or b == 127 for b in data): raise ValueError('binary or oversized file denied')
    return data.decode('utf-8'), len(data)
def lines(t):
    return t.removesuffix('\n').split('\n') if t else []
p = a.get('path') or '.'
out = {'root':root['id'], 'path':p, 'lines':[], 'truncated':False, 'visited':0, 'warnings':[]}
def append(p, n, t):
    t = t.removesuffix('\r')
    clipped = len(t.encode('utf-8')) > 2000
    if clipped: t = t.encode('utf-8')[:2000].decode('utf-8', errors='ignore')
    out['lines'].append({'root':root['id'], 'path':p, 'line':n, 'text':t, 'truncated':clipped})
    out['truncated'] |= clipped
if r['operation'] == 'read':
    start, limit = a.get('start_line') or 1, a.get('limit') or 100
    if start < 1 or not 1 <= limit <= 200: raise ValueError('invalid page')
    ls = lines(text(p)[0])
    for n in range(start, min(len(ls)+1, start+limit)): append(p, n, ls[n-1])
    if out['lines'] and start+len(out['lines']) <= len(ls):
        out['truncated'] = True
        out['next_line'] = start+len(out['lines'])
elif r['operation'] == 'search':
    query, limit = a['query'], a.get('limit') or 100
    if not query or len(query.encode('utf-8')) > 1024 or not 1 <= limit <= 100: raise ValueError('invalid search')
    start_fd = open_path(p)
    deadline, budget = time.monotonic()+5, 0
    def paths(directory_fd, prefix):
        if stat.S_ISREG(os.fstat(directory_fd).st_mode):
            out['visited'] = 1
            yield prefix
            return
        with os.scandir(directory_fd) as scan: entries = sorted(scan, key=lambda e: e.name)
        for entry in entries:
            q = entry.name if prefix == '.' else prefix+'/'+entry.name
            if out['visited'] >= 10000 or time.monotonic() >= deadline:
                out['truncated'] = True
                return
            out['visited'] += 1
            if not safe(q) or entry.is_symlink(): continue
            if entry.is_dir(follow_symlinks=False):
                try: child = os.open(entry.name, directory_flags, dir_fd=directory_fd)
                except OSError:
                    out['warnings'] = ['Some source paths were unavailable']
                    continue
                try: yield from paths(child, q)
                finally: os.close(child)
                if out['visited'] >= 10000 or time.monotonic() >= deadline:
                    out['truncated'] = True
                    return
            elif safe(q, True): yield q
    walker = paths(start_fd, p)
    for q in walker:
        if time.monotonic() >= deadline:
            out['truncated'] = True
            break
        try: t, size = text(q)
        except (OSError, ValueError, UnicodeError):
            out['warnings'] = ['Some source files were denied, binary, oversized or unavailable']
            continue
        budget += size
        if budget > 16777216:
            out['truncated'] = True
            break
        stop = False
        for n, line in enumerate(lines(t), 1):
            if query not in line: continue
            if len(out['lines']) >= limit:
                out['truncated'], stop = True, True
                break
            append(q, n, line)
        if stop: break
    walker.close()
    os.close(start_fd)
else: raise ValueError('invalid operation')
os.close(base_fd)
print(json.dumps(out))
`
