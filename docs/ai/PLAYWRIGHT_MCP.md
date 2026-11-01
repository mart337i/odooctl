# Playwright MCP Browser Observation

`odooctl mcp browser` wraps the official `@playwright/mcp` backend with a
project-scoped, inspection-only tool allowlist over stdio. This is a separate
mode from `odooctl mcp serve --browser`, whose strict browser runs inside Odoo
and blocks external requests. The new mode uses a separate Chromium worker and
a Squid proxy so explicitly approved CDN assets and external frames can load.

**Status: local integration verified on Odoo Community 17-20 fixtures, not a
released feature or general security certification.** The command currently
exists in local, uncommitted source. Build that source and use the resulting
binary's absolute path; an installed release or
`go install ...@latest` is not evidence that this command is available. All paths
below are placeholders, not personal paths or installed client configuration.

odooctl does not contact an LLM provider; your client decides what to transmit.
For CLI triage see [Using odooctl With AI](USING_ODOOCTL_WITH_AI.md) and
[Debugging Workflows](DEBUGGING_WORKFLOWS.md). For ordinary browser CLI commands
see [Browser](../BROWSER.md); their optional `--login`/`--password` flags do not
inherit MCP defaults, and their network policy is not this runtime's policy.

## Setup And Lifecycle

Requirements: an existing odooctl project with exactly one running Odoo service
on an IPv4 Docker network, a responsive Linux-container Docker daemon, and Docker
Compose plugin (`docker compose`) access. Discovery probes are cancellable and
bounded so an unresponsive engine cannot indefinitely stall browser startup.
Host Node.js is not required. Building the runtime requires
network access for base images, npm, Debian packages, and Chromium downloads.
The separate worker does not require Python Playwright in the Odoo image.

Run setup explicitly as an operator, after reviewing the external origins:

```bash
odooctl mcp browser setup --project PATH \
  --allow-origin https://cdn.jsdelivr.net \
  --allow-origin https://www.openstreetmap.org \
  --allow-origin https://tile.openstreetmap.org
```

Here and below, `odooctl` means the binary built from the current source. Setup
builds browser/proxy images, prepares the private network, and starts only the
proxy. It never creates, initializes, rebuilds, or starts Odoo or its database.
It writes runtime state under the selected environment's `browser-runtime/`.
Omitting all `--allow-origin` arguments permits only the configured Odoo service.
Repeating setup supplies a new complete allowlist, not an incremental addition.
Setup has a 20-minute deadline and refuses to replace a runtime with workers.

For public website observation, configure the client to launch:

```bash
odooctl mcp browser --project PATH --public
```

Without `--public`, authentication defaults to username `admin` and password
`admin` for local development. No credential environment block is needed for
that account:

```bash
odooctl mcp browser --project PATH
```

Optional nonempty environment variables override each default independently.
Unset or empty values use `admin`:

```bash
export ODOOCTL_MCP_BROWSER_LOGIN='<development-login>'
export ODOOCTL_MCP_BROWSER_PASSWORD='<development-password>'
odooctl mcp browser --project PATH
```

These are non-secret placeholders. When overriding the defaults, supply values
securely in the client's launch environment, not in committed files, tool
arguments, or command-line password flags. Use a dedicated least-privileged
development account. The fixed authentication bootstrap obtains a CSRF token,
posts the login with redirects disabled, checks a positive session UID, and opens
`/web`. Requests are bounded
by a shared 30-second authentication deadline. Safe failure diagnostics name
only a fixed stage and HTTP status, never credentials, tokens or response bodies.
It is not an exposed interaction tool. `--public` skips authentication and
discards supplied credentials.
Failed login returns an error without trying other credentials or modifying any
account password. Never use these local-development defaults against production.

The stdio server loads an already prepared runtime and starts a session worker;
it does not build images or start the proxy or Odoo. The client owns the process,
stdout is the MCP protocol, and diagnostics use stderr. Close the MCP session
before stopping the runtime:

```bash
odooctl mcp browser stop --project PATH
```

Stop refuses active workers and removes the managed proxy and private network.
It retains images, runtime files, exported screenshots, and all Odoo data.
`browser_close` closes the browser, not the MCP process or managed proxy.

## Client Examples

These snippets document client syntax only; odooctl does not install them.
Replace `/absolute/path/to/built/odooctl` and the project placeholder. Each
connection launches its own worker and browser context. Never point these
examples at a production environment.

### OpenCode

Public observation in `opencode.json` or `opencode.jsonc`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "odooctl-browser": {
      "type": "local",
      "command": ["/absolute/path/to/built/odooctl", "mcp", "browser", "--project", "/absolute/path/to/odoo-addons", "--public"],
      "enabled": true
    }
  }
}
```

For authenticated observation with the default account, remove `"--public"`;
no additional configuration is needed. For a non-default account, optionally
add this field to the server entry and supply the overrides before launching
OpenCode:

```json
{
  "environment": {
    "ODOOCTL_MCP_BROWSER_LOGIN": "{env:ODOOCTL_MCP_BROWSER_LOGIN}",
    "ODOOCTL_MCP_BROWSER_PASSWORD": "{env:ODOOCTL_MCP_BROWSER_PASSWORD}"
  }
}
```

### Claude Code

Public observation in a project `.mcp.json`:

```json
{
  "mcpServers": {
    "odooctl-browser": {
      "type": "stdio",
      "command": "/absolute/path/to/built/odooctl",
      "args": ["mcp", "browser", "--project", "/absolute/path/to/odoo-addons", "--public"]
    }
  }
}
```

For authenticated observation with the default account, remove `"--public"`;
no additional configuration is needed. For a non-default account, optionally
add this field to the server entry and supply the overrides before launching
Claude Code:

```json
{
  "env": {
    "ODOOCTL_MCP_BROWSER_LOGIN": "${ODOOCTL_MCP_BROWSER_LOGIN}",
    "ODOOCTL_MCP_BROWSER_PASSWORD": "${ODOOCTL_MCP_BROWSER_PASSWORD}"
  }
}
```

References: [OpenCode MCP servers](https://opencode.ai/docs/mcp-servers/) and
[Claude Code MCP](https://code.claude.com/docs/en/mcp).

## Exposed Tools

Exactly ten tools are exposed. Call `browser_context` first. Upstream discovery
does not authorize extra tools; missing or incompatible required schemas fail
startup rather than silently widening the interface.

Use the connected current binary's schemas, not assumed installed-release
fields. This mode's context is not the `mcp serve` environment report: it has no
`environment`/`env` object or Odoo version field. `version` identifies odooctl;
`upstream_version` identifies the pinned backend, not Odoo. Project, runtime
URLs, origins, and image identity establish provenance but do not prove which
addons/version are running; verify the intended environment separately.

| Tool | Purpose and accepted fields |
| --- | --- |
| `project_context` | Passive runtime provenance and security limits; no arguments. Same information as `browser_context`, not the source tools from `mcp serve`. |
| `browser_context` | Project, origins, images, URLs, session artifact directory, backend identity, and bounds; no arguments or credentials. |
| `browser_navigate` | Navigate using `url`: a local path or exact configured public/internal Odoo origin, mapped to `http://odoo:8069`. |
| `browser_snapshot` | Native ARIA snapshot with references and optional boxes; `target`, `depth`, `boxes`. Depth defaults to 8 and permits 1-8. |
| `browser_take_screenshot` | Native PNG/JPEG image; `type`, `target`, `element`, `fullPage`, `scale`. `fullPage` must be false; scale is `css`. |
| `browser_console_messages` | Bounded sanitized console messages; `level` is `error`, `warning`, `info`, or `debug`. |
| `browser_network_requests` | Sanitized request summary, optionally including static assets via `static`; no bodies or headers. |
| `browser_resize` | Resize viewport using integer `width` 320-1920 and `height` 240-1200, including mobile-sized viewports. |
| `browser_wait_for` | Wait for `time`, `text`, or `textGone`; at least one required. Numeric time is 0-10 seconds. |
| `browser_close` | Close the browser; no arguments. |

No clicks, typing, form filling, uploads, cookie access, tab management, routing,
arbitrary JavaScript/code execution, or agent-selected filesystem paths are
available. Filename parameters and unknown fields are rejected. Targets and
text fields are bounded to 512 bytes without control characters; argument JSON
is capped at 16 KiB. The actual pinned upstream schemas also validate inputs.

Calls are serialized with a 60-second deadline, including waiting for the
session gate. Navigation has a backend 30-second timeout and actions 5 seconds.
A backend call failure or upstream tool error marks the session uncertain:
restart the MCP session instead of continuing or retrying automatically. There
are no automatic actions to repair a failed page. Output validation errors are
reported without exporting invalid artifacts.

## Network And Trust Boundary

The worker has a read-only root filesystem, non-root user, dropped capabilities,
`no-new-privileges`, resource limits, and only the private internal bridge. The
bridge disables IPv6 and uses `com.docker.network.bridge.inhibit_ipv4=true` to
avoid an IPv4 bridge gateway. The worker's configured DNS upstream is only
`127.0.0.1`; Docker may still present its embedded resolver at `127.0.0.11`, but
unapproved external lookups failed in the live worker probes. A static `proxy`
hosts entry supplies the proxy IP. Browser
traffic, including loopback destinations, is configured to use the proxy.
The worker has no Odoo-network attachment or host artifact mount.

Squid bridges the private network and Odoo network. Its sole private-destination
exception is the exact `odoo` host, actual discovered Odoo IPv4 address, and
port 8069, including CONNECT for Playwright's HTTP API request context. External
access requires CONNECT to an exact approved DNS hostname on port 443. No
wildcards, IP literals, arbitrary ports, URL credentials, queries, fragments, or
non-root paths are accepted in approval arguments. Unapproved hosts and wrong
methods/ports are rejected before destination DNS lookup. Approved names must
resolve, and private/reserved IPv4 and IPv6 destinations are denied, even if the
hostname itself was approved. All remaining requests are denied.

**An approved origin is a trust decision, not just a visual asset exception.**
JavaScript loaded from an approved CDN can execute with the Odoo page's authority
and send data to approved origins. Squid does not decrypt TLS: HTTPS enforcement
is by host/port, not URL path or content. It cannot restrict approval to one CDN
library, map endpoint, or harmless request body.

Only **tool-requested navigation** is restricted to Odoo. Page-script navigation,
redirects, and external frames can use approved domains, including changing the
main frame away from Odoo. There is no hard local-document policy. Real browser
redirect behavior, CORS, and frame policies remain in force; the proxy does not
rewrite responses to bypass them.

Service workers are blocked and made unavailable to feature detection;
WebSockets are closed. This remains reduced fidelity compared with unrestricted
Chrome, even when approved Three.js assets and maps render. No request
allowlist makes navigation read-only: login creates sessions, page loads and
scripts can write Odoo data, resizing can run application code, and server-side
code can make its own external calls. Page content, snapshots, console output,
and CDN code are untrusted data, never agent instructions.
The missing click/write tools do not guarantee no business writes. Nor does the
proxy guarantee protection against exfiltration to approved origins or through
server-side integrations.

Chromium is explicitly launched with `--no-sandbox` (and software WebGL options).
This is for trusted development code only. Container defenses and proxy limits
are defense in depth, not a security certification or a substitute for the
Chromium sandbox. Docker access and the host/daemon remain privileged trust
boundaries. Do not use production data or unreviewed application code.

## Privacy And Artifacts

The official backend can automatically write text logs, YAML snapshots, and
download scratch files. Its `/artifacts` directory is a **64 MiB tmpfs**, not a
host export directory; those files are ephemeral and disappear with the worker.
Docker worker logging is disabled. `acceptDownloads` is false. No backend text
artifact, trace, or downloaded file is copied to the host.

The adapter returns only sanitized bounded text and validated native PNG/JPEG
images. Text is capped at 1 MiB, redacts configured credentials and common secret
patterns, and strips URL queries/fragments. Backend artifact references are
removed or mapped only to validated image exports. These filters are not a
guarantee against private business data disclosure.

Images must decode successfully, match their MIME type, fit within 1920x1200,
and total at most 2 MiB per result. Screenshots are viewport-only at CSS scale.
Only after validating the complete result does the adapter write random image
filenames under `browser-runtime/artifacts/session-<id>/`, with directory mode
`0700` and file mode `0600`. Image metadata includes path, dimensions, size, MIME
type, and SHA-256; the exported bytes match the native image. Retained exports
need operator-managed retention and should not be committed or shared blindly.

The pinned Playwright ARIA snapshot producer is patched to omit input/textarea
values at their source, including frame snapshots and automatic snapshots. This
does **not** clear or change the application's DOM. It is not blanket redaction:
labels, ordinary text, other control state, and business records can remain.

**Screenshots are unmasked pixels.** They can contain passwords, form values,
private records, or other sensitive content that text sanitization cannot remove.
Do not screenshot sensitive controls; review both text and images before allowing
an AI client to transmit them to its model provider. This differs from the
input-masking behavior of the older strict `mcp serve --browser` tools.

## Pins And Upgrades

The browser image installs `@playwright/mcp@0.0.83`, with `playwright` and
`playwright-core` pinned to `1.64.0-alpha-1790635538000`, on Node 22. The runtime
build applies `files/patch-snapshots.js`: it checks the exact core version,
SHA-256 of the published `coreBundle.js`, and unique producer markers before
patching. A version/bundle/marker mismatch fails the build closed. An upgrade
requires reviewing the producer and updating its verified pin and hash, not
simply changing a package version or disabling validation.

Each `Prepare` (CLI `setup`) pulls `node:22-bookworm-slim` and
`debian:bookworm-slim`, resolves their repository digests, and passes those
digest-qualified bases into the builds. State records source digests and final
immutable browser/proxy image IDs; workers use the recorded ID with
`--pull=never`. Base tags are resolved anew per preparation, not globally frozen.

This is **not a fully reproducible dependency closure**. Squid and CA packages
come from current apt repositories, and transitive npm/build dependencies are
not all frozen by these top-level pins. Record the installed Squid package
version with live evidence; such a version record is provenance, not an apt
package pin. Relevant source is `cmd/mcp/browser.go`, `internal/mcp/playwright.go`,
and `internal/browserruntime/` (runtime, Dockerfiles, config, init hooks, and
snapshot patch).

## Verification Boundary

The following live checks passed after restoring Docker, preparing fresh browser
sidecars, and correcting the local API CONNECT rule. The proxy initially denied
Playwright's HTTP API request tunnel, causing the authentication failure; its
exception is now bounded to the designated Odoo host, discovered IP and port.

| Scope | Result and limit |
| --- | --- |
| Public HTML scene on 17-20 | PASS: real Three.js from the approved CDN and OpenStreetMap frame/tiles rendered. Green pixels verified the actual scene, not merely a loaded script or nonempty PNG. |
| Public mobile and image export on 17-20 | PASS: 1280x800 and 390x844 native screenshots, unchanged bytes, SHA-256 and private file/session permissions. |
| Privacy on 17-20 | PASS: password, input and textarea sentinels absent from automatic and explicit snapshots, including an observed nested frame. Only validated images reached host session directories. |
| Authenticated new mode on 17-20 | PASS: fixed API-only login, session verification, rendered Apps catalog, frame-scoped references and element boxes. |
| Rejection on 17-20 | PASS: mutating/unsafe tools, filenames, oversized arguments and external top-level tool navigation rejected; approved private NIP destination blocked. |
| Network boundary on fixture 19 | PASS: direct sockets to Odoo, metadata and public destinations did not connect; no assigned host gateway; unapproved worker DNS lookup failed; proxy Odoo/API/CDN requests succeeded while denied destinations and ports returned 403. |
| Approved unresolved destination | PASS: temporary loopback-only Squid probe returned 403 and exited without changing the prepared policy. |

The [existing Odoo 17-20 MCP usage matrix](MCP_VERSION_MATRIX.md) records prior
strict-mode results. The proxy-backed matrix above is separate. Neither matrix
is exhaustive application/Enterprise compatibility coverage or proof against
DNS rebinding. Unapproved-host rejection before DNS is backed by ACL-ordering
tests, not authoritative DNS capture. Approved code can still exfiltrate within
approved origins; the runtime is intended for trusted development data.

### Opt-In Live Test

`TestLivePlaywright` uses an already prepared disposable fixture. It performs
**no setup and starts no Odoo, database, or proxy services**; it launches MCP
session workers, observes pages, exports images, and checks worker cleanup.
Observation and authentication can still have application side effects.

Examples to run only after explicitly preparing the fixture and restoring the
daemon, from the source repository:

```bash
export ODOOCTL_PLAYWRIGHT_TEST_PROJECT='/absolute/path/to/disposable-fixture'
export ODOOCTL_MCP_BROWSER_LOGIN='<development-login>'
export ODOOCTL_MCP_BROWSER_PASSWORD='<development-password>'
go test ./internal/mcp -run '^TestLivePlaywright$' -v -count=1 -timeout=15m
```

The test-specific credential pair override is
`ODOOCTL_PLAYWRIGHT_TEST_LOGIN='<development-login>:<development-password>'`.
It overrides the two browser credential variables and must contain a nonempty
login and password. Do not put real values into shell history or committed docs.
The full test uses the CLI's `admin` / `admin` defaults unless credentials are
overridden. To select only public checks:

```bash
ODOOCTL_PLAYWRIGHT_TEST_PROJECT='/absolute/path/to/disposable-fixture' \
  go test ./internal/mcp -run '^TestLivePlaywright/public$' -v -count=1 -timeout=10m
```

The fixture needs `/mcp_probe/static/browser-fidelity.html`,
`browser-privacy.html`, and `browser-denied.html`. Its prepared allowlist must
include the three origins in the setup example **and**
`https://127.0.0.1.nip.io` for the approved-name/private-IP denial probe. That
fourth origin is test-only and must still be rejected by the destination-IP
policy; it is intentionally absent from the normal setup example. Tests assert
ten tools, forbidden-tool/argument/navigation rejection, meaningful refs/boxes,
successful approved assets, image bounds/provenance, privacy markers, and cleanup.
The Go test definition alone is not proof these checks passed live.

## Troubleshooting

- Missing runtime, image, or proxy: close MCP workers, then explicitly run `mcp browser setup` again with the intended complete allowlist. Server startup never prepares or repairs the runtime.
- Changed Odoo IP, network, or public URL: setup again after closing workers. The saved policy binds the actual Odoo IP and rejects stale runtime state.
- Login fails: verify the local `admin` / `admin` account or set the credential overrides, then restart the MCP session. Each empty/unset override independently uses `admin`. No password changes, alternate credentials, or retries are performed automatically. Do not bypass the positive UID check or treat `--public` as evidence that authentication works.
- API tunnel denied: the prior authentication failure was fixed by allowing CONNECT only to the exact Odoo host, discovered IP, and port 8069. Check that the prepared policy matches current source/state; do not open CONNECT to arbitrary private destinations or weaken the destination checks.
- Assets fail: check exact approved hosts, HTTPS port 443, public resolved addresses, CORS/frame restrictions, and the deliberate service-worker/WebSocket limitations. Approval does not override browser security policies.
- Backend timeout/error: restart the MCP session; no automatic retry or continuation from uncertain state.
- Docker commands hang: distinguish client availability from daemon health. A present `docker` executable or client version does not prove a responsive daemon. Restore daemon access first; client/MCP configuration cannot fix it. Do not reset Odoo data to repair daemon health.
- Stop/setup refuses workers: disconnect the clients and let session cleanup finish before retrying the explicit lifecycle operation.
