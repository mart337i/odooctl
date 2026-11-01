# Architecture

odooctl is a Go CLI that generates and operates Docker development environments
for Odoo addons. It is not a production deployment system or database migration
engine. Start with the [project README](../README.md); browse detailed workflows
through the [documentation index](README.md).

This guide describes the current worktree, including local pending features.
The source catalog includes Odoo 12.0-20.0 and defaults to 19.0. Latest MCP changes,
Odoo 20 support, and advanced browser integration are not promised in published
releases, PPA packages, or `go install ...@latest`. Build the intended checkout
and consult the selected binary's help; see [Installation](INSTALLATION.md).

## Code Boundaries

| Location | Responsibility |
| --- | --- |
| `cmd/` | Cobra command registration, flags, orchestration, output |
| `internal/config/` | User settings, environment state, project lookup |
| `internal/project/`, `internal/git/` | Project and branch context |
| `internal/repository/` | Source revision resolution and managed checkouts |
| `internal/odoo/` | Version catalog |
| `internal/templates/` | Embedded Docker/config templates and rendering |
| `internal/docker/` | Container and Compose execution helpers |
| `internal/module/`, `internal/deps/` | Addon discovery and dependency analysis |
| `internal/mcp/` | Project-scoped MCP inspection and browser bridge |
| `internal/browserruntime/` | Separate official Playwright MCP Docker runtime |
| `internal/selfupdate/` | Install-method detection, update planning/execution |
| `pkg/` | Shared utilities, including prompts |

Templates are embedded in the binary. Editing template source requires rebuilding
the CLI; changing the CLI does not itself replace already generated files.
Command workflows decide when to render, rebuild, initialize, or synchronize.

## Creation And Execution

1. Detect the project root/name and Git branch, or a named non-Git environment.
2. Select an Odoo version explicitly, from branch context, or through a prompt.
3. Resolve Community and optional Enterprise source revisions to commit SHAs.
4. Expand optional presets and prepare pinned managed repositories.
5. Save environment state, render Docker/config files, and save the project link.
6. Later run/build commands operate on that state; database initialization is a separate, database-affecting action.

The Compose template includes PostgreSQL (`db`), Odoo, MailHog, and profile-gated
`odoo-init`/`odoo-update` services. Project addons are bind-mounted at
`/mnt/extra-addons`; additional addons and managed repositories are mounted
separately. Managed repositories live under the environment's `repositories/`.

Ports start from the selected major version: HTTP is `8000 + major * 100`,
MailHog is HTTP + 25, SMTP is `1000 + major * 100 + 25`, and debug is
`5000 + major * 100 + 78`. Conflict handling may change these values. Inspect
saved state or `docker debug-info` rather than assuming a fixed port.

## State And Selection

```text
~/.odooctl/
  config.json
  projects/<hash-of-absolute-project-root>.json
  <project>/<environment>/
    .odooctl-state.json
    Dockerfile
    docker-compose.yml
    odoo.conf
    .env
    repositories/
    browser-artifacts/
    browser-runtime/
```

The environment name is commonly the sanitized branch name, but it can also be
a non-Git environment name. State records project root, version, source pins,
Docker schema, modules, addons paths, ports, presets, dependency metadata, browser
settings, and build/initialization timestamps. Not every optional directory exists
in every environment.

Project links use SHA-256 of the cleaned absolute project root as their filename.
There is one link per root pointing to its last selected/saved environment, not
one link per Git branch. Lookup walks parent directories to find a link. Switching
Git branches alone does not automatically or instantly switch the environment.
Verify the selected path before running database-affecting commands.

Normal lookup can fall back to scanning saved environments and repair a link.
Passive lookup avoids that repair and rejects ambiguous fallback matches.
New workflows do not create repo-local `.odooctl` marker files; compatibility
cleanup may remove an obsolete marker referring into the global config directory.
See [Environments](ENVIRONMENTS.md) for operational selection and lifecycle.

## Source And Dependencies

The Odoo image builds from `odoo/odoo`, not an official prebuilt Odoo image or
moving nightly Debian package. Creation resolves the version branch once and
saves `odoo_commit`; builds fetch that immutable revision into `/opt/odoo-src`.
Older environments without pins can resolve them during later build/reconfigure
workflows, which requires network access.

Enterprise requires authorized repository access. Community and Enterprise use
the same version branch but resolve and store separate SHAs. This is not a claim
that the two commits are an exact paired release or have matching commit history.
BuildKit supplies authentication through SSH-agent forwarding, an SSH-key secret,
or a GitHub-token secret. Credentials are not intentionally retained in image
layers, but host config/state may contain tokens and must be treated as sensitive.

Baseline Python tooling and Odoo requirements live in `/opt/odoo-venv`, which is
first on `PATH` and is created with system-site-packages enabled. Odoo 12/13 use
matching Debian Python packages for Odoo dependencies; newer versions install
the pinned source tree's requirements. Baseline tools include debugpy and IPython.

Module-specific runtime dependencies live in the persistent `odoo-pydeps` volume
mounted at `/opt/odoo-extra-python`, exposed through `PYTHONPATH`. This is separate
from the image's baseline environment. Explicit `docker deps sync` or dependency
handling during module install can update it without an image rebuild.

Creation does not scan module Python dependencies unless `--auto-discover-deps`
is selected. Module installation defaults to runtime synchronization locally;
`CI=true` or `CI=1` defaults to failure on missing dependencies. Use explicit
`--deps-mode runtime|fail` when automation must not depend on ambient settings.
See [Modules](MODULES.md) and [Commands](COMMANDS.md).

Source pins and caches reduce drift and repeated work; they do not lock every
base-image digest, OS package, Python transitive dependency, or npm download.
The committed vendor tree covers Go modules, not the compiler or container build
inputs. Neither vendoring nor source pins guarantee completely offline or
bit-for-bit reproducible builds.

## Resource And Data Boundaries

New environments use Docker schema version 2 with environment-specific image
names and Compose-scoped networks/volumes. The volumes separate PostgreSQL,
filestore, sessions, and runtime Python dependencies.

Missing schema versions normalize to legacy schema 1. Regeneration retains the
legacy explicitly named, version-suffixed volumes. Those names can be shared
across old environments; retaining names is not guaranteed automatic adoption of
every historical volume layout. Before regeneration or migration, verify actual
volume names, Compose ownership labels, mounts, PostgreSQL version, and stored
data. Back up the database and matching filestore and test restoration first.

Current templates select PostgreSQL 15 for Odoo 12.0-19.0 and PostgreSQL 16 for
20.0. Regeneration only changes configuration; it does not migrate database data.
Never attach PostgreSQL 16 to a PostgreSQL 15 data directory. Plan a supported
database migration into a separate volume rather than an in-place image change.

Stopping containers is distinct from deleting volumes. `docker reset -v`,
`docker reset -vc`, and `docker deps clean` can destroy development data or
dependencies. They are not routine troubleshooting steps without explicit approval.

## Browser And MCP Boundaries

- `docker create --browser` enables Python Playwright/Chromium inside the Odoo image for Odoo 15.0+ browser inspection and web tests. Its Python browser environment is `/opt/odoo-browser-venv`; artifacts use `browser-artifacts/`.
- `mcp serve` exposes project-scoped inspection tools over stdio. Runtime options are opt-in; the in-Odoo browser mode blocks external requests. See [MCP](ai/MCP.md) for its security and credential boundaries.
- The local pending `mcp browser` mode uses the official Node `@playwright/mcp` backend in a separate Chromium worker and Squid proxy. It does not require Python Playwright in Odoo or Node on the host.
- Explicit browser setup builds images, prepares a private network, and starts only the proxy for an existing running Odoo environment. Serving uses a prepared runtime and starts a session worker, not Odoo or its database.
- External origins require explicit approval. Observation does not mean pages are side-effect-free, nor does an allowlist establish a general security certification. See [Playwright MCP](ai/PLAYWRIGHT_MCP.md).

The CLI is cross-built for Linux/macOS amd64/arm64 and Windows amd64. That does
not certify every Docker/browser host combination. Runtime requirements remain
a working Linux-container Docker daemon, Compose access, and valid project mounts.
The [Odoo 17-20 matrix](ai/MCP_VERSION_MATRIX.md) is limited pinned Community
fixture evidence, not comprehensive addon, Enterprise, migration, or production
compatibility. Contributor checks are in [Contributing](../CONTRIBUTING.md).
