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

| Command | Description |
|---------|-------------|
| `odooctl docker create` | Generate Docker environment files |
| `odooctl docker run` | Initialize database and start containers |
| `odooctl docker status` | Show container status |
| `odooctl docker logs` | View container logs |
| `odooctl docker reset` | Remove containers, volumes, and files |

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
