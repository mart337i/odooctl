# Command Reference

This is a family-level index of the registered CLI, not a replacement for its
Cobra schemas. Use `odooctl --help` and `odooctl FAMILY COMMAND --help` for the
options supported by your installed binary. Source may contain commands not yet
available in a release. `docker odoo-bin` passes flags directly to Odoo, so its
`--help` runs inside the container rather than being local Cobra help.

Run project commands from the intended addon project. MCP instead accepts an
explicit `--project PATH`. There is no registered `repository` command family;
managed addon repositories are configured through environment presets.

## Formats And Effects

Tables use **Inspect**, **Probe**, **Write**, **DB**, **Exec**, and **Delete** to
describe effects. Inspect reads state; Probe actively checks a runtime; Write
changes files, configuration, or services; DB can change database contents; Exec
runs arbitrary privileged code; Delete removes resources or data. Multiple
labels can apply. Browser observation also creates sessions and executes page
JavaScript, which can trigger application writes.

Most structured CLI output uses `--json`, only where registered. AI context and
debug reports use `--format json`; AI prompts are plain text. MCP servers use
stdio protocol messages, not CLI JSON. Advanced browser setup/stop always print
JSON and have no `--json` option.

**JSON is presentation, not a read-only guarantee.** Some individual commands
change behavior with JSON, but that does not generalize to other commands.
Review effects and flag combinations before automation.

## Root And Global Configuration

| Command | Purpose | Effects / format |
| --- | --- | --- |
| `doctor` | Diagnose project, Docker/Compose access, bind mounts, services, and dependencies. | Probe; `--json`. |
| `version` | Print CLI version; root also supports `--version`. | Inspect; `version --json`. |
| `update` | Plan and install an odooctl self-update. | Write/network; `--check` or `--dry-run` avoids installation; `--json` returns the plan without executing it. |
| `config show` | Show saved global settings. | Inspect; `--json`. |
| `config get <key>` | Read one global setting. | Inspect; `--json`. |
| `config set <key> <value>` | Save a global setting. | Write; `--json` still saves. |
| `config unset <key>` | Remove a global setting. | Write; `--json` still saves. |

Global keys are `ssh-key-path` and `github-token`. Global config show/get mask
the GitHub token in both text and JSON; JSON set results mask it too. The stored
token is still a credential, not a redacted file. Do not put actual tokens in
documentation, committed files, or copied shell history.

Self-update accepts `--method` (`auto`, `apt`, `release`, `go`, or `source`),
`--source-dir` for a source checkout, and `--force` for dev/dirty builds. It
updates the CLI, not Odoo modules or the project database.

Doctor is **not purely passive**: the bind-mount check creates a temporary marker
and runs an ephemeral Alpine container, potentially pulling its image. It does
not initialize the Odoo database. A present Docker client does not establish a
healthy daemon, accessible mounts, or working network.

## Docker Environments

All commands below have the `odooctl docker` prefix.

| Command | Purpose | Effects / format |
| --- | --- | --- |
| `create` | Generate and link a development environment. | Write; `--json` available. |
| `reconfigure` | Add packages/addon paths/presets or change browser tooling. | Write; stops/rebuilds by default. |
| `run` | Start containers, detached by default. | Write; optional initialization is DB-affecting. |
| `stop` | Stop environment containers. | Write; `--json`. |
| `restart [service...]` | Restart selected services; defaults to Odoo only. | Write; `--json`. |
| `status` | Report Compose service status. | Inspect; `--json`. |
| `logs [service]` | Read logs, optionally filter or follow. | Inspect; `--json` cannot combine with `--follow`. |
| `path` | Show environment location and status. | Inspect; `--json`. |
| `debug-info` | Show URLs, database name, config paths, debugger details. | Inspect; `--json`. |
| `open [odoo\|mailhog\|debug]` | Open/print a development endpoint. | Launches host browser; `--json` only reports the endpoint. |
| `goto` | Select a project, enter a shell there, optionally check out its branch. | Shell/Git changes; `--json` only lists projects. |
| `edit [file]` | Open environment configuration in an editor. | Write. |
| `preset list` | List built-in environment presets. | Inspect; `--json`. |
| `dump` | Archive database and filestore. | Writes a sensitive backup; `--output`, `--json`. |
| `reset` | Remove containers, optionally volumes/config files. | Delete; `--json` is not a dry run. |

`create` supports version/name, module selection, Enterprise, demo-data choice,
pip packages, repeated addon paths and presets, dependency discovery, and
`--browser`. Use help for exact options; it prepares configuration rather than
implicitly initializing a database.

`run` normally starts containers. On first use it can prompt to build images and
initialize the database. `--no-prompt` suppresses those prompts, not other startup
effects. `--build` rebuilds; `--init` (`-i`) builds and initializes before Odoo
starts. Startup can also prepare managed repositories, synchronize dependencies,
and rewrite configuration when ports conflict.

`reconfigure --browser --rebuild` prepares in-Odoo browser support for Odoo
15.0+. `--no-browser` disables it in generated configuration. Reconfiguration
defaults to `--rebuild=true` and `--stop-first=true`; review service disruption.

`logs` supports `--tail`, `--since`, `--grep`, and `--errors`. Logs may contain
secrets or business data. Do not assume ordinary logs or environment config
output receives the AI debug report's redaction.

`reset --volumes` (`-v`) deletes database and filestore volumes. `--files` (`-c`)
removes configuration files; `--force` (`-f`) skips confirmation. Require explicit
data-loss approval for volume removal, including combined `-vc`.

## Docker Execution And Dependencies

These also have the `odooctl docker` prefix.

| Command | Purpose | Effects / format |
| --- | --- | --- |
| `compose -- <args...>` | Raw Compose in the generated environment directory. | Exec; effects depend on arguments. |
| `exec <service> -- <command...>` | Run a service command. | Exec; `--root`, `--no-tty` (`-T`). |
| `shell [service]` | Bash (default service `odoo`) or `--odoo` Python shell. | Exec/DB; `--root`, `--service`. |
| `odoo-bin [args...]` | Pass arguments directly to container Odoo. | Exec/DB; Odoo owns its flags. |
| `db` | Open interactive PostgreSQL shell. | Exec/DB; optional `--database`. |
| `sql [query]` | Execute SQL text or `--file FILE`. | Exec/DB; `--database`, `--json`. |
| `install [modules...]` | Hash-aware module install/update. | DB/services/dependencies; JSON is intended for limited listing mode. |
| `test` | Run Odoo tests selected by modules or test tags. | DB/services; not read-only inspection. |
| `deps scan [modules...]` | Inspect manifest Python dependencies. | Inspect; `--modules`, `--json`. |
| `deps list` | List recorded environment Python dependencies. | Inspect; `--json`. |
| `deps sync [packages...]` | Install into the runtime dependency volume. | Write/services; `--modules` when packages omitted. |
| `deps clean` | Remove runtime Python dependencies. | Delete; explicit operator approval. |

Shell, eval, raw Compose, SQL and Odoo execution are privileged development
interfaces, not sandboxed inspection tools. CLI SQL uses the application database
role. Its `--json` wraps a query for JSON rows; there is **no read-only transaction
guard or restricted grammar**, and SELECT functions can have side effects.
The separate MCP SQL interface has different controls: see
[MCP SQL inspection](ai/MCP.md#sql-inspection) and [CLI source](../cmd/docker/sql.go).

Install without arguments detects changed local modules; `all` selects all local
modules and quoted patterns such as `'sale_*'` select local matches. Explicitly
naming a local module does **not** bypass unchanged-hash skipping. External/core
names go directly to Odoo without that hash check. `module upgrade` delegates to
this behavior, rather than forcing every named local module to update.

Use `docker install --list-only --json` for the ordinary local change preview,
without `--compute-hashes` or `--update-all`. Do not treat listing as a universal
dry run: the current listing branch requires local targets, so external-only
targets can fall through to installation. `--compute-hashes` takes precedence
and writes saved hashes; `--update-all` takes precedence and runs `-u base` in
text mode even with `--list-only`. Its JSON branch only reports that mode.
See [install source](../cmd/docker/install.go) before combining these flags.

Install may synchronize missing Python dependencies, stop Odoo, run install or
upgrade, restart Odoo, and save hashes. `--deps-mode` selects `runtime` or `fail`
(default `fail` when `CI=true`); `--skip-deps` skips scanning. Neither installing
nor testing is database-neutral. `test` supports `--modules`, `--test-tags`,
`--log-level`, and `--web`; web mode checks browser readiness and defaults tags
to `/web` when none are supplied.

## Local Modules And Odoo Runtime

| Command | Purpose | Effects / format |
| --- | --- | --- |
| `module list` | Discover local modules. | Inspect; `--json`. |
| `module manifest <module>` | Inspect parsed manifest. | Inspect; `--json`. |
| `module deps [modules...]` | Report manifest dependencies. | Inspect; `--json`. |
| `module changed` | Compare local hashes with last install. | Inspect; `--json`. |
| `module scaffold <name>` | Create addon files. | Write; `--json`. |
| `module test <module...>` | Run module tests; optional `--test-tags`. | DB/services. |
| `module upgrade <module...>` | Delegate to Docker install/update. | DB/services; local hash skipping still applies. |
| `module migrate plan <module>` | Print a checklist, not perform a migration. | Inspect; `--from`, `--to`, `--json`. |
| `module migrate scaffold <module>` | Create pre/post migration skeletons. | Write; requires `--to`, supports `--json`. |
| `odoo module-state <module...>` | Query installed/available module state. | Runtime inspection; `--json`. |
| `odoo update-apps` | Refresh Odoo's module catalog. | DB write. |
| `odoo shell` | Open Odoo Python shell. | Exec/DB. |
| `odoo eval <python expression>` | Evaluate Python in Odoo shell context. | Exec/DB; quote as one argument. |

Scaffolding supports `--author`, `--odoo-version`, `--depends`, `--description`,
and `--model`. Migration scaffold `--to` names the module migration version
directory, not an automatic Odoo major-version conversion. Existing skeletons
are retained. Manifest inspection is lightweight parsing, not arbitrary Python
execution or proof that every computed manifest value is understood.

## AI And Browser

| Command | Purpose | Effects / format |
| --- | --- | --- |
| `ai context` | Project/environment context, optionally `--module`. | Probe; `--format markdown\|json`. |
| `ai debug-report` | Diagnostic report; optional module, logs, browser check, output file. | Probe; file write with `--output`; `--format markdown\|json`. |
| `ai prompt debug` | Generate debugging prompt, optionally `--module`. | Probe; plain text. |
| `browser doctor` | Launch-check Python Playwright Chromium. | Probe; `--json`. |
| `browser inspect [path-or-url]` | Report page text, console errors, failed requests. | Browser/session effects; `--json`. |
| `browser snapshot [path-or-url]` | Return visible page text. | Browser/session effects; `--json`. |
| `browser check [path-or-url]` | Require visible `--expect-text TEXT`. | Browser/session effects; `--json`. |
| `browser screenshot [path-or-url]` | Save full-page PNG. | Browser/session effects and file write; `--output`, `--json`. |
| `browser trace [path-or-url]` | Save Playwright trace ZIP. | Browser/session effects and file write; `--output`, `--json`. |

Page commands share explicit `--login`/`--password`, millisecond `--timeout` and
`--wait`, and viewport `--width`/`--height`. These are not MCP credential defaults.
See [Browser Guide](BROWSER.md) for runtime preparation and artifact privacy.
AI debug reports redact common secret patterns, not every possible secret;
review output before sharing. See [AI workflows](ai/USING_ODOOCTL_WITH_AI.md) and
[debugging workflows](ai/DEBUGGING_WORKFLOWS.md).

## MCP

| Command | Purpose | Effects / format |
| --- | --- | --- |
| `mcp serve` | Project-scoped source/runtime inspection over stdio. | Capability-dependent; `--project` defaults to `.`. |
| `mcp browser` | Separate approved-origin Playwright observer over stdio. | Starts session worker; browser/session effects; optional `--public`. |
| `mcp browser setup` | Build worker/proxy images and start proxy. | Network/images/files/services; repeated `--allow-origin`; always JSON. |
| `mcp browser stop` | Remove managed proxy/private network after workers close. | Services; retains Odoo data/images/files/artifacts; always JSON. |

`mcp serve` adds strict browser tools with `--browser`, container source with
`--container-source`, restricted SQL with paired `--sql-role`/`--sql-tables`,
and ORM with paired `--odoo-user-id`/`--models` plus optional `--company-ids`.
It does not provision roles or start Odoo. Read [MCP](ai/MCP.md) for grants,
transaction limits, source boundaries, and client configuration. Advanced browser
requires separate preparation; see [Playwright MCP](ai/PLAYWRIGHT_MCP.md).
