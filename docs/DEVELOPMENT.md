# Development Workflow

Use this guide for daily addon development in an odooctl-managed environment.
For environment identity, source pins, ports, and cleanup, see
[Environments](ENVIRONMENTS.md). For scaffolding, hash selection, tests, and
migration helpers, see [Modules](MODULES.md).

## Check the Target

Run commands from your addon project. Before starting or updating anything,
inspect the selected environment:

```bash
odooctl doctor --json
odooctl docker path --json
odooctl docker status --json
odooctl docker debug-info --json
odooctl module list --json
```

The project link selects one environment for a project root. Checking out a
different Git branch does not automatically select another environment.
Verify the environment path and database before any database-affecting command.
URLs may change when startup resolves port conflicts; use status rather than a
hard-coded localhost URL.

## Create and Initialize

From a directory containing your addon directories:

```bash
odooctl docker create --odoo-version 18.0 --modules sale,stock
```

Creation writes Compose, Dockerfile, Odoo configuration, state, and the global
project link. It resolves and pins the Odoo Git revision, so Git and network
access are needed even though no image is built yet. Enterprise source also
requires authorized access when `--enterprise` is selected.

Creation does not scan manifests for Python dependencies by default.
`--auto-discover-deps` explicitly enables discovery; `--pip` records packages
or reads a requirements file. Presets may also prepare managed Git checkouts.

**Database write:** use initialization for a fresh development database:

```bash
odooctl docker run --init
odooctl docker status
```

Initialization builds the Odoo image, starts infrastructure, runs the one-shot
init service with `base`, `web`, and configured modules, writes `report.url`,
then starts Odoo. State records the first build and initialization timestamps.
Do not use `--init` as an ordinary restart or an assumed database reset.

| Startup command | Behavior |
| --- | --- |
| `docker run` | Starts services; on first use prompts to build and initialize, both defaulting to yes. |
| `docker run --build` | Builds before starting; an uninitialized environment can still prompt for initialization. |
| `docker run --init` | Builds and executes database initialization before starting Odoo. |
| `docker run --no-prompt` | Skips the interactive build/init questions; does not make startup read-only. |

Even with `--no-prompt`, startup can prepare repositories, regenerate files and
save new ports, and synchronize configured Python packages. Explicit `--build`
and `--init` still take effect. Use explicit choices for automation, not a
prompt-suppression flag as a safety boundary.

## Daily Loop

For an already initialized environment:

```bash
odooctl docker run
odooctl docker logs -f
odooctl docker open odoo
odooctl docker open mailhog
```

Project addons are bind-mounted into `/mnt/extra-addons`. Editing addon files
does not normally require rebuilding the image. Apply the operation matching
the change:

| Change | Typical action |
| --- | --- |
| Python implementation | Restart Odoo to load code; update if model/schema/data changes require it. |
| Models, manifest, XML, security, data | Preview selection, then install/update the affected modules. |
| Only `static/` assets | Use a version-appropriate asset reload and verify the browser; hash selection will not detect these files. |
| Only `tests/` | Run the intended tests explicitly; these files are excluded from module hashes. |
| External Python dependency | Scan and sync packages; then restart affected processes. |
| Image tooling or browser support | Reconfigure and rebuild, allowing for service interruption. |

Preview ordinary local changes without upgrading:

```bash
odooctl module changed --json
odooctl docker install --list-only --json
```

**Database write and service interruption:** install/update only after reviewing
the target and selection:

```bash
odooctl docker install
odooctl docker install my_module my_other_module
```

Positional module names are space-separated. Naming a local module does not
force an update: unchanged hashes are skipped. Likewise, `install all` selects
all local candidates, not an unconditional upgrade. Quote patterns such as
`'sale_*'` so odooctl, rather than the shell, expands them.

The install command stops Odoo, runs a one-shot install/update against the
environment database, and attempts to restart Odoo even after failure.
It can also install missing Python dependencies before the module operation.
See [Modules](MODULES.md) for hash exclusions and explicit update options.

## Container Tools

Use wrappers to avoid locating the generated Compose directory manually:

```bash
odooctl docker exec odoo -- python --version
odooctl docker compose ps
odooctl docker compose -- ps --services
odooctl docker logs --errors
odooctl docker logs --grep Traceback --since 10m
```

Restarting is a service interruption, not a module upgrade:

```bash
odooctl docker restart
odooctl docker restart odoo
```

Interactive tools can execute mutating commands. Confirm the database first:

```bash
odooctl docker shell
odooctl odoo shell
odooctl docker db
```

`docker exec --root`, raw Compose, SQL, `odoo eval`, and Odoo shell expose the
underlying runtime directly. They are not inspection-only APIs.
`odooctl odoo update-apps` also writes the database by refreshing its module list.

## Python Dependencies

Declare addon requirements in `external_dependencies['python']`, then inspect:

```bash
odooctl docker deps scan --modules my_module --json
odooctl docker deps list --json
```

These reports compare manifests with configured packages, not a live pip audit.
For approved package installation into the persistent runtime volume:

```bash
odooctl docker deps sync
odooctl docker deps sync requests pandas
odooctl docker restart odoo
```

Packages live at `/opt/odoo-extra-python`, on `PYTHONPATH`, separately from the
image's `/opt/odoo-venv`. Explicit package arguments can be used to resynchronize
a package already recorded in state. Sync uses pip with `--upgrade` and writes
dependency state; it is not a read-only check.

Install defaults to `--deps-mode runtime` locally, or `fail` with `CI=true`
(also `CI=1`). Failure mode reports missing configured requirements rather than
installing them. `--skip-deps` or `--auto-install-deps=false` bypasses scanning;
neither proves that the runtime can import the required packages.

## Reconfigure

**Service interruption and configuration/package writes:** reconfigure defaults
to `--stop-first=true` and `--rebuild=true` when changes are applied.
It takes services down, regenerates files, synchronizes added packages, saves
state, builds, and prompts to start services again.

```bash
odooctl docker reconfigure --add-addons-path ~/external-addons
odooctl docker reconfigure --add-pip ./requirements.txt
```

`--rebuild=false --stop-first=false` disables those two actions, not all side
effects: configuration, repository preparation, and package sync can still
occur. Arrange any required service recreation yourself afterward.

## Test and Diagnose

**Database-affecting:** tests use the selected environment database and shared
volumes, even though the test process runs in a disposable container.
Use a dedicated disposable environment for tests that must not affect daily data.

```bash
odooctl module test my_module
odooctl docker test --test-tags /my_module:TestMyClass
```

See [Modules](MODULES.md) for the distinction between `--modules` and test tags.
Browser-enabled environments can run `docker test --web --test-tags /web`;
browser tooling requires Odoo 15.0+ and image preparation.

For failures, inspect before rebuilding or deleting data:

```bash
odooctl doctor --json
odooctl docker logs --errors
odooctl ai debug-report --module my_module --include-logs
```

Fix Docker bind-mount/file-sharing failures before attempting Odoo updates.
Redact secrets and personal data from logs and reports you share.
Stop for the day with `odooctl docker stop`; it retains containers and volumes.
See [Environments](ENVIRONMENTS.md) before backup or destructive cleanup.

## Further Reading

- [AI usage](ai/USING_ODOOCTL_WITH_AI.md) and [debugging workflows](ai/DEBUGGING_WORKFLOWS.md).
- [MCP setup](ai/MCP.md), [Playwright MCP](ai/PLAYWRIGHT_MCP.md), and [version matrix](ai/MCP_VERSION_MATRIX.md).
- [Commands](COMMANDS.md), [Browser](BROWSER.md), [Installation](INSTALLATION.md), [Architecture](ARCHITECTURE.md), and the [documentation index](README.md).
