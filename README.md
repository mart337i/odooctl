# odooctl

A CLI tool for managing Odoo Docker development environments. Written in Go for cross-platform support (Linux, macOS, Windows).

**Supported Odoo Versions:** 12.0, 13.0, 14.0, 15.0, 16.0, 17.0, 18.0, 19.0  
**Enterprise Support:** Yes (via `--enterprise` flag)

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Typical Development Workflow](#typical-development-workflow)
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
- Auto-discovers Python dependencies from your `__manifest__.py` files

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
- Builds Docker image with Odoo + your specified pip packages
- Starts PostgreSQL and Odoo containers
- Initializes database with base modules
- Configures report.url for proper PDF generation
- Tracks initialization state in `.odooctl-state.json`

### 3. Daily Development

```bash
# Check what's running
odooctl docker status

# View logs
odooctl docker logs -f

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
odooctl docker shell --odoo
>>> self.env['res.partner'].search([])

# Open PostgreSQL shell
odooctl docker db
```

### 5. Adding Dependencies

```bash
# Add Python packages to existing environment
odooctl docker reconfigure --add-pip requests,pandas

# Or from requirements.txt
odooctl docker reconfigure --add-pip ./requirements.txt

# Auto-discover dependencies from manifests
odooctl docker reconfigure --auto-discover-deps

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

## Commands Reference

### Docker Commands

| Command | Description |
|---------|-------------|
| `odooctl docker create` | Generate Docker environment files |
| `odooctl docker run` | Initialize database and start containers |
| `odooctl docker status` | Show container status and access URLs |
| `odooctl docker logs` | View container logs (`-f` to follow) |
| `odooctl docker install` | Install/update modules with hash-based change detection |
| `odooctl docker test` | Run Odoo tests with advanced filtering |
| `odooctl docker shell` | Open bash or Odoo shell in container |
| `odooctl docker db` | Open PostgreSQL shell |
| `odooctl docker deps` | Scan, sync, list, or clean Python dependencies |
| `odooctl docker odoo-bin` | Run odoo-bin commands directly |
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
