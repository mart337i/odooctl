// Package browserruntime owns an isolated, explicitly prepared Docker MCP runtime.
package browserruntime

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mart337i/odooctl/internal/config"
)

//go:embed files/*
var files embed.FS

const label = "io.odooctl.browserruntime"
const internalURL = "http://odoo:8069"

type Runtime struct {
	Project        string
	PublicURL      string
	InternalURL    string
	Directory      string
	ArtifactsDir   string
	BrowserNetwork string
	ProxyContainer string
	ProxyIP        string `json:"-"`
	BrowserImage   string
	AllowedOrigins []string
	OdooNetwork    string
	OdooIP         string
	PolicyHash     string
	ProxyImage     string
	SourceImages   map[string]string
}

var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var workerName = regexp.MustCompile(`^odooctl-browser-[a-f0-9]{16}-worker-[a-f0-9]{24}$`)
var imageID = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func normalizeOrigins(origins []string) ([]string, error) {
	result := []string{}
	seen := map[string]bool{}
	for _, raw := range origins {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") || (u.Path != "" && u.Path != "/") || u.RawPath != "" {
			return nil, fmt.Errorf("approved origins must be exact HTTPS origins")
		}
		host := strings.ToLower(u.Hostname())
		if len(host) > 253 || !strings.Contains(host, ".") || net.ParseIP(host) != nil || (u.Port() != "" && u.Port() != "443") || strings.HasSuffix(u.Host, ":") {
			return nil, fmt.Errorf("approved origins require DNS names and port 443")
		}
		for _, part := range strings.Split(host, ".") {
			if !dnsLabel.MatchString(part) {
				return nil, fmt.Errorf("invalid origin DNS label")
			}
		}
		// Reject numeric hosts, including legacy browser IPv4 spellings.
		last := host[strings.LastIndex(host, ".")+1:]
		if _, err := strconv.ParseUint(last, 10, 64); err == nil {
			return nil, fmt.Errorf("numeric origin host is forbidden")
		}
		origin := "https://" + host
		if !seen[origin] {
			result = append(result, origin)
			seen[origin] = true
		}
	}
	sort.Strings(result)
	return result, nil
}

func squidConfig(ip string, origins []string) string {
	var b strings.Builder
	b.WriteString("http_port 3128\nvisible_hostname odooctl-proxy\naccess_log none\ncache_log /dev/null\ncache_store_log none\ncache deny all\ncache_mem 0 MB\npid_filename /tmp/squid.pid\nshutdown_lifetime 1 seconds\nhosts_file /etc/hosts\nacl connect method CONNECT\nacl http proto HTTP\nacl odoo_host dstdomain -n odoo\nacl odoo_port port 8069\n")
	fmt.Fprintf(&b, "acl odoo_ip dst %s/32\nhttp_access allow !connect http odoo_host odoo_port odoo_ip\n", ip)
	// Playwright's APIRequestContext uses CONNECT even for plain HTTP. This
	// exception reaches only the same inspected Odoo HTTP service, never peers.
	b.WriteString("http_access allow connect odoo_host odoo_port odoo_ip\n")
	if len(origins) == 0 {
		b.WriteString("http_access deny all\n")
		return b.String()
	}
	// Reject unapproved requests without resolving attacker-controlled names.
	b.WriteString("acl tls_port port 443\nacl approved dstdomain -n")
	for _, origin := range origins {
		b.WriteString(" " + strings.TrimPrefix(origin, "https://"))
	}
	b.WriteString("\nhttp_access deny !connect\nhttp_access deny !tls_port\nhttp_access deny !approved\n")
	// Failed lookups must not fall through to a hostname-only CONNECT grant.
	// "all" is an IP argument to the slow dst ACL, not the always-true all ACL:
	// DestinationIP::match requires a cached address even with this IP range.
	b.WriteString("acl resolved dst all\nhttp_access deny !resolved\n")
	// Squid maps IPv4 into IPv6 internally. Keep the trees separate and never
	// include ::ffff:0:0/96 in reserved6, which would deny public IPv4 as well.
	b.WriteString("acl reserved4 dst 0.0.0.0/8 10.0.0.0/8 100.64.0.0/10 127.0.0.0/8 169.254.0.0/16 172.16.0.0/12 192.0.0.0/24 192.0.2.0/24 192.31.196.0/24 192.52.193.0/24 192.88.99.0/24 192.168.0.0/16 192.175.48.0/24 198.18.0.0/15 198.51.100.0/24 203.0.113.0/24 224.0.0.0/3\nhttp_access deny reserved4\n")
	b.WriteString("acl reserved6 dst ::/96 ::ffff:0:0:0/96 64:ff9b::/96 64:ff9b:1::/48 100::/64 1000::/4 2001::/23 2001:db8::/32 2002::/16 2620:4f:8000::/48 3fff::/20 4000::/2 8000::/1\nhttp_access deny reserved6\nhttp_access allow connect tls_port approved\n")
	b.WriteString("http_access deny all\n")
	return b.String()
}

func docker(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Docker %s failed", args[0])
	}
	return out, nil
}

type containerInfo struct {
	ID     string
	Config struct {
		Labels map[string]string
		Image  string
	}
	State struct {
		Running  bool
		ExitCode int
		Pid      int
	}
	NetworkSettings struct {
		Networks map[string]struct{ IPAddress string }
	}
}

func inspectContainer(ctx context.Context, name string) (*containerInfo, error) {
	data, err := docker(ctx, "inspect", name)
	if err != nil {
		return nil, err
	}
	var items []containerInfo
	if json.Unmarshal(data, &items) != nil || len(items) != 1 {
		return nil, fmt.Errorf("invalid Docker container inspection")
	}
	return &items[0], nil
}

func resolve(ctx context.Context, project string, discover bool) (*Runtime, error) {
	state, err := config.LookupFromDir(project)
	if err != nil {
		return nil, fmt.Errorf("project environment lookup failed: %w", err)
	}
	root, err := filepath.Abs(state.ProjectRoot)
	if err != nil {
		return nil, err
	}
	env, err := config.EnvironmentDir(state.ProjectName, state.Branch)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(root + "\x00" + env))
	key := hex.EncodeToString(sum[:8])
	dir := filepath.Join(env, "browser-runtime")
	r := &Runtime{Project: root, PublicURL: fmt.Sprintf("http://localhost:%d", state.Ports.Odoo), InternalURL: internalURL, Directory: dir, ArtifactsDir: filepath.Join(dir, "artifacts"), BrowserNetwork: "odooctl-browser-" + key, ProxyContainer: "odooctl-browser-" + key + "-proxy"}
	if !discover {
		return r, nil
	}
	probeCtx, probeCancel := context.WithTimeout(ctx, 5*time.Second)
	probe := exec.CommandContext(probeCtx, "docker", "compose", "version")
	probe.WaitDelay = time.Second
	probeErr := probe.Run()
	probeCancel()
	if probeErr != nil {
		return nil, fmt.Errorf("Docker Compose plugin unavailable or probe cancelled")
	}
	cmd := exec.CommandContext(ctx, "docker", "compose", "ps", "-q", "odoo")
	cmd.Dir = env
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Odoo service discovery failed")
	}
	ids := strings.Fields(string(out))
	if len(ids) != 1 {
		return nil, fmt.Errorf("exactly one running Odoo service is required")
	}
	info, err := inspectContainer(ctx, ids[0])
	if err != nil {
		return nil, err
	}
	if !info.State.Running {
		return nil, fmt.Errorf("Odoo must be running")
	}
	names := []string{}
	for name, n := range info.NetworkSettings.Networks {
		if net.ParseIP(n.IPAddress).To4() != nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("Odoo requires an IPv4 Docker network")
	}
	r.OdooNetwork = names[0]
	r.OdooIP = info.NetworkSettings.Networks[names[0]].IPAddress
	return r, nil
}

func (r *Runtime) labels(kind string) []string {
	return []string{"--label", label + ".managed=true", "--label", label + ".project=" + r.Project, "--label", label + ".kind=" + kind, "--label", label + ".policy=" + r.PolicyHash}
}

func (r *Runtime) workers(ctx context.Context) ([]string, error) {
	out, err := docker(ctx, "ps", "-aq", "--filter", "label="+label+".managed=true", "--filter", "label="+label+".project="+r.Project, "--filter", "label="+label+".kind=worker")
	return strings.Fields(string(out)), err
}

func privateDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}

func privateFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

// Prepare builds and starts only the browser runtime, never the Odoo environment.
func Prepare(ctx context.Context, project string, origins []string) (*Runtime, error) {
	allowed, err := normalizeOrigins(origins)
	if err != nil {
		return nil, err
	}
	r, err := resolve(ctx, project, true)
	if err != nil {
		return nil, err
	}
	r.AllowedOrigins = allowed
	policy := squidConfig(r.OdooIP, allowed)
	sum := sha256.Sum256([]byte(policy))
	r.PolicyHash = hex.EncodeToString(sum[:])
	workers, err := r.workers(ctx)
	if err != nil {
		return nil, err
	}
	if len(workers) > 0 {
		return nil, fmt.Errorf("stop browser workers before preparing runtime")
	}
	// Existing names must be owned by this project before any replacement.
	if err := r.removeManaged(ctx); err != nil {
		return nil, err
	}
	for _, dir := range []string{r.Directory, r.ArtifactsDir, filepath.Join(r.Directory, "build")} {
		if err := privateDir(dir); err != nil {
			return nil, err
		}
	}
	build := filepath.Join(r.Directory, "build")
	for _, name := range []string{"browser.Dockerfile", "proxy.Dockerfile", "init-page.ts", "init-script.js", "config.json", "patch-snapshots.js"} {
		data, _ := files.ReadFile("files/" + name)
		if err := privateFile(filepath.Join(build, name), data); err != nil {
			return nil, err
		}
	}
	if err := privateFile(filepath.Join(build, "squid.conf"), []byte(policy)); err != nil {
		return nil, err
	}
	r.SourceImages = map[string]string{}
	for _, base := range []string{"node:22-bookworm-slim", "debian:bookworm-slim"} {
		if _, err := docker(ctx, "pull", base); err != nil {
			return nil, err
		}
		out, err := docker(ctx, "image", "inspect", "--format", "{{index .RepoDigests 0}}", base)
		if err != nil {
			return nil, err
		}
		pinned := strings.TrimSpace(string(out))
		if !strings.Contains(pinned, "@sha256:") {
			return nil, fmt.Errorf("source image digest unavailable")
		}
		r.SourceImages[base] = pinned
	}
	for _, kind := range []string{"browser", "proxy"} {
		base := "node:22-bookworm-slim"
		if kind == "proxy" {
			base = "debian:bookworm-slim"
		}
		tag := r.BrowserNetwork + "-" + kind + ":" + r.PolicyHash[:16]
		cmd := exec.CommandContext(ctx, "docker", "build", "--build-arg", "BASE="+r.SourceImages[base], "-f", filepath.Join(build, kind+".Dockerfile"), "-t", tag, build)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if cmd.Run() != nil {
			return nil, fmt.Errorf("browser runtime image build failed")
		}
		out, err := docker(ctx, "image", "inspect", "--format", "{{.Id}}", tag)
		if err != nil {
			return nil, err
		}
		if kind == "browser" {
			r.BrowserImage = strings.TrimSpace(string(out))
		} else {
			r.ProxyImage = strings.TrimSpace(string(out))
		}
	}
	args := append([]string{"network", "create", "--driver", "bridge", "--internal", "--opt", "com.docker.network.bridge.inhibit_ipv4=true", "--ipv6=false"}, r.labels("network")...)
	args = append(args, r.BrowserNetwork)
	if _, err := docker(ctx, args...); err != nil {
		return nil, err
	}
	args = append([]string{"create", "--init", "--name", r.ProxyContainer, "--network", r.OdooNetwork, "--add-host", "odoo:" + r.OdooIP, "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--tmpfs", "/tmp:rw,nosuid,nodev,mode=1777", "--user", "proxy", "--memory", "256m"}, r.labels("proxy")...)
	args = append(args, r.ProxyImage)
	if _, err := docker(ctx, args...); err != nil {
		return nil, err
	}
	if _, err := docker(ctx, "network", "connect", "--alias", "proxy", r.BrowserNetwork, r.ProxyContainer); err != nil {
		return nil, err
	}
	if _, err := docker(ctx, "start", r.ProxyContainer); err != nil {
		return nil, err
	}
	if err := r.waitProxy(ctx); err != nil {
		return nil, err
	}
	data, _ := json.MarshalIndent(r, "", "  ")
	if err := privateFile(filepath.Join(r.Directory, "runtime.json"), data); err != nil {
		return nil, err
	}
	return Load(ctx, project)
}

func (r *Runtime) waitProxy(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// Validate the immutable configuration before publishing prepared state.
	if _, err := docker(ctx, "exec", r.ProxyContainer, "squid", "-k", "parse", "-f", "/etc/squid/squid.conf"); err != nil {
		return fmt.Errorf("browser proxy configuration validation failed")
	}
	for {
		info, err := inspectContainer(ctx, r.ProxyContainer)
		if err != nil {
			return fmt.Errorf("browser proxy startup inspection failed")
		}
		if !r.owns(info.Config.Labels, "proxy") || !info.State.Running || info.State.Pid <= 0 {
			return fmt.Errorf("browser proxy stopped during startup (exit code %d)", info.State.ExitCode)
		}
		// check signals the daemon identified by Squid's configured PID file.
		if _, err := docker(ctx, "exec", r.ProxyContainer, "squid", "-k", "check", "-f", "/etc/squid/squid.conf"); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("browser proxy startup did not complete")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func readRuntime(ctx context.Context, project string) (*Runtime, error) {
	current, err := resolve(ctx, project, true)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(current.Directory, "runtime.json"))
	if err != nil {
		return nil, fmt.Errorf("browser runtime is not prepared")
	}
	var r Runtime
	if json.Unmarshal(data, &r) != nil {
		return nil, fmt.Errorf("invalid browser runtime state")
	}
	if r.Project != current.Project || r.Directory != current.Directory || r.ArtifactsDir != current.ArtifactsDir || r.BrowserNetwork != current.BrowserNetwork || r.ProxyContainer != current.ProxyContainer || r.InternalURL != internalURL {
		return nil, fmt.Errorf("invalid browser runtime identity")
	}
	if r.OdooIP != current.OdooIP || r.OdooNetwork != current.OdooNetwork || r.PublicURL != current.PublicURL {
		return nil, fmt.Errorf("Odoo network changed; prepare browser runtime again")
	}
	origins, err := normalizeOrigins(r.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(squidConfig(r.OdooIP, origins)))
	if r.PolicyHash != hex.EncodeToString(sum[:]) {
		return nil, fmt.Errorf("browser runtime policy mismatch")
	}
	if !imageID.MatchString(r.BrowserImage) || !imageID.MatchString(r.ProxyImage) {
		return nil, fmt.Errorf("browser runtime requires immutable image IDs")
	}
	return &r, nil
}

// Load is passive: it inspects state and Docker resources without starting them.
func Load(ctx context.Context, project string) (*Runtime, error) {
	r, err := readRuntime(ctx, project)
	if err != nil {
		return nil, err
	}
	info, err := inspectContainer(ctx, r.ProxyContainer)
	if err != nil {
		return nil, err
	}
	if !r.owns(info.Config.Labels, "proxy") || !info.State.Running || info.Config.Image != r.ProxyImage || len(info.NetworkSettings.Networks) != 2 {
		return nil, fmt.Errorf("browser proxy identity or policy mismatch; prepare again")
	}
	if _, ok := info.NetworkSettings.Networks[r.OdooNetwork]; !ok {
		return nil, fmt.Errorf("proxy Odoo network mismatch")
	}
	if _, ok := info.NetworkSettings.Networks[r.BrowserNetwork]; !ok {
		return nil, fmt.Errorf("proxy private network mismatch")
	}
	proxyIP := net.ParseIP(info.NetworkSettings.Networks[r.BrowserNetwork].IPAddress).To4()
	if proxyIP == nil || proxyIP.IsUnspecified() || proxyIP.IsLoopback() || proxyIP.IsMulticast() {
		return nil, fmt.Errorf("proxy private network IPv4 address unavailable")
	}
	r.ProxyIP = proxyIP.String()
	if err := r.checkNetwork(ctx, true); err != nil {
		return nil, err
	}
	if _, err := docker(ctx, "image", "inspect", r.BrowserImage); err != nil {
		return nil, fmt.Errorf("browser image unavailable; prepare again")
	}
	return r, nil
}

func (r *Runtime) owns(labels map[string]string, kind string) bool {
	return labels[label+".managed"] == "true" && labels[label+".project"] == r.Project && labels[label+".kind"] == kind && (r.PolicyHash == "" || labels[label+".policy"] == r.PolicyHash)
}

func (r *Runtime) checkNetwork(ctx context.Context, members bool) error {
	out, err := docker(ctx, "network", "inspect", r.BrowserNetwork)
	if err != nil {
		return err
	}
	var networks []struct {
		Driver     string
		EnableIPv6 bool
		Options    map[string]string
		Internal   bool
		Labels     map[string]string
		Containers map[string]struct{ Name string }
	}
	if json.Unmarshal(out, &networks) != nil || len(networks) != 1 || !networks[0].Internal || !r.owns(networks[0].Labels, "network") {
		return fmt.Errorf("browser private network identity mismatch")
	}
	// Cleanup checks ownership only, allowing explicit removal of old networks.
	if r.PolicyHash != "" && (networks[0].Driver != "bridge" || networks[0].EnableIPv6 || networks[0].Options["com.docker.network.bridge.inhibit_ipv4"] != "true") {
		return fmt.Errorf("browser network isolation options mismatch; prepare again")
	}
	if members {
		for id, member := range networks[0].Containers {
			if member.Name == r.ProxyContainer {
				continue
			}
			info, err := inspectContainer(ctx, id)
			if err != nil {
				return err
			}
			if !r.owns(info.Config.Labels, "worker") || len(info.NetworkSettings.Networks) != 1 {
				return fmt.Errorf("unexpected private network member")
			}
		}
	}
	return nil
}

// Command creates a launch command; credentials exist only in its environment.
func (r *Runtime) Command(ctx context.Context, public bool, login, password string) (*exec.Cmd, string, error) {
	checked, err := Load(ctx, r.Project)
	if err != nil {
		return nil, "", err
	}
	if checked.PolicyHash != r.PolicyHash || checked.BrowserImage != r.BrowserImage {
		return nil, "", fmt.Errorf("runtime changed; reload before launching")
	}
	// Exported metadata is not authority for mount paths or Docker arguments.
	r = checked
	if !public && (login == "" || password == "") {
		return nil, "", fmt.Errorf("browser login credentials required")
	}
	token := make([]byte, 12)
	if _, err := rand.Read(token); err != nil {
		return nil, "", err
	}
	name := r.BrowserNetwork + "-worker-" + hex.EncodeToString(token)
	dir, err := r.SessionArtifacts(name)
	if err != nil {
		return nil, "", err
	}
	if err := privateDir(dir); err != nil {
		return nil, "", err
	}
	uid, gid := "1000", "1000"
	if u, err := user.Current(); err == nil {
		if n, e := strconv.Atoi(u.Uid); e == nil && n > 0 {
			uid = u.Uid
		}
		if n, e := strconv.Atoi(u.Gid); e == nil && n > 0 {
			gid = u.Gid
		}
	}
	// Backend logs/snapshots never touch the host. Only the adapter exports images.
	args := append([]string{"run", "-i", "--rm", "--init", "--pull=never", "--log-driver=none", "--name", name, "--network", r.BrowserNetwork, "--dns", "127.0.0.1", "--add-host", "proxy:" + r.ProxyIP, "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--tmpfs", "/tmp:rw,nosuid,nodev,size=128m,mode=1777", "--tmpfs", "/artifacts:rw,nosuid,nodev,size=64m,uid=" + uid + ",gid=" + gid + ",mode=0700", "--shm-size", "256m", "--memory", "1536m", "--user", uid + ":" + gid, "-e", "HOME=/tmp"}, r.labels("worker")...)
	env := []string{"ODOOCTL_BROWSER_PUBLIC=" + strconv.FormatBool(public), "ODOOCTL_BROWSER_URL=" + internalURL}
	if !public {
		env = append(env, "ODOOCTL_BROWSER_LOGIN="+login, "ODOOCTL_BROWSER_PASSWORD="+password)
	}
	for _, v := range env {
		args = append(args, "-e", strings.SplitN(v, "=", 2)[0])
	}
	args = append(args, r.BrowserImage, "--config", "/opt/runtime/config.json", "--headless", "--browser", "chrome", "--executable-path", "/opt/chromium", "--isolated", "--no-sandbox", "--no-webmcp", "--block-service-workers", "--snapshot-boxes", "--codegen", "none", "--output-dir", "/artifacts", "--output-max-size", "16777216", "--proxy-server", "http://proxy:3128", "--proxy-bypass", "<-loopback>", "--timeout-navigation", "30000", "--timeout-action", "5000", "--init-script", "/opt/runtime/init-script.js", "--init-page", "/opt/runtime/init-page.ts")
	cmd := exec.CommandContext(ctx, "docker", args...)
	// Do not inherit upstream MCP configuration or stale browser credentials.
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "ODOOCTL_BROWSER_") && !strings.HasPrefix(v, "PLAYWRIGHT_MCP_") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, env...)
	return cmd, name, nil
}

// SessionArtifacts locates the host export directory; it is never mounted into
// the worker. The adapter must write only validated/redacted artifacts here.
func (r *Runtime) SessionArtifacts(name string) (string, error) {
	if !workerName.MatchString(name) || !strings.HasPrefix(name, r.BrowserNetwork+"-worker-") {
		return "", fmt.Errorf("invalid browser worker name")
	}
	return filepath.Join(r.ArtifactsDir, "session-"+strings.TrimPrefix(name, r.BrowserNetwork+"-worker-")), nil
}

func (r *Runtime) StopWorker(ctx context.Context, name string) error {
	if !workerName.MatchString(name) || !strings.HasPrefix(name, r.BrowserNetwork+"-worker-") {
		return fmt.Errorf("invalid browser worker name")
	}
	info, err := inspectContainer(ctx, name)
	if err != nil {
		return err
	}
	if !r.owns(info.Config.Labels, "worker") {
		return fmt.Errorf("refusing to remove unmanaged browser worker")
	}
	_, err = docker(ctx, "rm", "-f", name)
	return err
}

// Stop retains all artifacts, images and Odoo data, and refuses existing workers.
func Stop(ctx context.Context, project string) error {
	r, err := resolve(ctx, project, false)
	if err != nil {
		return err
	}
	workers, err := r.workers(ctx)
	if err != nil {
		return err
	}
	if len(workers) > 0 {
		return fmt.Errorf("stop browser workers before stopping runtime")
	}
	return r.removeManaged(ctx)
}

func (r *Runtime) removeManaged(ctx context.Context) error {
	// Listing first distinguishes absent resources from failed Docker inspection.
	out, err := docker(ctx, "ps", "-a", "--format", "{{.Names}}")
	if err != nil {
		return err
	}
	for _, name := range strings.Fields(string(out)) {
		if name != r.ProxyContainer {
			continue
		}
		info, err := inspectContainer(ctx, name)
		if err != nil {
			return err
		}
		saved := r.PolicyHash
		r.PolicyHash = ""
		owned := r.owns(info.Config.Labels, "proxy")
		r.PolicyHash = saved
		if !owned {
			return fmt.Errorf("refusing unmanaged proxy removal")
		}
		if _, err := docker(ctx, "rm", "-f", name); err != nil {
			return err
		}
	}
	out, err = docker(ctx, "network", "ls", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	for _, name := range strings.Fields(string(out)) {
		if name != r.BrowserNetwork {
			continue
		}
		saved := r.PolicyHash
		r.PolicyHash = ""
		err := r.checkNetwork(ctx, true)
		r.PolicyHash = saved
		if err != nil {
			return err
		}
		if _, err := docker(ctx, "network", "rm", name); err != nil {
			return err
		}
	}
	return nil
}
