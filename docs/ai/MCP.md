# MCP Inspection Server

`odooctl mcp serve` exposes project-scoped development inspection tools over
MCP stdio. The client launches the process; stdout is reserved for protocol
messages and diagnostics go to stderr. There is no HTTP listener, remote MCP
endpoint, or MCP authentication service.

```bash
odooctl mcp serve --project /absolute/path/to/odoo-addons
```

The project defaults to `.` and is fixed for the process lifetime. Discovery is
passive: the server does not create environments, repair state, build images,
start containers, install modules, or provision database permissions. Restart
the server after changing its project, environment, or capability options.
An existing project link selects the environment. Without a link, passive
fallback discovery rejects multiple matching environments instead of guessing;
repair/select the intended link through the ordinary CLI before starting MCP.

## Default Tools

Without an existing odooctl environment, exactly five local-only tools are
available. Neither Docker nor database credentials are needed for these tools.

| Tool | Purpose |
| --- | --- |
| `project_context` | Project metadata, approved source roots, and enabled capabilities. Call this first. |
| `module_list` | Local modules, parsed manifests, direct dependencies, and lookup warnings. |
| `module_info` | All local definitions of a technical module name and its direct dependencies. |
| `code_read` | A bounded, line-numbered source snippet from an approved root. |
| `code_search` | Literal, case-sensitive substring search, not a regular expression. |

When passive discovery finds an existing environment, `runtime_status` and
`logs_recent` are registered automatically, without an extra opt-in flag.
Status queries Compose without starting services. Logs are finite, default to
100 lines from `odoo`, allow `odoo` or `db`, and accept a `limit` of 1 to 500.
Log text is capped at 128 KiB; there is no follow mode. These tools need working
Docker/Compose access when called, but not during passive startup discovery.

## Capability Options

| Option | Default and requirement |
| --- | --- |
| `--project PATH` | `.`; existing project directory, resolved to an absolute path. |
| `--sql-role ROLE` | Disabled; explicit dedicated PostgreSQL login role (`SQLRole`). Must be supplied together with `--sql-tables`. |
| `--sql-tables TABLES` | Disabled; comma-separated allowlist of 1 to 50 unqualified `public` table names (`SQLTables`). |
| `--odoo-user-id UID` | Disabled; explicit active, non-superuser Odoo user ID greater than 1. Requires `--models`. |
| `--models MODELS` | Disabled; comma-separated allowlist of 1 to 50 technical model names. |
| `--company-ids IDS` | User's main company by default; comma-separated positive company IDs, at most 50. Used with ORM. |
| `--browser` | Disabled; requires both server environment credentials below. |
| `--container-source` | Disabled; adds core and Enterprise source lookup in the running Odoo container. |

SQL, ORM, browser, and container source require an existing odooctl environment.
Execution tools use existing running containers; none automatically starts
them. SQL needs `db`; ORM, browser, and container source need `odoo` (and any
runtime dependencies their operation requires). Capabilities can be enabled
independently or combined. A table/model allowlist is not a replacement for
database grants, Odoo access controls, or a security review.

## Client Configuration

These are documentation examples, not configuration installed by odooctl.
Merge the relevant entry into your chosen client's configuration. Replace the
project placeholder and use an absolute binary path if `odooctl` is not on the
client's `PATH`. Each client launches its own stdio process; multiple clients
can inspect the same project without sharing database transactions or browser
contexts.

### OpenCode

An `opencode.json` or `opencode.jsonc` entry uses `mcp`, a `local` type, and a
command **array** containing the executable and arguments:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "odooctl": {
      "type": "local",
      "command": ["odooctl", "mcp", "serve", "--project", "/absolute/path/to/odoo-addons"],
      "enabled": true
    }
  }
}
```

OpenCode uses `environment` for server environment overrides. For an explicitly
browser-enabled entry, append `"--browser"` to the command array and add:

```json
{
  "environment": {
    "ODOOCTL_MCP_BROWSER_LOGIN": "{env:ODOOCTL_MCP_BROWSER_LOGIN}",
    "ODOOCTL_MCP_BROWSER_PASSWORD": "{env:ODOOCTL_MCP_BROWSER_PASSWORD}"
  }
}
```

Supply those variables securely in the environment used to launch the client;
do not put actual credentials in version-controlled configuration. Check the
connection with `opencode mcp list`.

### Claude Code

Claude Code's project `.mcp.json` uses `mcpServers`, a command **string**, and a
separate argument array:

```json
{
  "mcpServers": {
    "odooctl": {
      "type": "stdio",
      "command": "odooctl",
      "args": ["mcp", "serve", "--project", "/absolute/path/to/odoo-addons"]
    }
  }
}
```

Alternatively, from the intended project directory, this command registers a
private, local-scoped entry in Claude Code's configuration:

```bash
claude mcp add --transport stdio --scope local odooctl -- \
  odooctl mcp serve --project /absolute/path/to/odoo-addons
```

The `--` separates Claude Code options from the server command. Review and
approve project-scoped configurations when prompted. Check with
`claude mcp list` or `/mcp` inside Claude Code.

For browser opt-in, append `"--browser"` to `args` and add an `env` object to
the server entry. Claude Code's interpolation syntax differs from OpenCode's:

```json
{
  "env": {
    "ODOOCTL_MCP_BROWSER_LOGIN": "${ODOOCTL_MCP_BROWSER_LOGIN}",
    "ODOOCTL_MCP_BROWSER_PASSWORD": "${ODOOCTL_MCP_BROWSER_PASSWORD}"
  }
}
```

Both variables must be set before launching the client. No password CLI flag
or agent-supplied credential tool argument is supported.

## SQL Inspection

Enable `database_query` and `database_schema` explicitly:

```bash
odooctl mcp serve --project /absolute/path/to/odoo-addons \
  --sql-role odooctl_inspect --sql-tables ir_module_module
```

Role and table names must be lowercase PostgreSQL identifiers. Application or
privileged role names such as `odoo`, `postgres`, and `pg_*` are rejected. The
server checks that the connected role is not superuser, cannot create roles or
databases, cannot replicate, cannot bypass RLS, and is the session login role.
Inherited privileged/application roles and effective ownership of any
allowlisted table are also rejected, because owners can bypass ordinary RLS.
It also requires ordinary `public` tables and supported built-in scalar
columns; views, generated columns, and user-defined column types are excluded.
Sensitive-looking table and column names are rejected even if grants allow
access. SQL bypasses Odoo ACLs and record rules; PostgreSQL grants and RLS are
the relevant authorization boundary.

### Restricted Grammar

`database_query` is not an arbitrary PostgreSQL console. Its `query` supports:

```text
SELECT column, column FROM allowed_table
  [WHERE column comparison literal [AND/OR column comparison literal ...]]
  [ORDER BY column [ASC/DESC], ...]
```

Comparisons are `=`, `<>`, `<`, `>`, `<=`, `>=`, plus `IS NULL` and `IS NOT NULL`.
Literals are non-negative numeric values, `true`/`false`, or single-quoted
printable ASCII strings without backslashes. Double a quote inside a string.
Normal SQL AND/OR precedence applies; parentheses are not supported.

There are no joins, functions (including `count()`), `*`, aliases, casts,
subqueries, comments, semicolons, qualified identifiers, user-supplied `LIMIT`
or `OFFSET`, or transaction controls. Use explicit columns in SELECT, WHERE,
and ORDER BY, all within the role's safe granted subset. Queries are limited
to 8192 bytes, 512 tokens, and 64 column references.

Example `database_query` arguments:

```json
{
  "query": "SELECT id, name, state FROM ir_module_module WHERE state = 'installed' ORDER BY name ASC"
}
```

The server rewrites the table to `ONLY public.<table>` and fetches at most 201
rows to detect truncation, returning at most **200 rows**. `database_schema`
accepts an optional `table`, limited to the configured allowlist, and also caps
its returned column definitions at 200. Results include provenance and a
`truncated` flag.

Each call uses a fresh `psql` process with `BEGIN READ ONLY`, a **5-second
statement timeout**, a 1-second lock timeout, a 10-second idle-in-transaction
timeout, `search_path = pg_catalog`, and `row_security = on`. It establishes
`SAVEPOINT inspection` and ends with a **full ROLLBACK**, not a commit or merely
a rollback to the savepoint. Errors stop psql and the connection closes. There
are no reusable SQL sessions, writes, commits, or session-management tools.

### Manual DBA Setup

There is **no automated role provisioning**. A DBA must create and audit the
dedicated role, grants, schema, and authentication configuration. The following
is an illustrative DBA SQL script, executed in the intended development
database, not an MCP query. Replace the database identifier `odoo_dev` with the
database reported by `project_context`:

```sql
CREATE ROLE odooctl_inspect LOGIN
  NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
ALTER ROLE odooctl_inspect SET default_transaction_read_only = on;

REVOKE ALL ON DATABASE odoo_dev FROM odooctl_inspect;
GRANT CONNECT ON DATABASE odoo_dev TO odooctl_inspect;
REVOKE ALL ON SCHEMA public FROM odooctl_inspect;
GRANT USAGE ON SCHEMA public TO odooctl_inspect;
REVOKE ALL ON TABLE public.ir_module_module FROM odooctl_inspect;
GRANT SELECT (id, name, state)
  ON TABLE public.ir_module_module TO odooctl_inspect;
```

Grant **only safe columns**, including any needed for filtering or ordering.
Do not grant blanket table SELECT, all-table SELECT, application-role
membership, table ownership, schema CREATE, or elevated attributes. Name-based
sensitive-field filtering is only defense in depth; apparently harmless
columns can contain private data. The example's REVOKEs remove direct grants,
not permissions inherited through `PUBLIC` or other roles.

PostgreSQL commonly grants function `EXECUTE` to `PUBLIC` by default. Revoking
EXECUTE only from `odooctl_inspect` cannot cancel permissions it inherits from
`PUBLIC`: PostgreSQL has no per-role deny overriding a PUBLIC grant. The DBA
must audit schema privileges, executable functions (especially
SECURITY DEFINER), RLS policies, table ownership, and role memberships. Where
necessary, revoke grants from `PUBLIC` and explicitly regrant them to the
appropriate application roles, after assessing the impact on the application.
Read-only transactions block database writes but do **not** sandbox functions,
RLS policy code, or other code with external side effects.

SQL inspection executes `psql -X --no-password -U odooctl_inspect` **inside the
`db` container**, without a host argument, using container-local authentication.
The generated `postgres:15` environment typically has local socket `trust`
authentication initialized by the image; inspect the actual `pg_hba.conf` of
your existing volume rather than assuming it does. A working, noninteractive
`psql --no-password` login as this role is a prerequisite. There is **no SQL
password input supported** by MCP: no password flag, tool argument, or MCP SQL
password environment variable. If local authentication requires a password or
rejects the role, a DBA must arrange an approved container-local authentication
configuration. Do not enable network-wide trust to make inspection work.
Local trust permits processes with container access to choose a database role;
Docker access itself is privileged and must be restricted.

## ORM Inspection

Enable `odoo_model_info`, `odoo_search_read`, and `odoo_read_group` explicitly:

```bash
odooctl mcp serve --project /absolute/path/to/odoo-addons \
  --odoo-user-id 42 --models res.partner --company-ids 1,2
```

The UID and companies above are examples; choose an existing active,
least-privileged user and companies they are authorized to access. The tools
construct a non-superuser Odoo environment, enforce model read access and
normal record rules, and use `active_test = True`. Without `--company-ids`, the
configured user's main company is used. Tool-level `company_ids` can narrow the
configured companies, never widen them beyond the server allowlist or the
user's company membership.

Company IDs select Odoo's `allowed_company_ids` context; they are **not a hard
row-isolation boundary**. Record rules may expose shared records or records
outside that selection. Use a suitably restricted user and reviewed record
rules when company-level data isolation is required.

Only accessible, non-sensitive, **stored scalar** fields are offered: boolean,
integer, float, monetary, char, text, selection, date, and datetime. Relational
fields, including **many2one**, are unavailable, even when stored. Dotted
related-field traversal is not supported in fields, domains, groups, or
ordering. The current field filter checks storage and scalar type, not the
`related` attribute itself: a stored scalar related field can still qualify.
Do not assume related fields are categorically excluded or custom field code
is sandboxed.

Example `odoo_search_read` arguments:

```json
{
  "model": "res.partner",
  "fields": ["id", "name", "active"],
  "domain": [["active", "=", true]],
  "order": "id asc",
  "company_ids": [1],
  "limit": 20
}
```

Domains support bounded scalar comparisons, membership, like/ilike variants,
and prefix `&`, `|`, `!`; no relational traversal or arbitrary expressions.
Ordering uses stored field names with optional lowercase `asc`/`desc`.
`odoo_read_group` requires explicit `fields` and `group_by`; aggregate suffixes
are `sum`, `avg`, `min`, `max`, `count`, and `count_distinct`. Defaults and bounds
include 200 results, 64 fields/domain nodes, four grouping fields, and an offset
of at most 10000. Internal group domain/context/range metadata is removed.

Each call runs a fresh Odoo shell, rolls back its initial transaction, begins a
driver-managed transaction with `SET TRANSACTION READ ONLY`, verifies
`SHOW transaction_read_only` is `on`, sets the same 5-second statement timeout
and transaction bounds as SQL, establishes a savepoint, and fully rolls back
in `finally`. Explicit `BEGIN READ ONLY` is not used through psycopg because
its implicit BEGIN would make the nested BEGIN ineffective.
There are no persistent ORM sessions, arbitrary Python/method execution tools,
write operations, or commits. Custom model methods, access rules, and field
code can still have side effects; database read-only is not a Python sandbox.
Registry initialization also runs existing addon code. An ordinary write in the
inspection transaction is rejected, but malicious custom Python can replace
transaction controls or open another connection. Only enable ORM against
trusted development code; the shell's application database credentials are not
a least-privilege SQL-role boundary.

## Source Inspection

Use `project_context` to discover root IDs before reading source. `project`
identifies the first local root; additional configured addon directories use
`addons-1`, `addons-2`, and so on. Configured external addon directories are
independently approved roots, not necessarily children of the project.
Module lookup remains local and reports collisions, unavailable manifests,
and missing direct dependencies, which may be supplied by the container.
Manifest parsing is lightweight and non-executing; computed Python manifest
values and unusual formatting may not be fully represented.

Example `code_read` arguments:

```json
{
  "root": "project",
  "path": "my_module/models/partner.py",
  "start_line": 1,
  "limit": 80
}
```

Example `code_search` arguments:

```json
{
  "root": "project",
  "path": "my_module",
  "query": "_inherit",
  "limit": 20
}
```

Reads default to 100 lines, permit 1 to 200, and return `next_line` when more
source remains. Searches return at most 100 matching lines, visit at most 10000
entries, and have a 16 MiB read budget and a 5-second search deadline. Regular
UTF-8 text files must be at most 1 MiB; individual output lines are capped at
2000 bytes. Results report truncation and warnings rather than promising a
complete codebase scan.

Paths must be relative to the selected root. Absolute paths, traversal/dot
components, hidden components, backslashes, NULs, and colons are denied.
Secret-ish component names containing `credential`, `secret`, `password`,
`token`, `config`, `state`, `private`, `api_key`, `access_key`, `.pem`, or `.key`
are denied, as are SSH private-key names and `auth.json`. This intentionally
also blocks some legitimate source paths such as `models/res_config.py`.

Allowed file extensions are `.py`, `.xml`, `.js`, `.ts`, `.scss`, `.css`, `.csv`,
`.md`, `.txt`, `.json`, `.html`, `.yaml`, `.yml`, `.toml`, and `.rst`. Binary
files and disallowed extensions (including `.go`) are not exposed. All source
file/directory symlinks beneath an approved root are denied. Host reads use
guarded root-relative handles; container reads use descriptor-relative opens
with no-follow flags. Module manifests are parsed from the safely read bytes.

Container source is separately opt-in:

```bash
odooctl mcp serve --project /absolute/path/to/odoo-addons --container-source
```

This adds `odoo-core` at `/opt/odoo-src` and `odoo-enterprise` at
`/opt/odoo-enterprise` for `code_read`/`code_search`, with equivalent path and
text restrictions. It requires a running Odoo container and the relevant root
to exist; Enterprise source is not fetched or installed by the tool. For
example, read `odoo/models.py` from `odoo-core` with a bounded line range.

## Browser Observation

Browser tools require explicit `--browser` plus these server environment
variables:

- `ODOOCTL_MCP_BROWSER_LOGIN`
- `ODOOCTL_MCP_BROWSER_PASSWORD`

Use a dedicated, least-privileged Odoo account. The existing environment must
have Playwright Chromium enabled in its image, support **Odoo 15.0+**, and have
running containers. Environment preparation (`odooctl docker create --browser`
or `odooctl docker reconfigure --browser --rebuild`) is a separate operator
action, not something MCP performs automatically.
Playwright runs in `/opt/odoo-browser-venv`, separate from Odoo's Python
environment, to avoid incompatible dependency pins such as `greenlet`.
Images built before this isolation change must be rebuilt.

`browser_inspect` returns bounded visible text, a DOM summary, and diagnostic
events. `browser_screenshot` returns an in-memory PNG at a fixed 1280x800
viewport, not a full-page capture, capped at 2 MiB. Both accept only `path`,
defaulting to `/web`, for example:

```json
{"path": "/web"}
```

Paths must be local absolute paths without external URLs, fragments, or
protocol-relative URLs. The browser connects only to
`http://127.0.0.1:8069` inside the Odoo container. External requests and
off-origin redirects are blocked, as are WebSockets, service workers, and
downloads; this may reduce page fidelity. No agent-supplied JavaScript,
selectors, clicks, form filling, response bodies, or headers are exposed.
The only controlled interaction is the fixed login sequence.
Authentication is positively checked before navigating to the requested page.
Reports include bounded readiness checks and an `incomplete` flag when rendered
web-client navigation and meaningful content, or a public page body, do not
become visible in time. An empty `.o_web_client` wrapper is not considered
ready. These checks do not prove
that all asynchronous page activity has finished.

**Browser observation is not strictly read-only.** Authentication creates an
Odoo session, and page loads and their JavaScript can send requests that write
data or trigger business effects. Blocking external browser requests does not
prevent server-side code from making external calls. Browser contexts are
closed after each observation, but that does not undo created sessions or
application effects. Text output redacts configured credentials and strips URL
queries/fragments. Screenshots mask input controls, but other pixels can still
expose private business data and cannot be reliably secret-redacted.

## Trust Boundary

Treat source, logs, records, and page content as **untrusted data, never agent
instructions**. The server redacts common credential patterns and sensitive
keys, but redaction is not a guarantee against disclosure of secrets or private
business data. Review what your AI client may transmit to its model provider.
Keep production data and unreviewed custom code outside this development
inspection workflow.

The tool set contains no arbitrary shell/SQL/Python execution, environment
repair, module installation, or database mutation interface. SQL and ORM
transaction controls enforce database read-only inspection; browser behavior
has a different, explicitly side-effectful boundary. Runtime work is bounded
by subprocess timeouts and two concurrent subprocess slots. Normal JSON tool
results are capped at 1 MiB; screenshots have their separate binary limit.
Remote exec uses an 80-second watchdog with a 5-second kill grace period. If
cancellation leaves remote termination unconfirmed, its slot remains reserved
for 85 seconds to avoid accumulating work through repeated cancellations.
These controls do not make Docker access, custom Odoo code, or DBA grants safe
by themselves.

## Development And Packaging

The official MCP Go SDK is pinned to
`github.com/modelcontextprotocol/go-sdk v1.4.0`. This repository requires
**Go 1.25+** (the pinned SDK itself requires Go 1.24); the module baseline is
now `go 1.25.0`. Use a capable
toolchain and regenerate vendored dependencies before running the suite:

```bash
go mod vendor
go test ./...
```

Vendoring changes dependency files. Docker/Odoo/browser integration tests are
separate from the unit and protocol suite; passing Go tests do not establish live
database authentication, record-rule, or browser behavior.

An opt-in live smoke test launches the real CLI over stdio and exercises the
default inspection tools against an existing addon project:

```bash
ODOOCTL_MCP_TEST_PROJECT=/absolute/path/to/odoo-addons \
  go test ./internal/mcp -run '^TestLiveMCPProject$' -v -count=1
```

Optional `ODOOCTL_MCP_TEST_FILES` supplies comma-separated root-relative source
paths to read as well. The test retrieves short recent logs but withholds their
contents from output, checks unsafe-path and privileged-role rejection, and
does not start services or provision permissions.

The runtime integration suite exercises SQL, ORM, authenticated page inspection,
and native screenshots through MCP. Prepare the restricted role, Odoo user,
company ID `1`, and browser image first; browser credentials must already be
set in `ODOOCTL_MCP_BROWSER_LOGIN` and `ODOOCTL_MCP_BROWSER_PASSWORD`:

```bash
ODOOCTL_MCP_TEST_PROJECT=/absolute/path/to/odoo-addons \
ODOOCTL_MCP_TEST_SQL_ROLE=odooctl_inspect \
ODOOCTL_MCP_TEST_ORM_UID=42 \
ODOOCTL_MCP_TEST_BROWSER=true \
  go test ./internal/mcp -run '^TestLiveMCPRuntime$' -v -count=1 -timeout=6m
```

Each capability variable is optional; unset it to skip that capability. The SQL
suite assumes `ir_module_module` grants limited to `id`, `name`, and `state`.
ORM uses `ir.module.module` and `res.partner` with company context `1`. It tests
allowlist/field/company rejection; it is not a comprehensive record-rule audit.

`TestDatabaseLiveORMReadOnly` uses the project and ORM UID variables to verify
that PostgreSQL rejects a fixed zero-row UPDATE with SQLSTATE `25006`, rolls
back to the savepoint, then completes the read. It changes no records and ends
with full rollback:

```bash
ODOOCTL_MCP_TEST_PROJECT=/absolute/path/to/odoo-addons \
ODOOCTL_MCP_TEST_ORM_UID=42 \
  go test ./internal/mcp -run '^TestDatabaseLiveORMReadOnly$' -v -count=1
```

When rebuilding legacy images, preserve their existing Compose project and
volume mappings. If the new image's Odoo UID differs from the volume owner,
application-data/session permissions must be adjusted to that UID without
resetting volumes or making them world-writable.

Debian packaging declares `golang-go (>= 1.25)` as its minimum build dependency.
Ubuntu Jammy/Noble packaging therefore requires a build archive that actually
provides a capable Go 1.25+ compiler. Vendored dependencies do not remove the
compiler requirement, and stock older build toolchains cannot build this SDK.
PPA publishing is gated by the repository variable
`ODOOCTL_PPA_GO_125_READY=true`; enable it only once the destination build
archives satisfy the new dependency. Binary release builds remain enabled.

## Troubleshooting

- Missing runtime tools: check `project_context`; status/log tools are added only when an existing environment was discovered. Restart after correcting the project/environment selection.
- SQL tools unavailable: supply both `--sql-role` and `--sql-tables`; the server never creates the role or grants for you.
- SQL subprocess failure: have the DBA verify container-local `psql --no-password` authentication, column grants, role attributes, and the running `db` service. Do not fall back to the application role.
- ORM denial: check the active UID, model allowlist, company membership, read ACLs/record rules, and stored scalar field eligibility.
- Source denial: check the root ID, extension, secret-ish path components, absence of symlinks, and file size.
- Browser failure: check `--browser`, both credential variables, Odoo 15.0+, image Playwright support, and running Odoo. MCP does not rebuild or start it.

Client syntax references: [OpenCode MCP servers](https://opencode.ai/docs/mcp-servers/)
and [Claude Code MCP](https://code.claude.com/docs/en/mcp).
