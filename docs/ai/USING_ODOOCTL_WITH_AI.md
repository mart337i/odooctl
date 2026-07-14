# Using odooctl With AI

odooctl helps AI tools by producing deterministic, local context. It does not call an LLM provider.

## Developer Workflow

1. Run diagnostics:

```bash
odooctl doctor
```

2. Generate context:

```bash
odooctl ai context --module my_module
```

3. Paste the output into your AI tool with the question or error you want help with.

4. If debugging, include logs explicitly:

```bash
odooctl ai debug-report --module my_module --include-logs
```

## Agent Workflow

Prefer JSON:

```bash
odooctl doctor --json
odooctl ai context --module my_module --format json
odooctl docker status --json
odooctl docker debug-info --json
odooctl docker deps scan --json
odooctl docker install my_module --list-only --json
odooctl odoo module-state my_module --json
```

Do not use destructive cleanup commands without explicit approval.

## Browser Context

When browser tooling is enabled with `odooctl docker create --browser` or
`odooctl docker reconfigure --browser --rebuild`, AI agents can inspect the live
UI without calling an LLM provider:

```bash
odooctl browser doctor --json
odooctl browser inspect /web --json
odooctl browser screenshot /web --json
odooctl browser check /web --expect-text "Discuss" --json
```

Use `odooctl ai debug-report --include-browser` to include Playwright Chromium
runtime status in a debugging report.

## Prompt Generation

Use:

```bash
odooctl ai prompt debug --module my_module
```

This produces a prompt with guardrails, current context, and safe next commands.
