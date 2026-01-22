# odooctl

A CLI tool for managing Odoo Docker development environments. Written in Go for cross-platform support (Linux, macOS, Windows).

## Installation

```bash
# Build from source
make build

# Or install to GOPATH/bin
make install
```

## Quick Start

```bash
# Create a new Docker environment
cd /path/to/your/odoo-modules
odooctl docker create --odoo-version 18.0 --modules sale,purchase

# Start containers (initializes database on first run)
odooctl docker run

# Check status
odooctl docker status

# View logs
odooctl docker logs
odooctl docker logs -f  # follow

# Clean up everything
odooctl docker reset
```

## Commands

### Docker Commands

| Command | Description |
|---------|-------------|
| `odooctl docker create` | Generate Docker environment files |
| `odooctl docker run` | Initialize database and start containers |
| `odooctl docker status` | Show container status |
| `odooctl docker logs` | View container logs |
| `odooctl docker install` | Install/update modules with hash-based change detection |
| `odooctl docker shell` | Open bash or Odoo shell in container |
| `odooctl docker db` | Open PostgreSQL shell |
| `odooctl docker odoo-bin` | Run odoo-bin commands directly |
| `odooctl docker reset` | Remove containers, volumes, and files |

### Module Commands

| Command | Description |
|---------|-------------|
| `odooctl module scaffold` | Create a new Odoo module |

## Module Scaffold

Create new Odoo modules with proper structure:

```bash
# Basic module (no model)
odooctl module scaffold my_module

# Module with a model
odooctl module scaffold my_module --model

# With custom options
odooctl module scaffold my_module \
  --author "My Company" \
  --depends sale,purchase \
  --odoo-version 18.0 \
  --model
```

### Generated Structure

```
my_module/
├── __manifest__.py
├── __init__.py
├── static/.gitkeep
├── data/.gitkeep
├── models/              # (with --model)
│   ├── __init__.py
│   └── my_module.py
├── views/               # (with --model)
│   └── my_module_views.xml
└── security/            # (with --model)
    └── ir.model.access.csv
```

### Version-Aware Templates

- **Odoo 18+**: Uses `<list>` element in views
- **Odoo 17 and below**: Uses `<tree>` element in views

## Install Command

The `install` command provides safe module updates with hash-based change detection:

```bash
# Install specific modules
odooctl docker install sale purchase

# Use wildcards
odooctl docker install sale_*
odooctl docker install *_account

# Auto-detect all modules in current directory
odooctl docker install all

# List what would be updated (dry run)
odooctl docker install --list-only

# Ignore specific modules
odooctl docker install all --ignore=base,web

# Only compute and store hashes (no update)
odooctl docker install --compute-hashes
```

### How it works:
1. Calculates SHA256 hash of each module (excluding tests, static, __pycache__)
2. Compares with stored hashes from previous run
3. Only installs/updates modules that have actually changed
4. Stores new hashes after successful update

## Shell Commands

```bash
# Bash shell in odoo container
odooctl docker shell

# Odoo Python shell (ipython with Odoo env)
odooctl docker shell --odoo

# Bash shell in database container
odooctl docker shell --service db

# PostgreSQL shell
odooctl docker db

# Run any odoo-bin command
odooctl docker odoo-bin --help
odooctl docker odoo-bin scaffold my_module /mnt/extra-addons
```

## Create Options

```
--name, -n          Project name (default: directory name or git repo name)
--odoo-version, -v  Odoo version: 16.0, 17.0, 18.0, 19.0
--modules, -m       Modules to install (comma-separated)
--enterprise, -e    Include Odoo Enterprise
--pip, -p           Extra pip packages (comma-separated)
--force, -f         Overwrite existing configuration
```

## How It Works

1. **Project Detection**: Automatically detects git repos (uses repo name + branch) or falls back to directory name
2. **Version Detection**: Extracts Odoo version from git branch name (e.g., `17.0-feature` → `17.0`) or prompts
3. **Port Calculation**: Auto-assigns ports based on version (e.g., Odoo 18 → port 9800)
4. **File Generation**: Creates Docker Compose, Dockerfile, and configs in `~/.odooctl/{project}/`

## Generated Files

```
~/.odooctl/{project}/
├── docker-compose.yml
├── Dockerfile
├── odoo.conf
├── entrypoint.sh
├── wait-for-psql.py
├── .env
├── .dockerignore
└── .odooctl-state.json
```

## Cross-Platform Builds

```bash
make build-all
```

Creates binaries for:
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64)

## License

MIT
