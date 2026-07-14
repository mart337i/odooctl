# odooctl

A CLI tool for managing Odoo Docker development environments. Written in Go for cross-platform support (Linux, macOS, Windows).

**Supported Odoo Versions:** 12.0, 13.0, 14.0, 15.0, 16.0, 17.0, 18.0, 19.0  
**Enterprise Support:** Yes (via `--enterprise` flag)

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Typical Development Workflow](#typical-development-workflow)
- [Using AI With odooctl](#using-ai-with-odooctl)
- [Commands Reference](#commands-reference)
- [Advanced Features](#advanced-features)
- [How It Works](#how-it-works)
- [Contributing](#contributing)

## Installation

### Quick Install (Linux/macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/mart337i/odooctl/main/install.sh | bash
```

### Ubuntu/Debian (PPA)

```bash
sudo add-apt-repository ppa:mart337i/odooctl
sudo apt update
sudo apt install odooctl
```

> **Note:** If you previously installed via another method, you may need to refresh your shell's command cache: run `hash -r` (bash/zsh) or open a new terminal.

### Using Go

```bash
go install github.com/mart337i/odooctl@latest
```

### Windows (PowerShell)

```powershell
# Download latest release to a folder in PATH
Invoke-WebRequest -Uri "https://github.com/mart337i/odooctl/releases/latest/download/odooctl-windows-amd64.exe" -OutFile "$env:LOCALAPPDATA\Microsoft\WindowsApps\odooctl.exe"
```

Or download manually from [GitHub Releases](https://github.com/mart337i/odooctl/releases) and add to your PATH.

### Build from Source

```bash
git clone https://github.com/mart337i/odooctl.git
cd odooctl
make install
```

## Quick Start

```bash
# Navigate to your Odoo module directory
cd /path/to/your/odoo-modules

# Create a new Docker environment
odooctl docker create --odoo-version 18.0 --modules sale,purchase

# Start containers and initialize database
odooctl docker run -i

# Access Odoo at http://localhost:9800
# Login: admin / admin
```

## Typical Development Workflow

### 1. Initial Setup

```bash
# Start in your module directory
cd ~/projects/my-odoo-modules

# Create environment (auto-detects git repo and version from branch)
odooctl docker create

# Or specify version explicitly
odooctl docker create --odoo-version 17.0 --modules sale,stock

# With Odoo Enterprise
odooctl docker create --odoo-version 18.0 --enterprise
```

**What happens:**
- Detects project name from git repo or directory name
- Extracts Odoo version from git branch (e.g., `17.0-feature` → `17.0`)
- Calculates ports based on version (Odoo 17 → port 9700)
- Generates Docker configs in `~/.odooctl/{project}/{branch}/`
- Stores project lookup links in `~/.odooctl/projects/` without repo-local marker files
- Does not scan Python dependencies unless `--auto-discover-deps` is explicitly passed

### 2. First Run

```bash
# Start containers and initialize database
odooctl docker run -i

# Or separately
odooctl docker run --build  # Build images first time
# Then later when ready
odooctl docker run -i       # Initialize database
```

**What happens:**
- Builds Docker image with Odoo and baseline developer tooling
- Starts PostgreSQL and Odoo containers
- Initializes database with base modules
- Configures report.url for proper PDF generation
- Tracks initialization state in `.odooctl-state.json`
- Keeps module Python dependencies in the runtime volume at `/opt/odoo-extra-python`

### 3. Daily Development

```bash
# Check what's running
odooctl docker status

# View logs
odooctl docker logs -f

# Open Odoo or MailHog
odooctl docker open odoo
odooctl docker open mailhog

# Develop your module...
# Edit code in your local directory

# Install/update only changed modules (smart detection)
odooctl docker install

# Or force update specific modules
odooctl docker install my_module,my_other_module

# Stop for the day
odooctl docker stop
```

### 4. Testing Your Changes

```bash
# Run Odoo tests for your module
odooctl docker test --modules my_module --test-tags post_install

# Or run specific test class
odooctl docker test --test-tags /my_module:TestMyClass

# Open Odoo shell for debugging
odooctl odoo shell
>>> self.env['res.partner'].search([])

# Open PostgreSQL shell
odooctl docker db

# Run quick SQL
odooctl docker sql "select id, login from res_users"
```

### 5. Adding Dependencies

```bash
# Scan manifests for external Python dependencies
odooctl docker deps scan

# Install packages into the runtime dependency volume
odooctl docker deps sync requests pandas

# Or sync missing dependencies discovered from manifests
odooctl docker deps sync

# Add custom addons path
odooctl docker reconfigure --add-addons-path ~/external-addons
```

### 6. Creating New Modules

```bash
# Scaffold a new module with model
odooctl module scaffold my_new_module --model

# Scaffold in specific version
odooctl module scaffold my_module --odoo-version 18.0 --model

# Install the new module
odooctl docker install my_new_module
```

### 7. Working with Branches

```bash
# Switch git branch
git checkout -b 18.0-new-feature

# Create new environment for this branch
odooctl docker create

# Now you have isolated environments:
# ~/.odooctl/my-project/17.0-main/
# ~/.odooctl/my-project/18.0-new-feature/
```

### 8. Cleanup

```bash
# Stop containers only
odooctl docker reset

# Stop and remove database
odooctl docker reset -v

# Full cleanup (containers, volumes, and config files)
odooctl docker reset -v -c -f
```

## Interacting With Containers During Development

odooctl wraps the generated Docker Compose environment so you do not need to find
the compose directory, remember service names, or type database/config paths.

Run arbitrary commands inside a service:

```bash
odooctl docker exec odoo -- python --version
odooctl docker exec odoo -- ls /mnt/extra-addons
odooctl docker exec --root odoo -- apt update
odooctl docker exec -T db -- psql -U odoo -d odoo-190 -c "select now();"
```

Use raw Compose when needed:

```bash
odooctl docker compose ps
odooctl docker compose -- top
odooctl docker compose -- ps --services
```

Restart only the Odoo service after code changes:

```bash
odooctl docker restart
odooctl docker restart odoo
odooctl docker restart db odoo
```

Open shells:

```bash
odooctl docker shell
odooctl docker shell db
odooctl docker shell --root
odooctl docker shell --odoo
```

Query the database without entering psql:

```bash
odooctl docker sql "select id, login from res_users"
odooctl docker sql --json "select name, state from ir_module_module where name = 'sale'"
odooctl docker sql --file debug.sql
```

Filter logs for Odoo errors:

```bash
odooctl docker logs --errors
odooctl docker logs --grep Traceback --since 10m
odooctl docker logs db --since 30m
```

Print URLs and debugger attach details:

```bash
odooctl docker open
odooctl docker debug-info
```

Use Odoo-specific helpers for ORM/runtime tasks:

```bash
odooctl odoo shell
odooctl odoo eval "env['res.users'].search([]).mapped('login')"
odooctl odoo update-apps
odooctl odoo module-state my_module --json
```

## Browser, Design, and Web Test Workflows

Enable Playwright Chromium when creating or reconfiguring an environment:

```bash
odooctl docker create --browser
odooctl docker reconfigure --browser --rebuild
```

Browser tooling is supported for Odoo 15.0+ environments. It installs Python
Playwright and Chromium inside the Odoo image, exposes `chromium` and
`google-chrome` for Odoo's browser tests, and stores generated artifacts under
the environment's `browser-artifacts/` directory.

Check browser readiness:

```bash
odooctl browser doctor
odooctl browser doctor --json
```

Use browser inspection for AI-assisted design and debugging:

```bash
odooctl browser snapshot /web
odooctl browser inspect /web --json
odooctl browser screenshot /web --output /tmp/odoo.png
odooctl browser check /web --expect-text "Discuss"
odooctl browser trace /web --output /tmp/odoo-trace.zip
```

If login is needed:

```bash
odooctl browser inspect /web --login admin --password admin --json
```

Run Odoo web/browser tests with an early Chromium readiness check:

```bash
odooctl docker test --web --test-tags /web
```

## Using AI With odooctl

odooctl can generate local, redacted context for ChatGPT, Claude, OpenCode,
Cursor, and other AI tools. It does not call an LLM API itself.

Start with diagnostics:

```bash
odooctl doctor
odooctl doctor --json
```

Generate compact context for a developer chat:

```bash
odooctl ai context
odooctl ai context --module my_module
```

Generate machine-readable context for an agent:

```bash
odooctl ai context --module my_module --format json
```

When debugging failures, create a redacted report:

```bash
odooctl ai debug-report --module my_module --include-logs
```

Generate a ready-to-paste prompt:

```bash
odooctl ai prompt debug --module my_module
```

Recommended safe first commands for agents:

```bash
odooctl doctor --json
odooctl ai context --format json
odooctl module list --json
odooctl docker status --json
odooctl docker install --list-only --json
odooctl docker debug-info --json
```

For browser or design tasks, first enable browser tooling and then use:

```bash
odooctl browser doctor --json
odooctl browser inspect /web --json
```

Agents should not run destructive commands such as `odooctl docker reset -v`,
`odooctl docker reset -vc`, or `odooctl docker deps clean` unless the developer
explicitly approves the data loss.

## Commands Reference

### Diagnostics and AI Commands

| Command | Description |
|---------|-------------|
| `odooctl doctor` | Diagnose project state, Docker access, services, files, and dependencies |
| `odooctl doctor --json` | Print structured diagnostics for AI agents and automation |
| `odooctl ai context` | Print compact AI-ready project context |
| `odooctl ai context --module my_module` | Print context focused on one module |
| `odooctl ai context --format json` | Print machine-readable AI context |
| `odooctl ai debug-report --include-logs` | Print a redacted debugging report with recent logs |
| `odooctl ai debug-report --include-browser` | Include Playwright Chromium runtime status |
| `odooctl ai prompt debug --module my_module` | Generate a ready-to-paste debugging prompt |

### Browser Commands

| Command | Description |
|---------|-------------|
| `odooctl browser doctor` | Check Playwright Chromium availability in the Odoo image |
| `odooctl browser inspect` | Return page title, visible text, console errors, and failed requests |
| `odooctl browser snapshot` | Print a compact visible-text snapshot |
| `odooctl browser screenshot` | Capture a full-page screenshot |
| `odooctl browser check` | Assert that expected visible text appears |
| `odooctl browser trace` | Record a Playwright trace ZIP |

### JSON Output

Useful inspection commands support `--json` for agents and scripts:

```bash
odooctl version --json
odooctl config show --json
odooctl config get ssh-key-path --json
odooctl config set ssh-key-path ~/.ssh/id_ed25519 --json
odooctl config unset github-token --json
odooctl doctor --json
odooctl docker create --json
odooctl docker status --json
odooctl docker path --json
odooctl docker stop --json
odooctl docker reset --json
odooctl docker logs --json
odooctl docker dump --json
odooctl docker restart --json
odooctl docker open --json
odooctl docker debug-info --json
odooctl docker sql --json "select name, state from ir_module_module"
odooctl docker deps scan --json
odooctl docker deps list --json
odooctl docker goto --json
odooctl docker install --list-only --json
odooctl module list --json
odooctl module deps my_module --json
odooctl module manifest my_module --json
odooctl module changed --json
odooctl module scaffold my_module --json
odooctl module migrate plan my_module --json
odooctl module migrate scaffold my_module --to 19.0.1.0.0 --json
odooctl odoo module-state my_module --json
odooctl browser doctor --json
odooctl browser inspect /web --json
odooctl browser screenshot /web --json
odooctl browser check /web --expect-text "Discuss" --json
odooctl browser trace /web --json
```

### Docker Commands

| Command | Description |
|---------|-------------|
| `odooctl docker create` | Generate Docker environment files |
| `odooctl docker compose` | Run docker compose in the generated environment directory |
| `odooctl docker run` | Initialize database and start containers |
| `odooctl docker exec` | Run a command inside a service |
| `odooctl docker restart` | Restart one or more services, defaulting to Odoo |
| `odooctl docker status` | Show container status and access URLs |
| `odooctl docker logs` | View container logs (`-f` to follow) |
| `odooctl docker install` | Install/update modules with hash-based change detection |
| `odooctl docker test` | Run Odoo tests with advanced filtering |
| `odooctl docker shell` | Open bash or Odoo shell in container |
| `odooctl docker db` | Open PostgreSQL shell |
| `odooctl docker sql` | Run quick SQL against the Odoo database |
| `odooctl docker deps` | Scan, sync, list, or clean Python dependencies |
| `odooctl docker odoo-bin` | Run odoo-bin commands directly |
| `odooctl docker open` | Open or print Odoo/MailHog URLs |
| `odooctl docker debug-info` | Show URLs, DB, config paths, and debugger attach config |
| `odooctl docker stop` | Stop running containers |
| `odooctl docker reset` | Remove containers, optionally volumes and files |
| `odooctl docker reconfigure` | Add pip packages or addons paths |
| `odooctl docker goto` | Navigate to environment directory |
| `odooctl docker path` | Print environment directory path |
| `odooctl docker edit` | Edit configuration files |

### Module Commands

| Command | Description |
|---------|-------------|
| `odooctl module scaffold` | Create a new Odoo module with proper structure |
| `odooctl module list` | List modules discovered in the project/addons paths |
| `odooctl module deps` | Show manifest module and Python dependencies |
| `odooctl module manifest` | Inspect a parsed module manifest |
| `odooctl module changed` | Show local modules whose hashes changed |
| `odooctl module test` | Run tests for modules using Odoo test tags |
| `odooctl module upgrade` | Install/update modules through Docker |
| `odooctl module migrate` | Plan or scaffold module migration files |

### Odoo Runtime Commands

| Command | Description |
|---------|-------------|
| `odooctl odoo shell` | Open Odoo's Python shell for the current database |
| `odooctl odoo eval` | Evaluate a Python expression in Odoo shell context |
| `odooctl odoo update-apps` | Refresh Odoo's apps/module list |
| `odooctl odoo module-state` | Inspect module states from `ir.module.module` |

## Advanced Features

### Smart Module Installation

The `install` command uses SHA256 hashing to detect actual code changes:

```bash
# Auto-detect and update only changed modules
odooctl docker install

# See what would be updated (dry run)
odooctl docker install --list-only

# Install all modules in directory
odooctl docker install all

# Use wildcards
odooctl docker install sale_*

# Ignore specific modules
odooctl docker install all --ignore=base,web

# Force full upgrade
odooctl docker install --update-all
```

**How it works:**
1. Calculates SHA256 hash of each module (excludes tests, static, __pycache__)
2. Compares with stored hashes from `module-hashes.json`
3. Only runs odoo-bin -u for modules that actually changed
4. Dramatically faster than always updating everything

### Automatic Python Dependency Discovery

`odooctl docker create` does not scan or prompt for module Python dependencies by default. That keeps environment creation predictable and avoids dependency-install failures during startup.

Use explicit dependency commands when you want to inspect or sync dependencies:

During `odooctl docker install`, odooctl also scans the modules being installed
or updated. Missing `external_dependencies['python']` packages are installed into
a persistent runtime dependency volume before Odoo runs the module install/update.
Local development defaults to `--deps-mode runtime`; when `CI=true`, the default
switches to `fail` so CI reports missing dependencies instead of mutating the
environment silently.

```bash
# Scan configured modules/addons paths
odooctl docker deps scan

# Install missing dependencies into the runtime dependency volume
odooctl docker deps sync

# Force install explicit packages
odooctl docker deps sync requests zeep

# Clean the runtime dependency volume
odooctl docker deps clean
```

You can still opt in during create or reconfigure:

```bash
odooctl docker create --auto-discover-deps
odooctl docker reconfigure --auto-discover-deps
```

Example manifest:
```python
{
    'external_dependencies': {
        'python': ['requests', 'pandas'],
    },
}
```

### Multi-Environment Support

Project structure: `~/.odooctl/{project}/{branch}/`

**Benefits:**
- Test same modules on different Odoo versions
- Isolate feature branches
- Switch between environments instantly

```bash
# In git repo with branch "17.0-main"
odooctl docker create
# → ~/.odooctl/my-project/17.0-main/

# Switch branch
git checkout 18.0-feature
odooctl docker create
# → ~/.odooctl/my-project/18.0-feature/

# Both environments coexist independently
```

### Port Auto-Resolution

Ports are calculated from Odoo version: `8000 + (version * 100)`

| Version | Odoo Port | Mailhog | Debug |
|---------|-----------|---------|-------|
| 12.0    | 9200      | 9225    | 5278  |
| 13.0    | 9300      | 9325    | 5378  |
| 14.0    | 9400      | 9425    | 5478  |
| 15.0    | 9500      | 9525    | 5578  |
| 16.0    | 9600      | 9625    | 5678  |
| 17.0    | 9700      | 9725    | 5778  |
| 18.0    | 9800      | 9825    | 5878  |
| 19.0    | 9900      | 9925    | 5978  |

If ports conflict, odooctl automatically finds available ports and regenerates configs.

### Version-Aware Module Scaffolding

Templates automatically adjust to Odoo version:

```bash
# Odoo 18+ uses <list> in views
odooctl module scaffold my_module --odoo-version 18.0 --model

# Odoo 17 and below uses <tree>
odooctl module scaffold my_module --odoo-version 17.0 --model
```

### Test Filtering

Run specific tests with powerful filtering:

```bash
# Run post_install tests only
odooctl docker test --modules my_module --test-tags post_install

# Run specific test class
odooctl docker test --test-tags /my_module:TestMyClass

# Run specific test method
odooctl docker test --test-tags .test_method_name

# Exclude slow tests
odooctl docker test --test-tags 'standard,-slow'

# With debug logging
odooctl docker test --modules my_module --log-level=test:DEBUG
```

## How It Works

### Architecture

- **Language:** Go 1.22 (cross-platform, single binary)
- **Package Structure:**
  - `cmd/` - CLI commands (cobra-based)
  - `internal/` - Core logic (config, docker, templates, git, modules)
  - `pkg/` - Public utilities (prompts)

### State Management

Environment state is stored in `~/.odooctl/{project}/{branch}/.odooctl-state.json`.
Project lookup uses global links in `~/.odooctl/projects/`, keyed by the absolute
project root. odooctl does not create a repo-local `.odooctl` marker file.

```json
{
  "project_name": "my-project",
  "odoo_version": "18.0",
  "branch": "main",
  "modules": ["sale", "purchase"],
  "ports": {
    "odoo": 9800,
    "mailhog": 9825
  },
  "python_deps_hash": "...",
  "initialized_at": "2024-01-15T10:30:00Z",
  "built_at": "2024-01-15T10:25:00Z"
}
```

### Docker Container Design

**Why use Python virtual environments?**

The generated Dockerfile installs baseline developer Python tools into `/opt/odoo-venv` instead of the system Python environment:

- Avoids conflicts with apt-managed Odoo dependencies
- Avoids PEP 668 `--break-system-packages` failures on newer base images
- Keeps Python tooling isolated and first on Odoo's Python path
- Still exposes apt-installed Odoo packages through `--system-site-packages`

Module-specific Python packages are installed into a persistent runtime volume at
`/opt/odoo-extra-python`, which is added to `PYTHONPATH`. This avoids rebuilding
the Docker image when a module adds or changes `external_dependencies['python']`.
The image keeps baseline developer tools in `/opt/odoo-venv`; module dependencies
are synchronized with `odooctl docker deps sync` or automatically by
`odooctl docker install`.

**Included Tools:**
- Python 3 + pip
- PostgreSQL client
- wkhtmltopdf (for PDF reports)
- git, vim, htop (development tools)
- debugpy (remote debugging)
- ipython (Odoo shell)

### Vendor Directory

The project includes a vendored `vendor/` directory (14MB) committed to git.

**Why?**
- Required for Debian PPA packaging
- Ensures reproducible builds
- Offline build capability

For development, you can use `go mod` normally. Vendoring is only needed for release builds.

## Contributing

### Development Setup

```bash
# Clone repository
git clone https://github.com/mart337i/odooctl.git
cd odooctl

# Build
make build

# Run
./bin/odooctl --help

# Format and vet
make fmt
make vet
```

### Code Quality

```bash
# Format code (excluding vendor)
make fmt

# Run static analysis
make vet

# Build for all platforms
make build-all
```

## Troubleshooting

### Port Conflicts

If you see port conflicts:
```bash
odooctl docker run
# → Automatically detects conflicts and uses available ports
```

### Database Issues

```bash
# Reset database and reinitialize
odooctl docker reset -v
odooctl docker run -i
```

### Module Not Updating

```bash
# Check if module actually changed
odooctl docker install --list-only

# Force update
odooctl docker install my_module

# Or force full upgrade
odooctl docker install --update-all
```

### Container Won't Start

```bash
# Check logs
odooctl docker logs

# Rebuild from scratch
odooctl docker reset -v
odooctl docker run --build -i
```

## License

MIT

## Links

- [GitHub Repository](https://github.com/mart337i/odooctl)
- [Issue Tracker](https://github.com/mart337i/odooctl/issues)
- [Releases](https://github.com/mart337i/odooctl/releases)
