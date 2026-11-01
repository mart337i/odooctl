# odooctl

Odoo Docker development environments, addon workflows, and AI inspection from
one Go CLI. Build from pinned Odoo source, work with your local addons, and
inspect the database, browser, and codebase through MCP.

[Quick Start](#quick-start) | [Documentation](docs/README.md) |
[Command Reference](docs/COMMANDS.md) | [Contributing](CONTRIBUTING.md)

## Capabilities

| Work | What odooctl provides | Guide |
| --- | --- | --- |
| Environments | Compose generation, pinned Community/Enterprise source, project selection, ports, backups | [Environments](docs/ENVIRONMENTS.md) |
| Addons | Discovery, manifest/dependency inspection, scaffolding, hash-aware install/update, migration skeletons | [Modules](docs/MODULES.md) |
| Dependencies | Runtime Python packages, optional discovery, OCA queue/migration presets | [Development](docs/DEVELOPMENT.md#python-dependencies) |
| Runtime | Container commands, logs, Odoo/PostgreSQL shells, SQL, module state, debugger URLs | [Commands](docs/COMMANDS.md) |
| Tests | Module/test-tag selection and Odoo web-test preparation | [Modules](docs/MODULES.md#tests) |
| Browser | Page inspection, screenshots, traces, responsive MCP observation, approved CDN/map content | [Browser](docs/BROWSER.md) |
| AI | Local context, debugging reports, generated prompts, source/database/browser MCP tools | [AI Workflows](docs/ai/USING_ODOOCTL_WITH_AI.md) |
| CLI upkeep | Global settings, structured output, install-method-aware self-update | [Installation](docs/INSTALLATION.md) |

## Requirements and Compatibility

- CLI builds target Linux, macOS, and Windows; container workflows use Linux
  containers and need a working Docker daemon, Compose, and project bind mounts.
- Git and network access are needed to resolve Odoo source revisions and build
  images. Enterprise additionally requires authorized repository access.
- Go **1.25+** is needed to build the current source, not to run a downloaded
  binary. Host Node.js is not required for the Docker browser runtime.
- The current source catalogs **Odoo 12.0-20.0**, with **19.0** the default.
  Generated PostgreSQL is **15 for 12-19** and **16 for 20**.

Regenerating configuration does not migrate a database. Never attach PostgreSQL
16 to a PostgreSQL 15 data directory. See the
[environment and data boundaries](docs/ENVIRONMENTS.md#sources-and-versions).

**Availability:** newer MCP/browser and Odoo 20 capabilities described here
require a build containing those changes. A published binary, PPA package, or
`go install ...@latest` may not include the current local work. Check the selected
binary's help output; local fixture results are not release-availability evidence.

## Installation

For current development features, build the checkout containing them:

```bash
make build
./bin/odooctl --help
```

This creates `bin/odooctl` without replacing an older CLI on `PATH`. Use its
explicit path for unreleased commands; `make install` installs the checkout to
the Go binary directory.

For published releases, the Linux/macOS installer is:

```bash
curl -fsSL https://raw.githubusercontent.com/mart337i/odooctl/main/install.sh | bash
```

This executes a remote script and may use `sudo`. Manual release downloads,
Windows, Go installation, PPA availability, PATH, and checksum caveats are in
the [installation guide](docs/INSTALLATION.md).

Inspect before updating an installed CLI:

```bash
odooctl update --check
odooctl update
```

Updates affect the CLI, not Odoo databases. See
[update methods and rollback](docs/INSTALLATION.md#updates-and-rollback).

## Quick Start

Run project commands from an addons root containing your addon directories.
The examples below assume `odooctl` selects the intended binary.

```bash
cd /path/to/odoo-addons
odooctl docker create --odoo-version 19.0 --modules sale --without-demo

# Fresh development database: builds the image and initializes Odoo data.
odooctl docker run --init

odooctl docker status
odooctl docker open odoo
```

Creation generates files and pins source commits; initialization is an explicit
database-affecting step. The local development account is `admin` / `admin`.
Use the URL reported by status: port conflicts can change the initial allocation.

For an existing environment, start with `odooctl doctor --json` and confirm
`odooctl docker path --json` before initialization or updates. Plain `docker run`
can prompt to build and initialize on first use; `--no-prompt` suppresses prompts,
not all startup side effects. Follow the [development guide](docs/DEVELOPMENT.md)
for service, dependency, and reconfiguration behavior.

## Everyday Development

Inspect local changes before applying them:

```bash
odooctl docker status
odooctl docker logs --errors --since 10m
odooctl module manifest my_module --json
odooctl module changed --json
odooctl docker install --list-only --json
```

The last command previews ordinary local hash selection. Do not combine it with
`--compute-hashes` or `--update-all`, or assume external-only targets are dry runs.

After approving the database operation:

```bash
odooctl docker install my_module my_other_module
odooctl docker test --test-tags /my_module
odooctl docker stop
```

Positional install names are space-separated. Naming a local module does not
force an update: unchanged hashes are skipped. Hashes exclude `static/` and
`tests/`, so frontend/test-only changes need the appropriate reload or test
workflow. Tests can affect the selected database; use a disposable environment
when isolation matters. See [module installation and tests](docs/MODULES.md).

## AI and MCP

odooctl produces local context and exposes tools to your AI client; it does not
call an LLM provider itself.

```bash
odooctl ai context --module my_module --format json
odooctl ai debug-report --module my_module --include-logs
odooctl ai prompt debug --module my_module
```

Reports and logs can contain private data; redaction is best-effort. See
[AI workflows](docs/ai/USING_ODOOCTL_WITH_AI.md).

### Source and Database Inspection

Configure your MCP client to launch the project-scoped server:

```bash
odooctl mcp serve --project /absolute/path/to/odoo-addons
```

Default tools inspect modules and source; an existing environment also adds
status and bounded logs. SQL and ORM are explicit capabilities requiring
configured roles/users and allowlists. MCP SQL uses restricted reads in verified
read-only transactions; ordinary CLI SQL and Odoo shell are privileged interfaces
and do not inherit those guards.

Setup, grants, tool limits, and OpenCode/Claude Code configuration are in the
[MCP guide](docs/ai/MCP.md).

### Browser Inspection

| Mode | Best for |
| --- | --- |
| `odooctl browser ...` | Manual page checks, full-page captures, and traces using a browser-enabled Odoo image |
| `odooctl mcp serve --browser` | Bounded, local-only browser observation beside the source/database tools |
| `odooctl mcp browser` | Official Playwright MCP snapshots, responsive screenshots, console/network diagnostics, approved CDN assets and map frames |

Prepare the separate Playwright runtime explicitly for a running project:

```bash
odooctl mcp browser setup --project /absolute/path/to/odoo-addons \
  --allow-origin https://cdn.jsdelivr.net \
  --allow-origin https://www.openstreetmap.org \
  --allow-origin https://tile.openstreetmap.org
```

Then configure the client to launch `odooctl mcp browser --project PATH`. Both
authenticated MCP browser modes default to `admin` / `admin` for local development;
nonempty `ODOOCTL_MCP_BROWSER_LOGIN` and `ODOOCTL_MCP_BROWSER_PASSWORD` independently
override those values. Add `--public` to the separate browser mode to skip login.
Ordinary browser CLI commands instead use explicit login flags.

The separate runtime exposes observation tools, not clicks, typing, uploads, or
arbitrary code. Approved origins remain a trust decision: page loads can write
data, and screenshots can expose private content. WebSockets/service workers
are unavailable; this is not an unrestricted user browser.

Use the [browser comparison](docs/BROWSER.md) to choose a mode and the
[Playwright MCP guide](docs/ai/PLAYWRIGHT_MCP.md) for client setup, network policy,
artifact privacy, and verified Odoo 17-20 fixture coverage.

## Safety and Troubleshooting

- Confirm the project link, environment path, and database before writes.
  Switching Git branches alone does not switch the selected environment.
- Initialization, installs/upgrades, tests, dependency sync, and arbitrary
  SQL/shell execution can change data or services. JSON is an output format,
  not a read-only guarantee.
- `docker stop` retains containers and data. `docker reset` removes containers;
  `docker reset -v` deletes volumes, and `-c` removes generated configuration.
  Volume removal and `docker deps clean` require explicit data-loss approval.
- Diagnose before deleting data. Doctor may create a temporary bind-mount marker
  and an ephemeral probe container; it does not initialize Odoo.

```bash
odooctl doctor --json
odooctl docker status --json
odooctl docker logs --errors
odooctl ai debug-report --include-logs
```

Fix daemon, WSL/file-sharing, and bind-mount failures before retrying Odoo work.
See [debugging workflows](docs/ai/DEBUGGING_WORKFLOWS.md) and the
[documentation index](docs/README.md) for focused recovery steps.

## Contributing

Use Go 1.25+ and the local binary when developing:

```bash
go test ./...
go vet ./...
make build
```

[Contributing](CONTRIBUTING.md) covers formatting, race/live tests, vendoring,
and packaging. [Architecture](docs/ARCHITECTURE.md) explains state, source pins,
container design, and runtime boundaries. The
[agent version matrix](docs/ai/MCP_VERSION_MATRIX.md) records scoped Community
fixture evidence, not comprehensive addon, Enterprise, or production certification.

## Project

MIT licensed. [Repository](https://github.com/mart337i/odooctl) |
[Issues](https://github.com/mart337i/odooctl/issues) |
[Releases](https://github.com/mart337i/odooctl/releases)
