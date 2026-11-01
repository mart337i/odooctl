# Browser Guide

Choose the browser mode by network needs, interaction limits, and artifact
privacy. All three are development workflows, not a guarantee of read-only
application behavior. Login creates sessions; GET requests, custom controllers,
and page JavaScript can trigger writes or external server-side effects.

## Three Modes

| Boundary | Ordinary `browser ...` CLI | Strict `mcp serve --browser` | Advanced `mcp browser` |
| --- | --- | --- | --- |
| Runtime | Python Playwright inside Odoo | Python Playwright inside Odoo | Separate Node/Microsoft Playwright MCP worker and Squid proxy |
| Preparation | Browser-enabled Odoo image, Odoo 15.0+ | Same image requirement | Explicit `mcp browser setup`; no Python browser required in Odoo image |
| Navigation | Paths and external HTTP/HTTPS URLs accepted | Local absolute paths only, fixed local Odoo origin | Tool navigation local; page-driven navigation/assets may use approved external HTTPS origins |
| Credentials | Empty defaults; explicit `--login` and `--password` | Server-side `admin` / `admin` defaults | Same defaults; `--public` skips login |
| Observation | Text, console/request failures, check, PNG, trace | Bounded inspect and native PNG tools | Eight browser tools plus two context tools |
| Viewport | Default 1440x1000, configurable | Fixed 1280x800 | Resizable, including mobile-sized viewports |
| Screenshots | Unmasked full-page file | Input-masked native PNG, 2 MiB limit, no disk artifact | Unmasked viewport PNG/JPEG, validated private exports |
| External policy | More permissive; no strict MCP origin guard | External requests/redirects blocked | Exact approved HTTPS hosts through proxy; private destinations denied |

Use ordinary CLI for manual captures and trace debugging, strict MCP for bounded
local observation alongside source/SQL/ORM tools, and advanced MCP when CDN,
Three.js, map frames, or responsive screenshots matter. None exposes a complete
unrestricted user browser through its inspection interface.

## Ordinary CLI

Operator preparation changes images/services and is separate from observation:

```bash
odooctl docker create --browser
# For an existing environment instead:
odooctl docker reconfigure --browser --rebuild
```

In-Odoo Python Playwright is isolated in `/opt/odoo-browser-venv` to avoid Odoo
dependency conflicts. Support starts at Odoo 15.0; images predating isolation
need rebuilding. The intended environment must be running for observation.

```bash
odooctl browser doctor --json
odooctl browser inspect /web --json
odooctl browser check /web/login --expect-text "Log in"
odooctl browser screenshot /web --width 390 --height 844 --output mobile.png
odooctl browser trace /web --output web-trace.zip
```

`doctor` launches Chromium to check readiness; it is an active ephemeral probe,
not just an installed-package check. `snapshot` returns visible text, not the
advanced backend's native ARIA snapshot with references. `check` tests visible
text, not all frontend functionality.

Page commands default to `/web`. Shared options are `--json`, `--login`,
`--password`, `--timeout` and `--wait` (milliseconds), `--width`, and `--height`.
Login/password defaults are empty: no login is attempted without `--login`.
No `admin` fallback or MCP credential environment variables apply. Prefer
disposable accounts; password flags can expose secrets in history/process lists.

External URLs are accepted and are not subject to strict MCP's local-origin
policy. Configured localhost Odoo URLs are mapped to the container-local server.
Review destinations and loaded code. Screenshots are full-page, unmasked PNGs;
traces include screenshots, snapshots, and sources and can retain sensitive
session/page information. Default artifacts are environment-local; `--output`
can copy to a chosen host path. Do not assume private file permissions or secret
redaction. Review captures before sharing or committing them.

## Strict MCP

```bash
odooctl mcp serve --project /absolute/path/to/odoo-addons --browser
```

This adds `browser_inspect` and `browser_screenshot` to the project inspection
server. Both accept only a local absolute `path`, default `/web`; external URLs,
protocol-relative paths, and fragments are rejected. Browser traffic stays on
`http://127.0.0.1:8069` inside Odoo; external requests and off-origin redirects
are blocked. WebSockets, service workers, and downloads are also blocked.

The CLI independently defaults login and password to `admin`. Each nonempty
`ODOOCTL_MCP_BROWSER_LOGIN` or `ODOOCTL_MCP_BROWSER_PASSWORD` overrides its
respective default; unset or empty values fall back to `admin`. This is different
from ordinary CLI page flags. Credentials are server environment values, never
tool inputs. Authentication failure does not change passwords or try alternatives.

Observation uses bounded readiness checks and reports `incomplete` when required
content does not settle. Screenshots are fixed 1280x800 viewport PNGs, returned
as native MCP image content in memory, capped at 2 MiB, with input controls
masked. The server writes no screenshot artifact; a client can still save it.
Other pixels and text can expose private records. Fixed login, page GETs and
JavaScript remain side-effectful even without click/eval tools.

Read [MCP browser observation](ai/MCP.md#browser-observation) for the strict
boundary and [MCP SQL/source controls](ai/MCP.md) for the other capabilities.
Neither ordinary CLI SQL nor browser observation inherits MCP SQL transaction
guards merely by printing JSON.

## Advanced MCP

This separate mode uses Microsoft's official `@playwright/mcp` backend pinned
to `0.0.83`, Node 22, Chromium, and Squid in Docker. It requires an existing
running Odoo environment, Linux containers, IPv4 Docker networking, and the
Docker Compose plugin. Host Node and Python Playwright in the Odoo image are
not required. The feature is currently source-level work: use a binary built
from that source and check its help. An installed release or `@latest` install
does not establish that these commands are available.

An operator explicitly prepares the runtime and reviews the complete allowlist:

```bash
odooctl mcp browser setup --project /absolute/path/to/odoo-addons \
  --allow-origin https://cdn.jsdelivr.net \
  --allow-origin https://www.openstreetmap.org \
  --allow-origin https://tile.openstreetmap.org
odooctl mcp browser --project /absolute/path/to/odoo-addons --public
```

Setup builds images, prepares a private network, and starts only the proxy. It
does not create, build, start, or initialize Odoo/database services. Repeating
setup replaces the complete allowlist; no origins means local Odoo only. The
stdio server loads prepared state and starts a session worker, not setup/repair.
Omit `--public` for authenticated observation, with the same independent
`admin` defaults and nonempty MCP environment overrides as strict mode.
Credentials reach the Node worker through its launch environment, not tool
arguments or password flags. Public mode discards them.

Eight browser tools are exposed: navigate, snapshot, take screenshot, console
messages, network requests, resize, wait, and close (all with `browser_` prefixes).
`project_context` and `browser_context` bring the total to ten tools. Call context
first and discover exact schemas. No clicks, typing, interaction/eval, arbitrary
code, uploads, cookies, tabs, or agent-selected filenames are available.
Console/request output helps diagnose frontend errors without exposing headers
or response bodies. Resize supports desktop and mobile-sized observation.

Approved origins must be exact external HTTPS DNS origins on port 443: no
wildcards, IP literals, credentials, or arbitrary paths/ports. Private/reserved
resolved destinations are denied even for approved names. Tool-requested
navigation stays local, but page scripts, frames, and redirects can navigate to
approved external hosts, including the main frame. Real redirects, CORS, and
frame policies still apply. Approval trusts the origin's code and data handling,
not just one harmless asset; Squid does not decrypt TLS or constrain URL paths.

Native ARIA production is patched to omit input/textarea values across frames
and automatic snapshots without changing the DOM. Other text/control state can
remain. **PNG/JPEG screenshots are unmasked pixels**, unlike strict mode.
Viewport-only images are validated, capped at 2 MiB per result, and exported
under `browser-runtime/artifacts/session-<id>/` with directories `0700` and files
`0600`. Text/log/snapshot scratch paths stay in a 64 MiB worker tmpfs and are not
exported. No raw trace export is exposed. Private permissions do not make pixels
safe to send to a model provider; review and manage retained image files.

Software WebGL can render approved Three.js scenes; Chromium uses `--no-sandbox`
for trusted development. Container/proxy restrictions are defense in depth, not
a security certification. WebSockets and service workers are blocked, reducing
normal browser fidelity. Do not claim full GPU, PWA, or unrestricted Chrome
compatibility, and keep production data/unreviewed code out of this workflow.

Disconnect MCP clients before `odooctl mcp browser stop --project PATH`. Stop
refuses active workers and removes the proxy/private network while retaining
images, runtime files, screenshots, and Odoo data. `browser_close` closes the
browser, not the MCP process/proxy. Restart a session after backend errors;
startup does not silently repair stale runtime state.

## Clients And Evidence

For OpenCode's local MCP command array in `opencode.json`/`opencode.jsonc`, use
the [advanced client examples](ai/PLAYWRIGHT_MCP.md#client-examples) or
[strict client configuration](ai/MCP.md#client-configuration). Those guides
also document Claude Code; do not mix their JSON schemas or commit credentials.

The [historical MCP matrix](ai/MCP_VERSION_MATRIX.md) records strict-mode Odoo
Community 17-20 fixture checks. The [advanced verification guide](ai/PLAYWRIGHT_MCP.md#verification-boundary)
separately records CDN/Three.js/maps, desktop/mobile, authentication, privacy,
and rejection checks. These are fixture results, not all-app/Enterprise coverage
or a statistical model benchmark; call counts/timings are evidence, not general
performance claims.

For failures, first distinguish daemon/mount/network health from page failures.
Doctor probes actively; browser doctor checks the in-Odoo runtime, not the
advanced sidecar. Fix Docker health before retrying setup. Never reset database
volumes to repair a browser. See [advanced troubleshooting](ai/PLAYWRIGHT_MCP.md#troubleshooting)
and [command effects](COMMANDS.md).
