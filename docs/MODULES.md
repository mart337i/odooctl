# Modules

Use module helpers to inspect and scaffold addons, select install/update work,
and prepare migration files. Runtime operations use the environment linked to
your project root; verify it with `docker path` and `docker debug-info` first.
See [Development](DEVELOPMENT.md) for daily work and
[Environments](ENVIRONMENTS.md) for lifecycle and isolation boundaries.

## Discover and Inspect

```bash
odooctl module list --json
odooctl module manifest my_module --json
odooctl module deps my_module --json
odooctl module changed --json
odooctl odoo module-state my_module --json
```

| Inspection | What it tells you |
| --- | --- |
| `module list` | Addons found on disk in scanned paths |
| `module manifest` | Parsed manifest metadata for the named addon |
| `module deps` | Manifest `depends` and external Python requirements |
| `module changed` | New/changed local hashes relative to saved hashes |
| `odoo module-state` | Runtime module state from the selected database |

An addon directory is recognized by `__manifest__.py`. Discovery looks at
immediate children of each scan root, not an arbitrary recursive tree.
With environment state, module inspection scans the project root, managed
preset addon paths, and configured extra addon paths. Without state it falls
back to the detected project root. Hash-based change detection requires state
and covers only local modules directly under `state.ProjectRoot`.

Disk discovery is not proof of installation, and a stored hash is not a live
database-state check. `module deps` prints declarations; it does not install
dependencies or prove that their Python imports are available.

## Scaffold an Addon

Scaffolding writes a new directory relative to your current working directory.
Choose an addons root that the intended environment mounts:

```bash
odooctl module scaffold my_module --odoo-version 18.0 --model \
  --author "My Company" --depends sale,purchase
```

Names must start with a lowercase letter, followed by lowercase letters,
digits, or underscores. An existing directory is rejected.
`--depends` takes CSV; the default is `base`. Pass the version explicitly when
the branch context differs from the selected environment.

| Generated content | Without `--model` | With `--model` |
| --- | --- | --- |
| `__manifest__.py`, `__init__.py` | Yes | Yes |
| `static/`, `data/`, `security/` | Yes | Yes |
| `models/` and model imports | No | Yes |
| Model view/action/menu XML | No | Yes |
| `security/ir.model.access.csv` | No | Yes |

The model name replaces underscores with dots: `my_module` becomes
`my.module`. Model view templates use `<tree>` through Odoo 17 and `<list>`
from Odoo 18 onward, using an integer major-version parser that also handles
20.0. Review the manifest, security, fields, and view/action behavior before
installing; generated boilerplate is a starting point, not a completed addon.
Scaffolding does not install or upgrade anything in a database.

## Select Install Work

`docker install` has two paths: hash-filtered local addons and named external
addons passed directly to Odoo. Use space-separated positional arguments:

```bash
odooctl docker install --list-only --json
```

This is the ordinary preview for local change selection, not a general-purpose
dry-run flag applicable to every combination or external-only invocation.
Keep the inspection command above separate from mutating modes.

**Database writes and service interruption:** after reviewing the target:

```bash
odooctl docker install
odooctl docker install my_module my_other_module
odooctl docker install 'sale_*'
odooctl docker install all --ignore=experimental_module,legacy_module
odooctl docker install sale purchase
```

| Selection | Behavior |
| --- | --- |
| No positional arguments | Checks all local candidates for new/changed hashes. |
| Named local module | Still skips an unchanged hash; naming is not forcing. |
| `all` as the sole argument | Selects all local candidates, still hash-filtered. |
| Quoted wildcard | Expands against local addon names. |
| Named nonlocal addon | Passed as an install target without local hash checking. |
| `--ignore=a,b` | Filters selected names; CSV is supported for this flag. |
| `--ignore-core` | Suppresses nonlocal positional targets. |

Do not use `my_module,my_other_module` as a positional local selection.
That is a single argument, not two names for local hashing.
Quote wildcards so your shell does not expand filesystem names first.
An addon on an extra addons path is nonlocal for this command, even if
`module list` discovers it. A misspelled local name can also be treated as an
external target rather than producing a local selection error.

## Hashes and Explicit Updates

SHA256 combines sorted relative file paths and contents, then compares the
result with the environment's `module-hashes.json`.
No saved hash selects a local module for installation; a different hash selects
an update. Equal hashes select neither.

| Excluded from hashes | Consequence |
| --- | --- |
| `static/` | JS, CSS, and static XML edits alone do not trigger install/update. |
| `tests/` | Test-only edits do not trigger install/update. |
| `__pycache__/`, `*.pyc`, `*.pyo`, `*.pyd` | Compiled Python artifacts do not count. |
| `i18n/*.pot`, `i18n_extra/*.pot`, `.git/` | Template/catalog metadata and Git internals do not count. |

`module upgrade my_module` delegates to `docker install my_module`. It retains
the same local hash checks; it is not a force-update escape hatch.
There is no `docker install --force` option.

For asset-only edits, use the asset reload procedure tested for your Odoo
version and inspect the resulting browser behavior. If you need an explicit
module update regardless of hashes, use a controlled direct Odoo operation.
For example, after backup and approval of the database write, stop the normal
Odoo service and run a one-shot update against the verified database name:

```bash
odooctl docker compose stop odoo
odooctl docker compose run --rm odoo odoo -c /etc/odoo/odoo.conf \
  -d odoo-180 -u my_module --stop-after-init
odooctl docker compose up -d odoo
```

`odoo-180` is only an example for 18.0; obtain the actual database from
`docker debug-info`. Ensure Odoo is restarted even if the update fails, then
inspect logs. This bypasses wrapper hash/dependency handling and does not
refresh its saved hashes. Do not substitute a broad upgrade for an asset reload
without checking the version-specific requirements.

`docker odoo-bin` forwards arguments via `exec` in the running Odoo container;
it does not stop the serving process or add the target database for you.
Prefer the controlled one-shot procedure for an explicit database update.

`docker install --update-all` runs `-u base`, bypassing ordinary hash selection,
and stops/restarts Odoo. Treat it as a broad database upgrade, not daily reload.
`--update-all` is handled before `--list-only`: non-JSON use can upgrade despite
the listing flag. `--compute-hashes` writes stored hashes before the ordinary
list branch and can mark code as known without applying it to the database.
Do not combine these modes with a preview or use them to infer a safe dry run.

## Python Requirements

```python
{
    'depends': ['base'],
    'external_dependencies': {'python': ['requests', 'pandas']},
}
```

`docker deps scan` scans configured addon roots and compares declarations with
the state's configured packages. `docker deps list` reports that configuration
and sync metadata, not the actual pip inventory. A configured result does not
prove that a volume still exists or an import succeeds.

Install scans selected new/changed local modules by default. Missing packages
are pip-installed into persistent `/opt/odoo-extra-python` in `runtime` mode;
`fail` mode reports them without installation. The default is runtime locally,
fail with `CI=true` or `CI=1`. Install supports only runtime/fail, not build mode.
Image build dependencies remain separate from this runtime package mechanism.
Explicit `docker deps sync requests` can reinstall a configured package;
`deps clean` deletes package data and requires data-loss approval.

## Tests

**Database-affecting:** both test wrappers use `state.DBName()` and shared
environment volumes in a one-off `docker compose run --rm odoo` container.
Removing that container does not isolate or reset the development database.
These are Odoo/PostgreSQL runtime tests, not SQLite fixtures or a disposable
test-database guarantee. Use a dedicated environment when isolation matters.

```bash
odooctl module test my_module my_other_module
odooctl docker test --modules my_module,my_other_module
odooctl docker test --test-tags /my_module:TestMyClass
odooctl docker test --test-tags /my_module:TestMyClass.test_method
odooctl docker test --test-tags 'standard,-slow'
odooctl docker test --test-tags /my_module --log-level=test:DEBUG
```

| Test option | Actual effect |
| --- | --- |
| `module test a b` | Defaults to tags `/a,/b`; does not add `-i`. |
| `module test --test-tags ...` | Replaces those default module tags. |
| `docker test --modules a,b` without tags | Adds `-i a,b`; module installation can write data. |
| `docker test --modules a,b --test-tags ...` | Uses the supplied tags; modules are context only, not an additional filter or install. |
| `docker test --web` | Checks Chromium readiness and defaults tags to `/web` if omitted. |

The flag is `--test-tags`. Its syntax is `[-][tag][/module][:class][.method]`.
Use `/module` in the tags when you need module filtering. Tags alone do not
install an absent addon, and module context with tags does not guarantee that
the requested tests are scheduled. Check output for the expected cases.
Unfiltered `docker test` prompts before attempting all tests.

## Migration Helpers

```bash
odooctl module migrate plan my_module --from 18.0 --to 19.0 --json
odooctl module migrate scaffold my_module --to 19.0.1.0.0 --json
```

Plan prints a generic checklist; version arguments label the report rather
than performing compatibility analysis. Scaffold writes `pre-migration.py`
and `post-migration.py` with `def migrate(env, version): pass` beneath
`migrations/<target-module-version>/`, preserving existing files.
These commands do not port code, change the manifest, execute migration scripts,
or upgrade a database. Review the generated signature against the framework
that will execute it and implement required data transformations yourself.
The migration preset prepares tooling, not an automatic major-version port
or an OCA compliance certification. Validate code and data on a backed-up copy.

## Further Reading

- [AI usage](ai/USING_ODOOCTL_WITH_AI.md), [debugging workflows](ai/DEBUGGING_WORKFLOWS.md), and [MCP](ai/MCP.md).
- [Commands](COMMANDS.md), [Browser](BROWSER.md), [Installation](INSTALLATION.md), [Architecture](ARCHITECTURE.md), and the [documentation index](README.md).
