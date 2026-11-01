# MCP Agent Usage Matrix

Run date: 2026-10-02. Scope: pinned Odoo Community 17.0, 18.0, 19.0 and 20.0
with a disposable addon and initialized `base`/`web` databases. This is a
development inspection compatibility test, not an exhaustive Odoo application,
Enterprise, record-rule, or migration certification.

## Identical Request

Four fresh general-purpose subagents received this exact task in each round:

> Using the Odoo MCP server, verify that base is installed and compare its id,
> name, and state through ORM and SQL. Find the local mcp_probe addon and report
> its dependencies with a source-file and line reference. Inspect the
> authenticated /web page and take a screenshot. Report the Odoo version and
> any failures. Do not change business data.

Only connection parameters changed: project, expected version, and artifact
directory. Agents discovered tool schemas first and chose their own calls.
They were instructed to invoke only the test client bridge, with no direct filesystem,
Docker, HTTP, SQL or Odoo-shell inspection and no code edits. Each bridge call
used the official SDK and actual CLI over MCP stdio. This was agent-driven
usage, not four copies of the scripted integration test.

The test client verifies `project_context` binding before each operation,
records a redacted JSONL transcript, and saves verified native MCP PNG content
with dimensions, size and SHA-256. Browser credentials remain server-side.

## Isolated Fixtures

Each version had its own project root, Compose project, database volume,
browser-enabled image, Odoo user 2/company 1, and `odooctl_inspect` login.
The SQL role had only column-level SELECT on `ir_module_module.id/name/state`,
with no ownership, elevated role attributes or application-role membership.

All four fixture manifests were byte-identical. `mcp_probe` has no models or
business data, depends only on `base`, and was installed alongside `base` and
`web` without demo data. The dependency declaration is at
`mcp_probe/__manifest__.py:6`.

Manifest SHA-256:
`8117af92e934a2a0d49da5b52b27681f13e15657a5b6e2db0172f1581cd170b1`.

The identical manifest used for each fixture:

```python
{
    'name': 'MCP Usage Probe',
    'version': '1.0.0',
    'license': 'LGPL-3',
    'summary': 'Disposable addon for MCP version-matrix inspection',
    'depends': ['base'],
    'installable': True,
    'application': False,
}
```

Its `__init__.py` contains only a comment, so it executes no addon logic.

| Odoo | Pinned Community Revision | PostgreSQL | Observed Web Port |
| --- | --- | --- | ---: |
| 17.0 | `86b6f4c73e5e55eeaada58bd55cd7afb8bec68f8` | 15 | 9710 |
| 18.0 | `f08f1b8d9ce350c09f7a59766a0edb6df79517db` | 15 | 9820 |
| 19.0 | `5a4e4c21d8eb968af60dcbb8511a59ceaf3ff3e7` | 15 | 9940 |
| 20.0 | `b927730f3000b0ea15fb4c24d503298c04cafa9b` | 16 | 10000 |

Actual Odoo binary versions were verified during setup. Port allocation avoided
existing projects; these observed ports are not fixed prerequisites for reruns.
Only the four new fixture databases were initialized. Existing projects and
volumes were not reset or migrated.

## First Round

Every agent verified matching ORM/SQL values and the correct dependency source.
All four correctly reported PARTIAL rather than counting browser errors as a
clean pass.

| Odoo | Browser Outcome | SQL Retry |
| --- | --- | --- |
| 17.0 | Authenticated observation and screenshot failed; session expired | Removed unsupported `LIMIT 1` |
| 18.0 | Navbar-only capture with interrupted RPCs and service-worker errors | None |
| 19.0 | Navbar-only capture with interrupted RPCs and service-worker errors | None |
| 20.0 | Navbar-only capture with interrupted RPCs and service-worker errors | Removed unsupported `LIMIT 1` |

The first-round screenshots for 18-20 were approximately 9 KB and did not
demonstrate fully rendered application content. They and their transcripts were
retained separately; first-round failures were not overwritten.

## Fixes

- Odoo 20 requires PostgreSQL 16: PostgreSQL 15 failed initialization on
  `any_value`. Templates now select 16 for 20+ and retain 15 for 12-19. The
  failed disposable 20 PostgreSQL 15 volume was preserved and replaced in the
  fixture mapping with a new PostgreSQL 16 volume, not upgraded in place.
- Explicit HTTP interface configuration preserves host-port access with Odoo
  20's changed default, while respecting user configuration overrides.
- Guarded redirect fetches no longer replay the original Cookie header after
  session rotation; Playwright's shared URL-scoped cookie jar supplies it.
  Every redirect hop remains origin-checked with automatic redirects disabled.
- Login startup settles before target navigation. Web-client readiness requires
  rendered action content and a bounded quiet interval across guarded requests,
  not merely a navbar. Events are copied before teardown so context-close
  cancellations are not misreported as observed failures.
- The browser exposes service workers as unavailable while continuing to block
  them. It does not emulate successful registration or suppress real page errors.
- SQL schemas explicitly prohibit `LIMIT`, `OFFSET`, semicolons and functions,
  and explain the server's 200-row cap.

## Second Agent Round

Four new agents received the same request without first-round results or
version-specific troubleshooting hints. All passed, with no retries and no
reported browser events. Every result was independently checked against its
saved transcript and actual PNG bytes.

| Odoo | Matching ORM/SQL `base` | Dependency Citation | Browser Events | PNG Bytes | Outcome |
| --- | --- | --- | ---: | ---: | --- |
| 17.0 | `37 / base / installed` | Line 6, `['base']` | 0 | 149279 | PASS |
| 18.0 | `34 / base / installed` | Line 6, `['base']` | 0 | 140740 | PASS |
| 19.0 | `36 / base / installed` | Line 6, `['base']` | 0 | 136319 | PASS |
| 20.0 | `34 / base / installed` | Line 6, `['base']` | 0 | 151341 | PASS |

Each agent used six requested tool calls and one schema-discovery invocation.
The bridge additionally made seven automatic binding checks: 13 tool executions
and one schema discovery per agent. All PNGs were 1280x800, showed the rendered
Apps catalog, and had `web_client_visible` readiness with `incomplete=false`.

Summed client invocation time, excluding agent reasoning and build/setup time:
17: 9.966 seconds; 18: 10.448; 19: 11.444; 20: 11.721.

Second-round binaries, identical across the four agents:

- CLI SHA-256: `2435a3e90bd6eeaf176c7e8b861f7756c70135571beec15df45e8498ec929339`.
- Client bridge SHA-256: `75f0af6e78c278cdc3a113e25ce615d9e9722e70ee1e8b307b31da5e9ca983db`.

These are worktree build fingerprints, not release identifiers. The additional
aggregation fix below followed this agent round and was verified through live
regressions; the requested agent task did not invoke aggregation.

## Broader Regressions

In addition to the agent task, each version passed the opt-in local/source,
SQL/ORM/browser, browser-fidelity, and ORM read-only transaction suites.
Coverage includes model metadata, grouped counts, bounded record reads,
ungranted-column/write/function/transaction-control rejection, model and company
allowlists, sensitive-field rejection, off-origin rejection and clean stdio EOF.

Broader testing identified another Odoo 20 change: public `read_group` still
exists, but its legacy `fields` signature was replaced by the tuple/aggregates
API. The MCP adapter retains the 17-19 call and normalizes modern grouped tuples
into the same dictionary shape. The Odoo 20 full live suite then passed; live
legacy grouping remained passing on 17-19.

On all four versions, `TestDatabaseLiveORMReadOnly` confirmed SQLSTATE `25006`
for a fixed zero-row UPDATE, recovered to a savepoint and completed the read.
The probe changes no records and ends with full rollback.

## Evidence And Reruns

Local artifact root: the sibling directory
`../odooctl-mcp-matrix-20261002/artifacts` relative to the repository root.
Per-version directories are `v17`, `v18`, `v19` and `v20`; each contains separate
`round1` and `round2` transcripts and returned PNGs. Artifact directories are
owner-only and files use `0600`; pixels can still contain sensitive data on
non-disposable environments.

Build the test-only bridge and CLI:

```bash
go build -o bin/odooctl-mcp-matrix .
go build -o bin/mcp-usage ./internal/mcp/usagebridge
```

Provide each agent with its own `PROJECT`, `VERSION`, `ARTIFACTS` and an absolute
`PINNED_BINARY` path. Schema discovery and calls use:

```bash
bin/mcp-usage --project "$PROJECT" --expected-version "$VERSION" \
  --artifacts "$ARTIFACTS" --binary "$PINNED_BINARY" --development-login list

bin/mcp-usage --project "$PROJECT" --expected-version "$VERSION" \
  --artifacts "$ARTIFACTS" --binary "$PINNED_BINARY" --development-login \
  call TOOL '{"argument":"value"}'
```

`--development-login` is only for authorized disposable fixtures. Otherwise use
the server credential environment variables documented in [MCP.md](MCP.md).
Agents should choose actual tool names and arguments from the returned schemas.

Independently audit the second-round saved evidence:

```bash
ODOOCTL_MCP_MATRIX_ARTIFACTS=/absolute/path/to/artifacts \
  go test ./internal/mcp/usagebridge -run '^TestAgentUsageTranscripts$' -v -count=1
```

Live regression commands and runtime prerequisites are in [MCP.md](MCP.md).
No live capabilities may be counted as passing when their opt-in tests skip.

## Remaining Limits

- One deterministic addon/task and two agent rounds are not a statistical agent
  benchmark or comprehensive compatibility coverage.
- Many icon buttons have no text in the DOM summary. The client output also
  duplicates structured/native/text results and can be verbose.
- External requests, service workers and websockets are unavailable. The
  controlled browser is not equivalent to an unrestricted user browser.
- Authentication creates sessions and page loads can have side effects. These
  runs used disposable data and performed no business-action clicks. Read-only
  ORM transactions are not a sandbox for custom Python.
- The four fixture services are stopped after verification. Their volumes,
  pinned source state and artifacts are retained for inspection or reruns;
  unrelated active projects remain running.
