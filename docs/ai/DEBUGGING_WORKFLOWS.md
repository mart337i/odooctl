# AI Debugging Workflows

## Environment Fails To Start

Run:

```bash
odooctl doctor --json
odooctl docker status --json
odooctl docker debug-info --json
odooctl ai debug-report --include-logs --include-browser
```

Check Docker daemon and bind-mount diagnostics first. On WSL, bind-mount failures usually mean Docker Desktop WSL integration or file sharing is broken.

## Module Fails To Install

Run:

```bash
odooctl ai context --module my_module
odooctl module manifest my_module --json
odooctl module deps my_module --json
odooctl docker deps scan --modules my_module --json
odooctl docker install my_module --list-only --json
odooctl odoo module-state my_module --json
```

If dependencies are missing, use:

```bash
odooctl docker deps sync --modules my_module
```

Then retry:

```bash
odooctl docker install my_module
```

## Tests Fail

Run:

```bash
odooctl ai debug-report --module my_module --include-logs
odooctl docker logs --errors --since 10m
odooctl browser inspect /web --json
odooctl module test my_module
```

If a specific test fails, include the exact `--test-tags` command and traceback in the AI prompt.

## Browser Or UI Looks Wrong

Run:

```bash
odooctl browser doctor --json
odooctl browser screenshot /web --output /tmp/odoo.png
odooctl browser inspect /web --json
odooctl browser trace /web --output /tmp/odoo-trace.zip
```

For Odoo browser tests:

```bash
odooctl docker test --web --test-tags /web
```

## Safe Cleanup

Stopping containers is safe:

```bash
odooctl docker reset
```

Removing volumes deletes database and filestore data. Only run after explicit approval:

```bash
odooctl docker reset -v
odooctl docker reset -vc
```
