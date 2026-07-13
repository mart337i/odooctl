# Agent Guide for odooctl

This repository builds `odooctl`, a Go CLI for Odoo Docker development environments.

## Safe First Commands

Use these read-only commands before changing code or touching containers:

```bash
odooctl doctor --json
odooctl ai context --format json
odooctl module list --json
odooctl docker status --json
odooctl docker path --json
odooctl docker debug-info --json
odooctl docker install --list-only --json
```

For a module-focused task:

```bash
odooctl ai context --module <module> --format json
odooctl module manifest <module> --json
odooctl module deps <module> --json
odooctl odoo module-state <module> --json
```

## Destructive Commands

Do not run these unless the user explicitly approves the data loss:

```bash
odooctl docker reset -v
odooctl docker reset -vc
odooctl docker deps clean
```

Database-affecting commands should be called out before use:

```bash
odooctl docker run -i
odooctl docker install <module>
odooctl docker sql "select ..."
odooctl odoo update-apps
odooctl docker test --modules <module>
odooctl module test <module>
```

## Debugging Workflow

Use `odooctl doctor --json` first. If Docker bind mounts fail, fix Docker Desktop WSL/file sharing before trying Odoo commands.

Use `odooctl ai debug-report --module <module> --include-logs` when the user reports an install, upgrade, test, or startup failure.

Redact secrets from any copied logs or config. `odooctl ai debug-report` redacts common token/password patterns by default.
