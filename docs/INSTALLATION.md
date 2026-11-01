# Installation And Updates

Use the [project README](../README.md) for a quick start and the
[documentation index](README.md) for workflow guides. This guide covers choosing
a binary, prerequisites, installation trust, and update behavior.

## Choose The Right Binary

- Published binaries come from [GitHub Releases](https://github.com/mart337i/odooctl/releases).
- The current worktree requires Go 1.25.0 or newer and catalogs Odoo 12.0-20.0, with 19.0 the default selection.
- Latest MCP changes, Odoo 20 support, and advanced browser integration are local, unreleased pending features. Do not assume a release, PPA package, or `go install ...@latest` contains them.
- For these features, build the intended local checkout and use that binary's explicit path. Its `--help` output is authoritative for available commands and flags.

The CLI is cross-platform; the generated Odoo and browser environments use Linux
containers. Install Docker with a working Linux-container daemon and the Compose
plugin (`docker compose`) for container workflows. Docker Desktop users must
enable sharing/bind-mount access to their project directory.

Go is needed to compile the CLI, not to run a downloaded binary. Git, make, and
Go are needed for the source workflow. Building Odoo images requires network
access to source repositories, base images, and package registries. Enterprise
also requires authorized access to `odoo/enterprise`.

Host Node.js is not required for Docker browser operation. Python Playwright in
the Odoo image and the official Node Playwright MCP sidecar are separate options;
see [Browser](BROWSER.md) and [Playwright MCP](ai/PLAYWRIGHT_MCP.md).

## Installer Script

Linux/macOS quick install:

```bash
curl -fsSL https://raw.githubusercontent.com/mart337i/odooctl/main/install.sh | bash
```

This executes remote code from the moving `main` branch. Review the script first
or use a manual release download when that trust model is unsuitable. The script
queries the latest GitHub release, detects OS/architecture, and replaces
`/usr/local/bin/odooctl`, using `sudo` if that directory is not writable.

It attempts SHA-256 verification when `checksums.txt` is available. Verification
is skipped with a warning if the file, matching entry, or local hash utility is
missing. This is not mandatory integrity verification or a signature check.
The script does not install Docker, Go, or your Odoo environment.

## Manual Release Binary

Choose a tag and download the matching asset from that release:

| Host | Asset |
| --- | --- |
| Linux x86-64 | `odooctl-linux-amd64` |
| Linux ARM64 | `odooctl-linux-arm64` |
| macOS Intel | `odooctl-darwin-amd64` |
| macOS Apple Silicon | `odooctl-darwin-arm64` |
| Windows x86-64 | `odooctl-windows-amd64.exe` |

On Linux/macOS, mark the download executable, rename it to `odooctl`, and place
it in a directory on `PATH`, such as `~/.local/bin` or `/usr/local/bin`. System
directories may require administrator access. Keep the previous binary if you
need rollback; installation does not create a backup.

When a release supplies `checksums.txt`, compare the downloaded asset's SHA-256
with its exact entry using `sha256sum` or `shasum -a 256`. If checksums are absent,
decide whether to accept an unverified download. The current release workflow
uploads binaries but does not generate or upload `checksums.txt`; the existence
of `make checksums` alone does not guarantee release checksums.

## Windows

Download the Windows asset manually, or use PowerShell:

```powershell
Invoke-WebRequest -Uri "https://github.com/mart337i/odooctl/releases/latest/download/odooctl-windows-amd64.exe" -OutFile "$env:LOCALAPPDATA\Microsoft\WindowsApps\odooctl.exe"
Get-Command odooctl -All
odooctl --help
```

The example assumes that destination exists and is on `PATH`. Alternatively,
use a dedicated tools directory and add it to your user `PATH`. The command above
does not verify a checksum. If the release provides checksums, use
`Get-FileHash -Algorithm SHA256` and compare with the Windows asset entry.
Windows ARM64 is not a cross-built release target in the current Makefile.

## APT And PPA

For an Ubuntu series with an available package:

```bash
sudo add-apt-repository ppa:mart337i/odooctl
sudo apt update
apt-cache policy odooctl
sudo apt install odooctl
```

APT packaging does not imply this Ubuntu PPA supports arbitrary Debian releases.
Check the candidate version and Launchpad build status before relying on it.
The current Debian source package requires `golang-go (>= 1.25)` at build time;
build archives lacking Go 1.25 cannot build it with the declared dependencies.
Installing a successfully built binary package does not require the Go compiler.

PPA publishing is gated by repository variable `ODOOCTL_PPA_GO_125_READY == 'true'`
and release workflow conditions. This static configuration is not evidence that
the variable is enabled, a package was built, or a current version is available.
Use release binaries or a local source build when the package is unavailable.

## Go And Source Builds

With Go 1.25.0 or newer for the current source:

```bash
go install github.com/mart337i/odooctl@latest
```

`@latest` selects a published module version, not local edits. Go installs to
`GOBIN` if set, otherwise the `bin` directory under `GOPATH` (normally `~/go/bin`).
Add that directory to `PATH`; inspect `go env GOBIN GOPATH` if uncertain.

To build the intended checkout without changing the installed CLI:

```bash
git clone https://github.com/mart337i/odooctl.git
cd odooctl
make build
./bin/odooctl version
./bin/odooctl --help
```

For local pending features, use the worktree containing them rather than assuming
a fresh clone includes uncommitted changes. `make install` installs that checkout
to Go's binary directory. `make build` embeds `git describe --tags --always --dirty`
(or `dev` without Git metadata); `VERSION` can override it. Plain `go build` uses
the current `cmd/root.go` fallback `0.2.5`. `go install ...@latest` likewise does
not inject Makefile metadata and uses the fallback in its selected module version.
Version output alone is not feature evidence.

Vendored modules support builds without Go module downloads when synchronized.
They do not supply a Go compiler or make Docker image builds offline or fully
reproducible. See [Contributing](../CONTRIBUTING.md).

## Updates And Rollback

Inspect before updating:

```bash
odooctl update --check
odooctl update --dry-run
odooctl update --check --json
odooctl update
```

- Planning queries GitHub's latest release even for APT, Go, and source methods. Checks need network access; availability is based on version inequality, not semantic ordering or an APT candidate check.
- Detection tries Linux APT ownership, an explicit source directory, an enclosing checkout, then exported `GOBIN`/`GOPATH` paths; otherwise it selects release replacement. Default Go paths may not be detected if those variables are unset.
- Override with `--method auto|apt|release|go|source`; use `--source-dir /path/to/checkout` for source updates outside a recognized checkout. Inspect `odooctl update --help` in the selected binary first.
- APT runs `sudo apt update` and `sudo apt install --only-upgrade odooctl`. Go runs `go install github.com/mart337i/odooctl@latest`.
- Source runs `git pull --ff-only` then `make install` in the checkout. It follows the current branch's upstream, does not switch branches, and installs to Go's binary directory rather than replacing `bin/odooctl`.
- Non-release version strings, including typical dirty or ahead-of-tag builds, require `--force`. There is no independent clean-worktree or branch preflight; Git can still refuse a conflicting dirty tree, missing upstream, or non-fast-forward pull. `--force` does not reset Git, discard edits, or override fast-forward safety.
- Release updates verify SHA-256 if the release supplies `checksums.txt`; a supplied file missing the asset entry causes failure. Without the file, verification is skipped with a warning. Replacement needs write access beside the executable; Windows schedules replacement after process exit.
- `--json` returns the plan without executing, even without `--check`. No update method creates a rollback backup. Preserve local work and the old binary/package before proceeding.

After changing install methods, check `command -v odooctl` (PowerShell:
`Get-Command odooctl -All`), refresh the shell cache with `hash -r` where supported,
or open a new terminal. CLI updates do not migrate Odoo databases or regenerate
existing environments. Continue with [Environments](ENVIRONMENTS.md).
