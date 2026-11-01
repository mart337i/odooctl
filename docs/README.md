# Documentation

Start with the [project README](../README.md) for installation, quick start, and
the capability overview. These guides describe the current source; an installed
release may have fewer commands. Check the selected binary's `--help` when an
example does not match your installation.

## Start And Develop

| Guide | Use it for |
| --- | --- |
| [Installation](INSTALLATION.md) | Release binaries, source/Go builds, Windows, PPA, updates, and verification |
| [Development](DEVELOPMENT.md) | Initial setup, daily work, container tools, dependencies, and service changes |
| [Environments](ENVIRONMENTS.md) | Project selection, source pins, ports, Enterprise, presets, backups, and cleanup |
| [Modules](MODULES.md) | Discovery, scaffolding, hash-aware installation, tests, and migration helpers |
| [Commands](COMMANDS.md) | Complete command-family reference, output formats, and side effects |

## Inspect And Debug

| Guide | Use it for |
| --- | --- |
| [Browser](BROWSER.md) | Choose between CLI captures, strict MCP observation, and the approved-origin browser |
| [Debugging](ai/DEBUGGING_WORKFLOWS.md) | Startup, module, test, and UI failures; diagnostics before recovery |
| [AI workflows](ai/USING_ODOOCTL_WITH_AI.md) | Context reports, prompt generation, and agent-safe workflows |

The debugging guide is useful without an AI client. JSON is an output format,
not permission to execute a command or a guarantee that it changes no state.

## Connect AI Clients

| Guide | Use it for |
| --- | --- |
| [Inspection MCP](ai/MCP.md) | Source/runtime tools, restricted SQL, ORM queries, credentials, and OpenCode/Claude Code setup |
| [Playwright MCP](ai/PLAYWRIGHT_MCP.md) | Official browser backend, CDN/map approval, responsive captures, privacy, and network policy |
| [Version matrix](ai/MCP_VERSION_MATRIX.md) | Dated Odoo 17-20 agent-task evidence, failures, fixes, and reproducible checks |

Both authenticated MCP browser modes default to `admin` / `admin` for local
development. Nonempty credential environment variables override each value;
the separate Playwright mode supports `--public` to skip login. Ordinary browser
CLI commands instead use explicit login flags. See [Browser](BROWSER.md) before
choosing a runtime or sharing artifacts.

## Understand And Contribute

- [Architecture](ARCHITECTURE.md): packages, generated files, state, dependencies,
  and resource boundaries.
- [Contributing](../CONTRIBUTING.md): Go 1.25+ setup, tests, vendoring, packaging,
  and documentation conventions.

The version matrix covers pinned Community fixtures, not all addons, Enterprise
workflows, migrations, or production security. New MCP/browser and Odoo 20
capabilities require a build containing those changes; published-package
availability is not established by documentation or local test results.
