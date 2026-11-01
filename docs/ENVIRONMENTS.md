# Environments

An environment combines generated configuration, pinned source revisions,
Compose resources, and a link from an addon project root.
Use [Development](DEVELOPMENT.md) for the daily loop and [Modules](MODULES.md)
for addon installation and testing.

## Identity and Lookup

Environment files live at `~/.odooctl/<project>/<environment>/`.
The state's `branch` field supplies the environment directory name, even for
non-Git projects where it is a user-selected name rather than a branch.

| Context at creation | Project name | Environment name |
| --- | --- | --- |
| Git repository | Detected repository name | Sanitized current Git branch |
| Git repository with `--name` | `--name` overrides the project name | Still the Git branch |
| Non-Git directory | Directory-derived project name | Defaults to that project name |
| Non-Git directory with `--name` | Directory-derived project name | `--name` sets the environment name |

The `--name` flag is not a uniform environment selector across both contexts.
Creating an existing project/environment pair is rejected; use reconfigure to
change settings rather than deleting data merely to alter configuration.

Version detection uses the Git branch first, then `.odooversion` at the root,
then `ODOO_VERSION`. `--odoo-version` overrides detection. If no version is
found, creation prompts; the version catalogue default remains 19.0.
Use an explicit version in repeatable setup instructions.

```bash
odooctl docker create --odoo-version 17.0 --modules sale,stock
```

New project lookup links are global JSON files under `~/.odooctl/projects/`,
keyed by a hash of the absolute project root. No repo-local marker is created.
There is one link per root, not one automatically selected link per Git branch.
Creation and commands that save a link can replace that root's selected target.

Before starting or mutating an environment:

```bash
odooctl docker path --json
odooctl docker status --json
odooctl docker debug-info --json
```

Switching Git branches changes mounted source files, not the linked environment
or its Odoo version. Creating on another branch can produce a separate
environment and update the root link, but does not stop the old one.
Do not assume every stored environment is active, or that branch checkout
provides an instant environment switch. Verify the selected target explicitly.
Concurrent environments need separate ports and care around shared source.

## Files and State

| Location | Purpose |
| --- | --- |
| `docker-compose.yml` | Services, mounts, images, ports, and volume definitions |
| `Dockerfile` | Odoo source build and baseline runtime/tooling |
| `odoo.conf` | Generated Odoo runtime configuration |
| `.odooctl-state.json` | Source pins, version, identity, paths, ports, and lifecycle metadata |
| `module-hashes.json` | Local module hashes used by install/change detection |
| `repositories/` | Managed preset Git checkouts |
| `browser-artifacts/` | Browser outputs when tooling is enabled |

State includes Community and optional Enterprise commit SHAs, schema version,
init modules, pip packages, dependency sync metadata, preset expansion, and
first-build/first-initialization timestamps. Timestamps are bookkeeping, not a
live proof that images or database volumes still exist.

Generated files can be overwritten by regeneration. Prefer supported create
or reconfigure settings for persistent configuration changes.
State and global `~/.odooctl/config.json` may contain Enterprise credentials;
protect them and redact them before sharing. They are written owner-only.

## Sources and Versions

Creation resolves the selected Odoo branch through Git and records its exact
commit. It needs Git/network access before any Docker build. Later builds use
the stored SHA rather than silently following the current branch head.
Older environments without pins can resolve them when building or initializing.

The image builds Community from `odoo/odoo`, and optional Enterprise from
`odoo/enterprise`, rather than consuming an official prebuilt Odoo image.
Enterprise uses the same version branch and needs checkout access.
Build authentication uses an SSH agent or build secrets for an SSH key/token;
credentials are not copied into the image.

| Odoo version | Generated PostgreSQL image |
| --- | --- |
| 12.0-19.0 | `postgres:15` |
| 20.0 | `postgres:16` |

20.0 is an explicit version in the Go catalogue and generated templates, not
an alias for 19.0. Specify `--odoo-version 20.0` for that target.
The default remains 19.0; catalogue/template support is not a guarantee that
every addon or application works on the target version.
See the scoped [MCP version matrix](ai/MCP_VERSION_MATRIX.md) for its evidence.

Regeneration does not migrate PostgreSQL data. Never attach a PostgreSQL 15
data directory to PostgreSQL 16. Plan and verify a manual database migration or
restore into a new compatible volume; retain the old data until validation.
Odoo major-version data migration is a separate concern from PostgreSQL upgrade.

## Images and Resources

Odoo code resides at `/opt/odoo-src`; the Python virtual environment and baseline
developer tools live at `/opt/odoo-venv`. Version-specific build dependencies
come from pinned requirements or the matching Debian packages for Odoo 12/13.
The image includes PostgreSQL client tools, PDF tooling, debugpy, and IPython.

Stable dependency layers precede generated configuration layers; BuildKit
caches pip/npm downloads. Local addons are mounted, not baked into the image.
Module Python packages live in a persistent volume at `/opt/odoo-extra-python`,
added to `PYTHONPATH`, so ordinary package changes need not rebuild Odoo.

New environments record `docker_schema_version: 2` and use Compose-scoped
images, networks, and volumes. The Compose name incorporates project,
environment, and version. The database name is version-derived, for example
`odoo-170`; resource scoping, not a unique SQL name, isolates databases.

Legacy state without a schema version is normalized to schema 1. Regeneration
preserves legacy globally named volumes rather than silently abandoning data.
Those names can be shared by old same-version environments. Inspect generated
volume mappings before assuming isolation or removing anything.
Moving legacy data to scoped resources requires an explicit, verified migration;
regeneration alone does not copy data or upgrade PostgreSQL.

## Ports and URLs

For major version `M`, initial ports are:

| Service | Formula | Odoo 17.0 example |
| --- | --- | --- |
| Odoo HTTP | `8000 + M * 100` | 9700 |
| MailHog UI | `8000 + M * 100 + 25` | 9725 |
| SMTP | `1000 + M * 100 + 25` | 2725 |
| Debug attach | `5000 + M * 100 + 78` | 6778 |

| Version | Odoo | MailHog | SMTP | Debug |
| --- | --- | --- | --- | --- |
| 12.0 | 9200 | 9225 | 2225 | 6278 |
| 13.0 | 9300 | 9325 | 2325 | 6378 |
| 14.0 | 9400 | 9425 | 2425 | 6478 |
| 15.0 | 9500 | 9525 | 2525 | 6578 |
| 16.0 | 9600 | 9625 | 2625 | 6678 |
| 17.0 | 9700 | 9725 | 2725 | 6778 |
| 18.0 | 9800 | 9825 | 2825 | 6878 |
| 19.0 | 9900 | 9925 | 2925 | 6978 |
| 20.0 | 10000 | 10025 | 3025 | 7078 |

Startup checks all four ports. On conflict it tries sets offset by 10, up to
nine increments beyond the base, then falls back to the base if none is free.
It regenerates configuration and saves chosen ports. Resolution is bounded,
not a guarantee of available ports. Read actual URLs and debugger settings
from `docker status`, `docker open`, and `docker debug-info` after startup.

## Presets and Reconfiguration

Presets are opt-in expansions of explicit state, not automatic migration tools.
There are two built-in presets:

| Preset | Expansion |
| --- | --- |
| `queue-job` | OCA queue checkout; `queue_job` init/server-wide module; 2 workers, 1 cron thread, `root:4` channels |
| `migration` | OCA OpenUpgrade addons, Odoo upgrade-util checkout, and `openupgradelib` |

```bash
odooctl docker preset list
odooctl docker create --odoo-version 18.0 --preset queue-job
```

Managed repositories have their own pinned commits and require Git checkout
access to the requested branch/commit. They live inside the environment and
are mounted read-only. Preset availability does not prove upstream branch
availability, migration completeness, or OCA certification.

**Mutating operation:** `docker reconfigure --preset migration` prepares
checkouts and applies configuration/packages to the selected environment.
When changes apply, reconfigure defaults to stopping services and rebuilding,
then prompts to start them. Adding init modules to state does not itself run
a module installation in an already initialized database.
See [Development](DEVELOPMENT.md) for flags and dependency side effects.

## Backup and Cleanup

With containers running, create a database/filestore artifact before destructive
work. Choose a new output file to avoid overwriting an existing archive:

```bash
odooctl docker dump --output ./before-upgrade.zip
```

The ZIP contains `database.sql` and `filestore/`. It is not a backup of source,
configuration, credentials, or installed Python packages. Database and
filestore are captured separately; arrange a quiet write window and verify a
restore before relying on it. Archives can contain passwords, personal data,
and attachments. Store them securely and outside a directory you will delete.

| Command | Retained or removed |
| --- | --- |
| `docker stop` | Stops services; retains containers, volumes, and files. |
| `docker reset` | Removes containers/orphans and Compose network; retains volumes and environment files. |
| `docker reset -v` | Also deletes Compose volumes, including database, filestore, sessions, and runtime packages. |
| `docker reset -c` | Also deletes the selected environment directory and project link; volumes remain without `-v`. |
| `docker reset -v -c` | Deletes volumes and environment files. |
| `docker deps clean` | Deletes installed runtime Python packages and clears sync metadata, retaining configured package names. |

Require explicit approval for data loss before `reset -v`, file deletion, or
`deps clean`. `--force` on reset only skips confirmation, not deletion.
Destructive JSON reset requires `--force`; JSON output is not a dry run.
Legacy shared volumes need particular care. Do not use reset as the first
response to a startup failure; inspect doctor and logs first.

## Further Reading

- [AI usage](ai/USING_ODOOCTL_WITH_AI.md), [MCP](ai/MCP.md), and [Playwright MCP](ai/PLAYWRIGHT_MCP.md).
- [Commands](COMMANDS.md), [Browser](BROWSER.md), [Installation](INSTALLATION.md), [Architecture](ARCHITECTURE.md), and the [documentation index](README.md).
