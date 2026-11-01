package browserruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mart337i/odooctl/internal/config"
)

func TestNormalizeOrigins(t *testing.T) {
	got, err := normalizeOrigins([]string{"https://CDN.Example.com:443/", "https://cdn.example.com", "https://xn--bcher-kva.example"})
	if err != nil || !reflect.DeepEqual(got, []string{"https://cdn.example.com", "https://xn--bcher-kva.example"}) {
		t.Fatalf("normalize: %v %v", got, err)
	}
	for _, origin := range []string{"http://example.com", "https://example.com:444", "https://example.com:", "https://example.com/a", "https://example.com/%2f", "https://user:pass@example.com", "https://example.com?", "https://example.com#", "https://example.com?q=x", "https://*.example.com", "https://example.com.", "https://-bad.example", "https://bad_.example", "https://bücher.example", "https://127.0.0.1", "https://[::1]", "https://2130706433", "https://0x7f.0.0.1", "https://example.com\nhttp_access allow all", "https://example.com;foo", "https://localhost", "https://example..com"} {
		t.Run(origin, func(t *testing.T) {
			if _, err := normalizeOrigins([]string{origin}); err == nil {
				t.Fatalf("accepted unsafe origin %q", origin)
			}
		})
	}
}

func TestSquidPolicy(t *testing.T) {
	text := squidConfig("172.22.0.7", []string{"https://cdn.example.com"})
	rules := []string{"http_access allow !connect http odoo_host odoo_port odoo_ip", "http_access allow connect odoo_host odoo_port odoo_ip", "http_access deny !connect", "http_access deny !tls_port", "http_access deny !approved", "http_access deny !resolved", "http_access deny reserved4", "http_access deny reserved6", "http_access allow connect tls_port approved", "http_access deny all"}
	last := -1
	for _, rule := range rules {
		at := strings.Index(text, rule)
		if at <= last {
			t.Fatalf("ACL ordering for %q", rule)
		}
		last = at
	}
	for _, required := range []string{"dstdomain -n odoo", "dst 172.22.0.7/32", "odoo_port port 8069", "tls_port port 443", "approved dstdomain -n cdn.example.com", "access_log none", "cache_log /dev/null", "cache deny all", "100.64.0.0/10", "169.254.0.0/16", "acl resolved dst all", "::/96", "::ffff:0:0:0/96", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "8000::/1", "2001:db8::/32"} {
		if !strings.Contains(text, required) {
			t.Errorf("missing %q", required)
		}
	}
	for _, forbidden := range []string{"ssl_bump", "http_access allow all", ".cdn.example.com", "request_header_access", "reply_header_access", "0.0.0.0/0", " ::/0", " ::/3 ", "::ffff:0:0/96"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("unexpected %q", forbidden)
		}
	}
	closed := squidConfig("172.22.0.7", nil)
	if strings.Contains(closed, "allow connect tls_port") || strings.Contains(closed, "acl resolved") || strings.Contains(closed, "acl reserved") || !strings.HasSuffix(closed, "http_access allow connect odoo_host odoo_port odoo_ip\nhttp_access deny all\n") {
		t.Fatal("unapproved traffic may reach destination DNS resolution")
	}
}

func TestSquidReservedTreesDoNotOverlap(t *testing.T) {
	for _, line := range strings.Split(squidConfig("172.22.0.7", []string{"https://cdn.example.com"}), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || (fields[1] != "reserved4" && fields[1] != "reserved6") {
			continue
		}
		prefixes := []netip.Prefix{}
		for _, cidr := range fields[3:] {
			prefix, err := netip.ParsePrefix(cidr)
			if err != nil {
				t.Fatal(err)
			}
			if prefix.Addr().Is4() != (fields[1] == "reserved4") {
				t.Fatal("mixed address families in Squid ACL")
			}
			if fields[1] == "reserved6" && prefix.Contains(netip.MustParseAddr("::ffff:8.8.8.8")) {
				t.Fatal("IPv6 ACL denies public IPv4 mapping")
			}
			for _, previous := range prefixes {
				if previous.Overlaps(prefix) {
					t.Fatalf("overlapping Squid CIDRs: %s %s", previous, prefix)
				}
			}
			prefixes = append(prefixes, prefix)
		}
	}
}

func TestSquidOdooAPITunnelIsBounded(t *testing.T) {
	for _, origins := range [][]string{nil, {"https://cdn.example.com"}} {
		policy := squidConfig("172.30.9.6", origins)
		for _, bound := range []string{"acl odoo_host dstdomain -n odoo\n", "acl odoo_port port 8069\n", "acl odoo_ip dst 172.30.9.6/32\n"} {
			if !strings.Contains(policy, bound) {
				t.Fatalf("missing exact API tunnel bound %q", bound)
			}
		}
		tunnel := "http_access allow connect odoo_host odoo_port odoo_ip"
		if strings.Count(policy, tunnel) != 1 {
			t.Fatal("API HTTP transport requires exactly one pinned Odoo CONNECT exception")
		}
		for _, rule := range strings.Split(policy, "\n") {
			if !strings.HasPrefix(rule, "http_access allow ") {
				continue
			}
			switch rule {
			case "http_access allow !connect http odoo_host odoo_port odoo_ip", tunnel:
			case "http_access allow connect tls_port approved":
				if len(origins) == 0 {
					t.Fatal("external CONNECT permitted without approved origins")
				}
			default:
				t.Fatalf("unexpected broadened access rule %q", rule)
			}
		}
	}
}

const fakeDocker = `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_CALLS"
case "$1 $2" in
 'compose version') if [ "$FAKE_PROBE_DELAY" = true ]; then exec sleep 1; fi; exit 0;;
 'compose ls') exit 0;;
 'compose ps') printf '%s\n' odoo-id; exit 0;;
 'ps -aq') printf '%s' "$FAKE_WORKERS"; exit 0;;
 'ps -a') printf '%s\n' "$FAKE_NAMES"; exit 0;;
 'network ls') printf '%s\n' "$FAKE_NETWORK_NAMES"; exit 0;;
 'network inspect') printf '%s\n' "$FAKE_NETWORK"; exit 0;;
 'image inspect')
   if [ "$4" = '{{index .RepoDigests 0}}' ]; then printf '%s\n' 'base@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
   elif [ "$3" = '--format' ]; then printf '%s\n' 'sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb';
   else printf '%s\n' '[]'; fi
   exit 0;;
 'exec '*)
   if [ "$5" = parse ]; then exit "${FAKE_PARSE_EXIT:-0}"; fi
   if [ "$5" = check ]; then
     if [ "$FAKE_CHECK_PENDING" = true ] && [ ! -f "$FAKE_CALLS.ready" ]; then
       touch "$FAKE_CALLS.ready"; exit 1
     fi
     exit "${FAKE_CHECK_EXIT:-0}"
   fi
   exit 1;;
esac
if [ "$1" = inspect ]; then
 if [ "$2" = odoo-id ]; then printf '%s\n' "$FAKE_ODOO";
 elif [ "$2" = "$FAKE_PROXY_NAME" ]; then printf '%s\n' "$FAKE_PROXY";
 else printf '%s\n' "$FAKE_WORKER"; fi
 exit 0
fi
case "$1" in
 rm|pull|build|create|start) exit 0;;
 network) exit 0;;
esac
exit 1
`

func fixture(t *testing.T) *Runtime {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake Docker uses POSIX shell")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(fakeDocker), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_CALLS", filepath.Join(home, "calls"))
	for _, name := range []string{"FAKE_WORKERS", "FAKE_NAMES", "FAKE_NETWORK_NAMES"} {
		t.Setenv(name, "")
	}
	project := t.TempDir()
	state := &config.State{ProjectName: "test", Branch: "main", ProjectRoot: project, Ports: config.Ports{Odoo: 18069}}
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_ODOO", `[{"ID":"odoo-id","State":{"Running":true},"NetworkSettings":{"Networks":{"legacy_override":{"IPAddress":"172.22.0.7"}}}}]`)
	r, err := resolve(context.Background(), project, true)
	if err != nil {
		t.Fatal(err)
	}
	r.AllowedOrigins = []string{"https://cdn.example.com"}
	sum := sha256.Sum256([]byte(squidConfig(r.OdooIP, r.AllowedOrigins)))
	r.PolicyHash = hex.EncodeToString(sum[:])
	r.BrowserImage = "sha256:" + strings.Repeat("b", 64)
	r.ProxyImage = r.BrowserImage
	for _, dir := range []string{r.Directory, r.ArtifactsDir} {
		if err := privateDir(dir); err != nil {
			t.Fatal(err)
		}
	}
	saveRuntime(t, r)
	t.Setenv("FAKE_PROXY_NAME", r.ProxyContainer)
	proxy := &containerInfo{}
	proxy.State.Running = true
	proxy.State.Pid = 123
	proxy.Config.Image = r.ProxyImage
	proxy.Config.Labels = testLabels(r, "proxy")
	proxy.NetworkSettings.Networks = map[string]struct{ IPAddress string }{r.OdooNetwork: {IPAddress: "172.22.0.8"}, r.BrowserNetwork: {IPAddress: "172.23.0.2"}}
	setJSON(t, "FAKE_PROXY", []*containerInfo{proxy})
	setJSON(t, "FAKE_NETWORK", []any{testNetwork(r, map[string]any{"proxy-id": map[string]string{"Name": r.ProxyContainer}})})
	return r
}

func testLabels(r *Runtime, kind string) map[string]string {
	return map[string]string{label + ".managed": "true", label + ".project": r.Project, label + ".kind": kind, label + ".policy": r.PolicyHash}
}

func testNetwork(r *Runtime, members map[string]any) map[string]any {
	return map[string]any{"Internal": true, "Driver": "bridge", "EnableIPv6": false, "Options": map[string]string{"com.docker.network.bridge.inhibit_ipv4": "true"}, "Labels": testLabels(r, "network"), "Containers": members}
}

func setJSON(t *testing.T, key string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(key, string(data))
}
func saveRuntime(t *testing.T, r *Runtime) {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.Directory, "runtime.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
func calls(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(os.Getenv("FAKE_CALLS"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func clearCalls(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(os.Getenv("FAKE_CALLS"), nil, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadPassiveAndLegacyNetwork(t *testing.T) {
	r := fixture(t)
	clearCalls(t)
	loaded, err := Load(context.Background(), r.Project)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.OdooNetwork != "legacy_override" || loaded.InternalURL != internalURL || loaded.ProxyIP != "172.23.0.2" {
		t.Fatalf("wrong discovery: %+v", loaded)
	}
	for _, line := range strings.Split(calls(t), "\n") {
		if strings.Contains(line, "create") || strings.Contains(line, "start") || strings.Contains(line, "build") || strings.Contains(line, "pull") || strings.HasPrefix(line, "run ") {
			t.Fatalf("Load mutated Docker: %s", line)
		}
	}
	links, _ := config.ProjectLinksDir()
	if _, err := os.Stat(links); !os.IsNotExist(err) {
		t.Fatal("passive lookup repaired project links")
	}
	entries, _ := os.ReadDir(r.ArtifactsDir)
	if len(entries) != 0 {
		t.Fatal("Load created artifacts")
	}
	t.Setenv("FAKE_ODOO", strings.ReplaceAll(os.Getenv("FAKE_ODOO"), "172.22.0.7", "172.22.0.9"))
	if _, err := Load(context.Background(), r.Project); err == nil || !strings.Contains(err.Error(), "prepare") {
		t.Fatalf("changed IP: %v", err)
	}
}

func TestDiscoveryCancelsHungComposeProbe(t *testing.T) {
	r := fixture(t)
	t.Setenv("FAKE_PROBE_DELAY", "true")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := resolve(ctx, r.Project, true); err == nil {
		t.Fatal("cancelled discovery succeeded")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("Compose discovery ignored its context deadline")
	}
}

func TestLoadRejectsPolicyAndTopology(t *testing.T) {
	for _, change := range []string{"policy", "proxy labels", "network labels", "not internal", "wrong driver", "ipv6", "host gateway", "missing isolation option", "extra member", "extra proxy network", "missing proxy IP"} {
		t.Run(change, func(t *testing.T) {
			r := fixture(t)
			switch change {
			case "policy":
				r.AllowedOrigins = append(r.AllowedOrigins, "https://other.example")
				saveRuntime(t, r)
			case "proxy labels":
				t.Setenv("FAKE_PROXY", strings.ReplaceAll(os.Getenv("FAKE_PROXY"), r.PolicyHash, "wrong"))
			case "network labels":
				t.Setenv("FAKE_NETWORK", strings.ReplaceAll(os.Getenv("FAKE_NETWORK"), r.PolicyHash, "wrong"))
			case "not internal":
				t.Setenv("FAKE_NETWORK", strings.ReplaceAll(os.Getenv("FAKE_NETWORK"), `"Internal":true`, `"Internal":false`))
			case "wrong driver":
				t.Setenv("FAKE_NETWORK", strings.ReplaceAll(os.Getenv("FAKE_NETWORK"), `"Driver":"bridge"`, `"Driver":"overlay"`))
			case "ipv6":
				t.Setenv("FAKE_NETWORK", strings.ReplaceAll(os.Getenv("FAKE_NETWORK"), `"EnableIPv6":false`, `"EnableIPv6":true`))
			case "host gateway":
				t.Setenv("FAKE_NETWORK", strings.ReplaceAll(os.Getenv("FAKE_NETWORK"), `"com.docker.network.bridge.inhibit_ipv4":"true"`, `"com.docker.network.bridge.inhibit_ipv4":"false"`))
			case "missing isolation option":
				t.Setenv("FAKE_NETWORK", strings.ReplaceAll(os.Getenv("FAKE_NETWORK"), `"com.docker.network.bridge.inhibit_ipv4":"true"`, ``))
			case "extra member":
				setJSON(t, "FAKE_NETWORK", []any{testNetwork(r, map[string]any{"alien": map[string]string{"Name": "alien"}})})
				t.Setenv("FAKE_WORKER", `[{}]`)
			case "missing proxy IP":
				t.Setenv("FAKE_PROXY", strings.ReplaceAll(os.Getenv("FAKE_PROXY"), "172.23.0.2", ""))
			case "extra proxy network":
				var info []containerInfo
				_ = json.Unmarshal([]byte(os.Getenv("FAKE_PROXY")), &info)
				info[0].NetworkSettings.Networks["extra"] = struct{ IPAddress string }{"1.2.3.4"}
				setJSON(t, "FAKE_PROXY", info)
			}
			if _, err := Load(context.Background(), r.Project); err == nil {
				t.Fatal("accepted changed runtime")
			}
		})
	}
}

func TestCommandIsolationAndCredentials(t *testing.T) {
	r := fixture(t)
	t.Setenv("ODOOCTL_BROWSER_PASSWORD", "stale-secret")
	t.Setenv("PLAYWRIGHT_MCP_PORT", "9999")
	cmd, name, err := r.Command(context.Background(), false, "unique-login", "unique-password")
	if err != nil {
		t.Fatal(err)
	}
	if !workerName.MatchString(name) {
		t.Fatalf("invalid worker %q", name)
	}
	args := strings.Join(cmd.Args, " ")
	for _, secret := range []string{"unique-login", "unique-password", "stale-secret"} {
		if strings.Contains(args, secret) {
			t.Fatalf("secret in command args: %s", secret)
		}
	}
	for _, required := range []string{"--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pull=never", "--log-driver=none", "--shm-size 256m", "--memory 1536m", "--no-webmcp", "--snapshot-boxes", "--proxy-bypass <-loopback>", "--init-page /opt/runtime/init-page.ts", "--executable-path /opt/chromium", "--dns 127.0.0.1", "--add-host proxy:172.23.0.2", "--config /opt/runtime/config.json", "--output-max-size 16777216", "/tmp:rw,nosuid,nodev,size=128m,mode=1777", "/artifacts:rw,nosuid,nodev,size=64m,uid="} {
		if !strings.Contains(args, required) {
			t.Errorf("missing %s", required)
		}
	}
	if strings.Contains(args, "--mount") || strings.Contains(args, "--volume") || strings.Contains(args, r.Project) && strings.Contains(args, "type=bind") || strings.Count(args, "--tmpfs") != 2 || strings.Count(args, "--network") != 1 || strings.Contains(args, "docker.sock") || strings.Contains(args, "--user 0:") || strings.Contains(args, "--port") {
		t.Fatalf("unsafe args: %s", args)
	}
	env := strings.Join(cmd.Env, "\n")
	if !strings.Contains(env, "ODOOCTL_BROWSER_PASSWORD=unique-password") || strings.Contains(env, "stale-secret") || strings.Contains(env, "PLAYWRIGHT_MCP_PORT=") {
		t.Fatal("incorrect secret/config environment")
	}
	entries, _ := os.ReadDir(r.ArtifactsDir)
	if len(entries) != 1 {
		t.Fatal("expected one session directory")
	}
	exportDir, err := r.SessionArtifacts(name)
	if err != nil || exportDir != filepath.Join(r.ArtifactsDir, entries[0].Name()) {
		t.Fatalf("session export directory mismatch: %s %v", exportDir, err)
	}
	if strings.Contains(args, exportDir) {
		t.Fatal("host export directory forwarded to worker")
	}
	data, err := os.ReadFile(filepath.Join(r.Directory, "runtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"unique-login", "unique-password", "stale-secret", "ProxyIP"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("unexpected runtime state data: %s", secret)
		}
	}
	for _, path := range []string{r.Directory, r.ArtifactsDir, filepath.Join(r.ArtifactsDir, entries[0].Name())} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0700 {
			t.Fatalf("permissions for %s: %v %v", path, info, err)
		}
	}
	info, _ := os.Stat(filepath.Join(r.Directory, "runtime.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("runtime state permissions")
	}
	public, _, err := r.Command(context.Background(), true, "ignored-login", "ignored-password")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(public.Env, "\n"), "ODOOCTL_BROWSER_PASSWORD=") || strings.Contains(strings.Join(public.Args, " "), "ODOOCTL_BROWSER_LOGIN") {
		t.Fatal("public mode forwarded credentials")
	}
	for _, line := range strings.Split(calls(t), "\n") {
		if strings.HasPrefix(line, "run ") {
			t.Fatal("Command executed worker instead of returning it")
		}
	}
}

func TestCleanupManagedOnlyAndRetainsArtifacts(t *testing.T) {
	r := fixture(t)
	ctx := context.Background()
	sentinel := filepath.Join(r.ArtifactsDir, "retain")
	if err := os.WriteFile(sentinel, []byte("retain"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_NAMES", r.ProxyContainer+"\nodoo\npostgres\nother-proxy")
	t.Setenv("FAKE_NETWORK_NAMES", r.BrowserNetwork+"\nlegacy_override\nother-network")
	t.Setenv("FAKE_WORKERS", "worker-id")
	clearCalls(t)
	if err := Stop(ctx, r.Project); err == nil {
		t.Fatal("removed proxy with active worker")
	}
	if strings.Contains(calls(t), "rm ") {
		t.Fatal("cleanup mutated active runtime")
	}
	if _, err := Prepare(ctx, r.Project, []string{"https://new.example"}); err == nil {
		t.Fatal("prepared policy while worker exists")
	}
	t.Setenv("FAKE_WORKERS", "")
	t.Setenv("FAKE_ODOO", `[]`)
	clearCalls(t)
	if err := Stop(ctx, r.Project); err != nil {
		t.Fatal(err)
	}
	log := calls(t)
	if !strings.Contains(log, "rm -f "+r.ProxyContainer) || !strings.Contains(log, "network rm "+r.BrowserNetwork) {
		t.Fatalf("missing managed cleanup: %s", log)
	}
	for _, bad := range []string{"rm -f odoo\n", "rm -f postgres", "network rm legacy_override", "compose", "volume"} {
		if strings.Contains(log, bad) {
			t.Fatalf("unrelated cleanup: %s", log)
		}
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatal("artifacts deleted")
	}
	t.Setenv("FAKE_PROXY", strings.ReplaceAll(os.Getenv("FAKE_PROXY"), r.Project, "/different/project"))
	clearCalls(t)
	if err := Stop(ctx, r.Project); err == nil {
		t.Fatal("removed other project's proxy")
	}
	if strings.Contains(calls(t), "rm ") {
		t.Fatal("unmanaged cleanup executed")
	}
}

func TestStopWorkerValidatesNameAndLabels(t *testing.T) {
	r := fixture(t)
	ctx := context.Background()
	name := r.BrowserNetwork + "-worker-" + strings.Repeat("a", 24)
	if err := r.StopWorker(ctx, "odoo"); err == nil {
		t.Fatal("accepted arbitrary container")
	}
	info := &containerInfo{}
	info.Config.Labels = testLabels(r, "worker")
	info.Config.Labels[label+".project"] = "other"
	setJSON(t, "FAKE_WORKER", []*containerInfo{info})
	clearCalls(t)
	if err := r.StopWorker(ctx, name); err == nil {
		t.Fatal("accepted wrong project")
	}
	if strings.Contains(calls(t), "rm ") {
		t.Fatal("removed wrong worker")
	}
	info.Config.Labels = testLabels(r, "worker")
	setJSON(t, "FAKE_WORKER", []*containerInfo{info})
	if err := r.StopWorker(ctx, name); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(calls(t), "rm -f "+name) {
		t.Fatal("worker was not removed")
	}
}

func TestCommandUsesValidatedMetadata(t *testing.T) {
	r := fixture(t)
	artifacts := r.ArtifactsDir
	r.ArtifactsDir = r.Project
	r.OdooNetwork = "untrusted-network"
	r.ProxyContainer = "untrusted-proxy"
	r.ProxyIP = "127.0.0.1"
	cmd, _, err := r.Command(context.Background(), true, "", "")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(cmd.Args, " ")
	if strings.Contains(args, "--mount") || strings.Contains(args, "untrusted-") || !strings.Contains(args, "--add-host proxy:172.23.0.2") {
		t.Fatal("caller-modified metadata used as Docker authority")
	}
	entries, err := os.ReadDir(artifacts)
	if err != nil || len(entries) != 1 {
		t.Fatal("validated host export directory not used")
	}
}

func TestEmbeddedSourcesPinned(t *testing.T) {
	data, _ := files.ReadFile("files/browser.Dockerfile")
	text := string(data)
	for _, pin := range []string{"@playwright/mcp@0.0.83", "playwright@1.64.0-alpha-1790635538000", "playwright-core@1.64.0-alpha-1790635538000", "playwright install --with-deps chromium"} {
		if !strings.Contains(text, pin) {
			t.Fatalf("missing pin %s", pin)
		}
	}
	if strings.Contains(text, "@latest") {
		t.Fatal("unpinned backend")
	}
	if strings.Index(text, "node /opt/runtime/patch-snapshots.js") > strings.Index(text, "playwright install --with-deps") || !strings.Contains(text, "COPY patch-snapshots.js /opt/runtime/") || !strings.Contains(text, "COPY init-page.ts init-script.js config.json /opt/runtime/") {
		t.Fatal("snapshot patch/config not installed before browser setup")
	}
	if strings.Index(text, "COPY init-page.ts") < strings.Index(text, "playwright install --with-deps") {
		t.Fatal("init-page changes invalidate dependency/browser installation")
	}
	data, _ = files.ReadFile("files/config.json")
	var cfg struct {
		Browser struct {
			ContextOptions struct{ AcceptDownloads *bool }
			LaunchOptions  struct{ Args []string }
		}
	}
	if json.Unmarshal(data, &cfg) != nil || cfg.Browser.ContextOptions.AcceptDownloads == nil || *cfg.Browser.ContextOptions.AcceptDownloads {
		t.Fatal("downloads are not disabled")
	}
	if !reflect.DeepEqual(cfg.Browser.LaunchOptions.Args, []string{"--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader"}) {
		t.Fatal("software renderer configuration mismatch")
	}
	data, _ = files.ReadFile("files/init-page.ts")
	text = string(data)
	for _, required := range []string{"routeWebSocket", "socket.close()", "process.env.ODOOCTL_BROWSER_PASSWORD", "result?.uid", "Odoo browser login failed", "maxRedirects: 0", "const base = 'http://odoo:8069'", "target.origin !== base", "html.length > 1048576", "response.dispose()"} {
		if !strings.Contains(text, required) {
			t.Errorf("missing init behavior %s", required)
		}
	}
	if strings.Contains(text, "console.") || strings.Contains(text, "throw error") || strings.Contains(text, ".fill(") || strings.Contains(text, ".click(") || strings.Contains(text, "process.env.ODOOCTL_BROWSER_URL") {
		t.Fatal("unsafe initialization logging")
	}
}

func TestPrepareWritesPrivatePinnedRuntime(t *testing.T) {
	r := fixture(t)
	build := filepath.Join(r.Directory, "build")
	if err := os.MkdirAll(build, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(build, "squid.conf"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare(context.Background(), r.Project, r.AllowedOrigins)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{r.Directory, r.ArtifactsDir, build} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0700 {
			t.Fatalf("directory permissions: %s %v", path, err)
		}
	}
	for _, name := range []string{"squid.conf", "browser.Dockerfile", "proxy.Dockerfile", "init-page.ts", "init-script.js", "config.json", "patch-snapshots.js"} {
		info, err := os.Stat(filepath.Join(build, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("file permissions: %s %v", name, err)
		}
	}
	if len(prepared.SourceImages) != 2 {
		t.Fatal("source image digests not recorded")
	}
	for _, digest := range prepared.SourceImages {
		if !strings.Contains(digest, "@sha256:") {
			t.Fatal("unpinned source")
		}
	}
	log := calls(t)
	for _, required := range []string{"network create --driver bridge --internal --opt com.docker.network.bridge.inhibit_ipv4=true --ipv6=false", "create --init --name " + r.ProxyContainer, "--add-host odoo:172.22.0.7", "network connect --alias proxy", "BASE=base@sha256:", "squid -k parse -f /etc/squid/squid.conf", "squid -k check -f /etc/squid/squid.conf"} {
		if !strings.Contains(log, required) {
			t.Errorf("missing setup %s", required)
		}
	}
	if strings.Contains(log, "--publish") || strings.Contains(log, "docker.sock") || strings.Contains(log, "compose up") {
		t.Fatal("unsafe setup")
	}
}

func TestProxyStartupValidation(t *testing.T) {
	for _, scenario := range []string{"parse failure", "daemon exited", "daemon pending", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			r := fixture(t)
			ctx := context.Background()
			switch scenario {
			case "parse failure":
				t.Setenv("FAKE_PARSE_EXIT", "1")
			case "daemon exited":
				var info []containerInfo
				_ = json.Unmarshal([]byte(os.Getenv("FAKE_PROXY")), &info)
				info[0].State.Running = false
				info[0].State.Pid = 0
				info[0].State.ExitCode = 1
				setJSON(t, "FAKE_PROXY", info)
			case "daemon pending":
				t.Setenv("FAKE_CHECK_PENDING", "true")
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := r.waitProxy(ctx)
			if scenario == "daemon pending" {
				if err != nil {
					t.Fatal(err)
				}
				if strings.Count(calls(t), "squid -k check") != 2 {
					t.Fatal("did not wait for daemon readiness")
				}
			} else if err == nil {
				t.Fatal("accepted unhealthy proxy")
			}
		})
	}
}

func TestPrepareDoesNotSaveInvalidProxy(t *testing.T) {
	r := fixture(t)
	state := filepath.Join(r.Directory, "runtime.json")
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_PARSE_EXIT", "1")
	if _, err := Prepare(context.Background(), r.Project, r.AllowedOrigins); err == nil {
		t.Fatal("prepared invalid proxy")
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("published failed proxy state")
	}
}

func TestInitPageAuthenticationHarness(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable; static authentication guard tests still run")
	}
	source, err := files.ReadFile("files/init-page.ts")
	if err != nil {
		t.Fatal(err)
	}
	harness, err := os.ReadFile(filepath.Join("testdata", "init-page.test.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "--input-type=module")
	cmd.WaitDelay = time.Second
	cmd.Stdin = strings.NewReader(strings.Replace(string(source), "export default", "const init =", 1) + "\n" + string(harness))
	if output, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			t.Fatal("authentication harness exceeded its independent deadline")
		}
		t.Fatalf("authentication harness: %v\n%s", err, output)
	}
}

func TestSessionArtifactsValidatesWorker(t *testing.T) {
	r := fixture(t)
	name := r.BrowserNetwork + "-worker-" + strings.Repeat("a", 24)
	want := filepath.Join(r.ArtifactsDir, "session-"+strings.Repeat("a", 24))
	got, err := r.SessionArtifacts(name)
	if err != nil || got != want {
		t.Fatalf("SessionArtifacts: %s %v", got, err)
	}
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Fatal("SessionArtifacts unexpectedly created directory")
	}
	for _, bad := range []string{"odoo", name + "/../escape", name + "/extra", r.BrowserNetwork + "-worker-" + strings.Repeat("a", 23), "odooctl-browser-" + strings.Repeat("f", 16) + "-worker-" + strings.Repeat("a", 24)} {
		if _, err := r.SessionArtifacts(bad); err == nil {
			t.Fatalf("accepted invalid worker %q", bad)
		}
	}
}

func TestProxyIPIsLiveMetadata(t *testing.T) {
	r := fixture(t)
	r.ProxyIP = "127.0.0.1"
	saveRuntime(t, r)
	// Even a manually inserted state value is ignored in favor of Docker.
	file := filepath.Join(r.Directory, "runtime.json")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	data = append([]byte(`{"ProxyIP":"127.0.0.1",`), data[1:]...)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_PROXY", strings.ReplaceAll(os.Getenv("FAKE_PROXY"), "172.23.0.2", "172.23.0.9"))
	loaded, err := Load(context.Background(), r.Project)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ProxyIP != "172.23.0.9" {
		t.Fatalf("proxy IP not discovered from Docker: %s", loaded.ProxyIP)
	}
	cmd, _, err := r.Command(context.Background(), true, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(cmd.Args, " "), "--add-host proxy:172.23.0.9") {
		t.Fatal("worker used stale proxy IP")
	}
}

func TestSnapshotProducerPatch(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable; static build guards still run")
	}
	source, err := files.ReadFile("files/patch-snapshots.js")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "patch-snapshots.cjs")
	if err := os.WriteFile(file, source, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, filepath.Join("testdata", "patch-snapshots.test.cjs"), file)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("snapshot patch harness: %v\n%s", err, out)
	}
	// CLI must fail closed rather than silently succeeding without the pin.
	cmd = exec.CommandContext(ctx, node, file)
	if out, err := cmd.CombinedOutput(); err == nil || string(out) != "Pinned Playwright snapshot patch validation failed\n" {
		t.Fatalf("patch did not fail closed: %v %s", err, out)
	}
}

// Opt-in only: uses an already prepared runtime, with no browser/Odoo commands.
func TestLiveSquidUnresolvedDestination(t *testing.T) {
	project := os.Getenv("ODOOCTL_PLAYWRIGHT_TEST_PROJECT")
	if project == "" {
		t.Skip("live browser runtime project not selected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := Load(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	// A separate loopback-only listener exercises NXDOMAIN as an approved name.
	// Neither the prepared image/config nor its externally reachable policy changes.
	policy := squidConfig(r.OdooIP, []string{"https://odooctl-missing-destination.invalid"})
	policy = strings.Replace(policy, "http_port 3128", "http_port 127.0.0.1:3129", 1)
	policy = strings.Replace(policy, "pid_filename /tmp/squid.pid", "pid_filename /tmp/odooctl-dns-probe.pid", 1)
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", r.ProxyContainer, "squid", "-N", "-f", "/dev/stdin")
	cmd.Stdin = strings.NewReader(policy)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		stop := exec.CommandContext(cleanupCtx, "docker", "exec", "-i", r.ProxyContainer, "squid", "-k", "shutdown", "-f", "/dev/stdin")
		stop.Stdin = strings.NewReader(policy)
		if err := stop.Run(); err != nil {
			t.Errorf("temporary Squid shutdown failed: %v", err)
		}
		if err := cmd.Wait(); err != nil {
			t.Errorf("temporary Squid did not exit cleanly: %v", err)
		}
	}()
	// Read and report only the HTTP status line, never headers or response bodies.
	probe := `exec 3<>/dev/tcp/127.0.0.1/3129; printf 'CONNECT odooctl-missing-destination.invalid:443 HTTP/1.1\r\nHost: odooctl-missing-destination.invalid:443\r\nConnection: close\r\n\r\n' >&3; IFS= read -r line <&3; printf '%s\n' "$line"`
	for {
		out, err := exec.CommandContext(ctx, "docker", "exec", r.ProxyContainer, "bash", "-e", "-c", probe).Output()
		if err == nil {
			status := strings.Fields(string(out))
			if len(status) < 2 || status[1] != "403" {
				t.Fatalf("unresolved approved destination was not denied by dst guard: %q", strings.TrimSpace(string(out)))
			}
			t.Logf("approved NXDOMAIN: %s", strings.TrimSpace(string(out)))
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("temporary Squid probe timed out")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func stopLiveWorker(t *testing.T, r *Runtime, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stopErr := r.StopWorker(ctx, name)
	// --rm normally removed it already. Verify absence even when StopWorker fails.
	out, err := docker(ctx, "ps", "-aq", "--filter", "name=^/"+name+"$")
	if err != nil || len(strings.TrimSpace(string(out))) != 0 {
		t.Errorf("owned live worker cleanup failed: stop=%v verification=%v", stopErr, err)
	}
	dir, err := r.SessionArtifacts(name)
	if err != nil {
		t.Error(err)
	} else if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
		t.Errorf("remove empty probe session directory: %v", err)
	}
}

func TestLiveWorkerSecurityProbes(t *testing.T) {
	project := os.Getenv("ODOOCTL_PLAYWRIGHT_TEST_PROJECT")
	if project == "" {
		t.Skip("live browser runtime project not selected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	r, err := Load(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"https://cdn.jsdelivr.net", "https://127.0.0.1.nip.io"} {
		if !slices.Contains(r.AllowedOrigins, origin) {
			t.Fatal("security probe requires prepared CDN and loopback DNS origins; policy left unchanged")
		}
	}
	out, err := docker(ctx, "network", "inspect", r.BrowserNetwork)
	if err != nil {
		t.Fatal(err)
	}
	var networks []struct {
		IPAM struct{ Config []struct{ Gateway string } }
	}
	if json.Unmarshal(out, &networks) != nil || len(networks) != 1 {
		t.Fatal("invalid network gateway inspection")
	}
	gateways := []string{}
	for _, entry := range networks[0].IPAM.Config {
		if entry.Gateway != "" {
			gateways = append(gateways, entry.Gateway)
		}
	}
	t.Logf("internal network validated; inhibit_ipv4=true; no_assigned_gateway=%t", len(gateways) == 0)
	// A 403 alone could also mean NXDOMAIN. Confirm the approved name currently
	// resolves to loopback on the proxy before attributing denial to the dst ACL.
	dnsCtx, dnsCancel := context.WithTimeout(ctx, 5*time.Second)
	out, err = docker(dnsCtx, "exec", r.ProxyContainer, "getent", "ahostsv4", "127.0.0.1.nip.io")
	dnsCancel()
	if err != nil || !strings.HasPrefix(string(out), "127.0.0.1 ") {
		t.Fatal("approved private DNS fixture did not resolve to loopback (details withheld)")
	}
	cmd, name, err := r.Command(ctx, true, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer stopLiveWorker(t, r, name)
	input, err := json.Marshal(map[string]any{"odooIP": r.OdooIP, "proxyIP": r.ProxyIP, "gateways": gateways})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := os.ReadFile(filepath.Join("testdata", "live-security-probes.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	replaced := false
	for i, arg := range cmd.Args {
		if arg == r.BrowserImage {
			cmd.Args = append(cmd.Args[:i], "--entrypoint", "node", r.BrowserImage, "--input-type=module")
			replaced = true
			break
		}
	}
	if !replaced {
		t.Fatal("worker image argument missing")
	}
	cmd.WaitDelay = time.Second
	cmd.Stdin = strings.NewReader("const fixture = " + string(input) + ";\n" + string(harness))
	out, runErr := cmd.Output()
	if len(out) > 2048 {
		t.Fatal("security probe metadata exceeded limit (withheld)")
	}
	var results []struct {
		Probe     string `json:"probe"`
		Passed    bool   `json:"passed"`
		Status    int    `json:"status"`
		ByteCount int    `json:"byteCount"`
	}
	if json.Unmarshal(out, &results) != nil || len(results) != 19+len(gateways) {
		t.Fatal("invalid security probe metadata (details withheld)")
	}
	for _, result := range results {
		t.Logf("probe=%s passed=%t status=%d byte_count=%d", result.Probe, result.Passed, result.Status, result.ByteCount)
		if !result.Passed {
			t.Errorf("security probe failed: %s", result.Probe)
		}
	}
	if runErr != nil {
		t.Fatal("security harness failed or exceeded deadline (details withheld)")
	}
}

func TestLiveWorkerCLIEOF(t *testing.T) {
	project := os.Getenv("ODOOCTL_PLAYWRIGHT_TEST_PROJECT")
	if project == "" {
		t.Skip("live browser runtime project not selected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	r, err := Load(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	cmd, name, err := r.Command(ctx, true, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer stopLiveWorker(t, r, name)
	cmd.WaitDelay = time.Second
	cmd.Stdin = strings.NewReader("")
	if err := cmd.Run(); err != nil {
		t.Fatal("public MCP CLI did not exit cleanly on stdin EOF (details withheld)")
	}
	t.Log("public MCP CLI exited cleanly on stdin EOF")
}

func TestLiveInitPage(t *testing.T) {
	project := os.Getenv("ODOOCTL_PLAYWRIGHT_TEST_PROJECT")
	if project == "" {
		t.Skip("live browser runtime project not selected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	r, err := Load(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	cmd, name, err := r.Command(ctx, false, os.Getenv("ODOOCTL_MCP_BROWSER_LOGIN"), os.Getenv("ODOOCTL_MCP_BROWSER_PASSWORD"))
	if err != nil {
		t.Fatal(err)
	}
	defer stopLiveWorker(t, r, name)
	for i, arg := range cmd.Args {
		if arg == r.BrowserImage {
			cmd.Args = append(cmd.Args[:i], "--entrypoint", "node", r.BrowserImage, "--input-type=module")
			break
		}
	}
	source, err := files.ReadFile("files/init-page.ts")
	if err != nil {
		t.Fatal(err)
	}
	harness, err := os.ReadFile(filepath.Join("testdata", "live-init-page.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdin = strings.NewReader(strings.Replace(string(source), "export default", "const init =", 1) + "\n" + string(harness))
	// The trusted harness emits allowlisted scalar metadata only; stderr is withheld.
	out, err := cmd.Output()
	t.Logf("auth stage metadata:\n%s", out)
	if err != nil {
		t.Fatal("fixed init-page authentication failed (details withheld)")
	}
}
