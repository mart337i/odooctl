# Contributing

odooctl is a Go CLI for Odoo Docker development environments. Keep changes focused,
test behavior at the appropriate boundary, and distinguish source capabilities
from published release capabilities.

Start with the [README](README.md) and [documentation index](docs/README.md).
Use [Architecture](docs/ARCHITECTURE.md) for state and runtime boundaries,
[Development](docs/DEVELOPMENT.md) for daily workflows, and
[Installation](docs/INSTALLATION.md) for distribution and update details.

## Local Setup

The current worktree requires Go 1.25.0 or newer. Install Git and make for the
Makefile workflow. Docker with Linux containers and the Compose plugin is needed
only for container/live integration work, not ordinary CLI compilation.

```bash
git clone https://github.com/mart337i/odooctl.git
cd odooctl
make build
./bin/odooctl version
./bin/odooctl --help
```

`make build` produces `bin/odooctl` with a version from Git description, including
dirty state when applicable. Use this explicit binary path during development;
an older `odooctl` on `PATH` can silently test the wrong implementation.
`make build VERSION=dev` explicitly sets the embedded version. Plain `go build`
uses the `cmd/root.go` fallback, currently `0.2.5`, rather than Makefile metadata.

Latest MCP changes, Odoo 20 support, and advanced browser integration are local
pending features, not evidence of availability in a release, PPA, or `@latest`.
For feature verification, the selected build's help is authoritative.

## Checks Before Review

Run from the repository root:

```bash
make fmt
go test ./...
go test -race ./...
go vet ./...
make build
```

- `make fmt` rewrites Go files outside `vendor/`; inspect the diff for unrelated formatting changes.
- `make test` runs verbose Go tests; `make vet` is the Makefile static-analysis target.
- Run focused package tests while iterating, then the wider suite. Add regression tests for behavior changes, including error and safety paths.
- Race tests require a supported platform and C toolchain. Report an unavailable race check as not run, not passed.
- `make lint` requires a separately installed `golangci-lint`; it is not a prerequisite installed by the Makefile.

For cross-compilation, `make build-all` builds Linux and macOS amd64/arm64 plus
Windows amd64. **It runs `clean` first and removes the entire `bin/` directory.**
Do not keep irreplaceable binaries or artifacts there. Cross-compilation is not
a runtime test of those operating systems or their Docker installations.

## Dependencies And Packaging

Update module declarations and checksums together. After intentional dependency
changes, regenerate the committed vendor tree with the required Go toolchain:

```bash
go mod tidy
go mod vendor
go test ./...
```

Review `go.mod`, `go.sum`, and `vendor/` changes together; do not hand-edit vendored
libraries as a substitute for updating their upstream dependency. Validate that
the vendor tree matches the module files before relying on `-mod=vendor` builds.

Vendoring avoids Go module downloads when synchronized. It does not provide the
compiler, guarantee identical binaries, or remove image/apt/pip/npm/Chromium
network requirements. Debian packaging uses `GOFLAGS=-mod=vendor` and requires
`golang-go (>= 1.25)`; its rules currently skip tests. A package build is therefore
not test-suite evidence.

CI reads its Go version from `go.mod`, checks formatting, runs vet/tests, and
builds the CLI. Release automation builds platform assets with an injected tag.
PPA publishing is separately gated by `ODOOCTL_PPA_GO_125_READY`; workflow source
does not prove that Launchpad has a working current package or suitable compiler.

## Live Tests And Evidence

Ordinary tests include mocks and opt-in live tests. For a routine local check,
leave live-test environment variables unset. Inspect verbose output for skips:
a passing default suite does not mean skipped Docker, browser, SQL, ORM, or
version-matrix tests passed. Node/Python-dependent checks can also skip when their
host interpreter is unavailable; host Node is not required for Docker operation.

- `ODOOCTL_MCP_TEST_PROJECT` opts into inspection of an existing fixture project.
- `ODOOCTL_MCP_TEST_SQL_ROLE`, `ODOOCTL_MCP_TEST_ORM_UID`, and `ODOOCTL_MCP_TEST_BROWSER=true` enable specific runtime checks; use least-privileged fixture identities.
- `ODOOCTL_PLAYWRIGHT_TEST_PROJECT` opts into prepared official Playwright runtime tests. Follow the test source and [Playwright MCP guide](docs/ai/PLAYWRIGHT_MCP.md) for setup and credentials.
- Use disposable development projects and databases. Do not point live tests at production data, production accounts, or unrelated existing environments.
- Record the binary/source revision, Odoo pins, PostgreSQL version, enabled checks, failures, and skipped checks. Do not turn a partial run into a compatibility claim.

The [MCP version matrix](docs/ai/MCP_VERSION_MATRIX.md) and
[Playwright MCP evidence](docs/ai/PLAYWRIGHT_MCP.md) describe limited Odoo Community
17-20 fixtures. They do not certify all addons, Enterprise, record rules,
migrations, browser hosts, or production security. See [Browser](docs/BROWSER.md)
for the distinction between the in-Odoo Python browser and Node MCP sidecar.

## Safety And Documentation

Never commit credentials, private keys, tokens, production dumps, or unredacted
logs/screenshots. Config/state can contain secrets; automatic redaction is not
a guarantee that every artifact is safe to share. Review reports before publishing.

Call out database-affecting initialization, module upgrades, SQL, and live web
tests before running them. Obtain explicit data-loss approval before volume
resets or runtime dependency cleanup. Verify environment selection and volume
ownership first; see [Environments](docs/ENVIRONMENTS.md).

Keep the README short and user-facing. Put detailed installation, architecture,
development, [module](docs/MODULES.md), [command](docs/COMMANDS.md), and browser
behavior in their guides. Update examples when flags/defaults change, preserve
relative links, and label local unreleased functionality explicitly. In review
notes, summarize the behavior change, checks actually run, remaining limits, and
any generated-state or data compatibility risk.
