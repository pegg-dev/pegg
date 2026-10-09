---
icon: lucide/download
---

# Pegg Installation

Pegg ships as a single static binary with no runtime dependencies. Release builds
are produced for Linux, macOS, and Windows on both `amd64` and `arm64`.

## Requirements

| | |
|---|---|
| **Runtime** | None — the binary is self-contained (`CGO_ENABLED=0`) |
| **Build from source** | Go 1.26.5 or newer |
| **Terminal** | Any modern terminal. The TUI uses 24-bit color and mouse/paste support when available |

## Install

=== "Install script"

    ```bash
    curl -fsSL https://pegg.dev/install | bash
    ```

    Downloads the latest release binary for your platform and installs it to a
    directory on your `PATH`.

=== "npm"

    ```bash
    npm i -g @peggco/pegg
    ```

    Installs the `pegg` binary globally via npm. 

    - Pin a version: `PEGG_VERSION=v0.1.5 npm i -g @peggco/pegg`
    - Skipped install scripts (`--ignore-scripts`, or pnpm blocking build
      scripts) leave no binary; reinstall without them, or for pnpm run
      `pnpm approve-builds`.
    - npm 11+ may block scripts until approved:
      `npm i -g --allow-scripts=@peggco/pegg`.

=== "Prebuilt binary"

    Download the archive for your platform from the
    [GitHub releases page](https://github.com/peggco/pegg/releases):

    | Platform | Archive |
    |---|---|
    | Linux | `pegg_linux_amd64.tar.gz`, `pegg_linux_arm64.tar.gz` |
    | macOS | `pegg_darwin_amd64.tar.gz`, `pegg_darwin_arm64.tar.gz` |
    | Windows | `pegg_windows_amd64.zip`, `pegg_windows_arm64.zip` |

    ```bash
    # Example: Linux amd64
    tar -xzf pegg_linux_amd64.tar.gz
    sudo mv pegg /usr/local/bin/
    ```

    Each release also publishes `checksums.txt` so you can verify your download:

    ```bash
    sha256sum -c checksums.txt --ignore-missing
    ```

=== "go install"

    ```bash
    go install github.com/peggco/pegg/cmd/pegg@latest
    ```

    The binary is placed in `$(go env GOPATH)/bin`. Make sure that directory is on
    your `PATH`.

=== "Build from source"

    ```bash
    git clone https://github.com/peggco/pegg.git
    cd pegg
    make build        # produces ./bin/pegg
    make run          # or run it immediately
    ```

    Other useful targets: `make test`, `make lint`, `make fmt`, `make vet`,
    `make clean`.

## Verify

```bash
pegg version
# pegg version 0.1.0
```

## First run

The first launch creates the global config directory and a default config file, and
creates a project-local `.pegg/` directory in the current working directory.

```bash
pegg login
```

The login command opens an interactive provider picker, then asks for an API key
(masked). Keys are stored in `~/.peggco/pegg.json`. You can skip the interactive
prompt:

```bash
pegg login --provider openai --api-key sk-...
```

Once at least one provider is configured, start the TUI:

```bash
pegg
```

Or run a one-shot headless prompt:

```bash
pegg run "Summarize this repository"
echo "Fix the failing test" | pegg
```

See [CLI usage](usage/cli.md) for every command.

## Update

Pegg can update itself in place from GitHub releases:

```bash
pegg update
```

`pegg doctor` also checks for updates after testing provider connectivity and
offers to install a newer version when one is available.

## File locations

Run `pegg files` to print the resolved paths and whether each exists.

| Purpose | Path |
|---|---|
| Global config | `~/.peggco/pegg.json` |
| Logs | `~/.pegg/logs.db` (SQLite driver) |
| Model cache | `~/.pegg/cache.db` (SQLite driver) |
| Sessions | `~/.pegg/sessions.db` (SQLite driver) |
| Permission approvals | `~/.pegg/permissions.json` |
| Global skills | `~/.pegg/skills/` |
| Global rules | `~/.pegg/rules/` |
| Downloaded LSP binaries | `~/.pegg/lsps/` |
| Project data | `<project>/.pegg/` |
| Project MCP config | `<project>/.mcp.json` |
| Project LSP config | `<project>/.lsp.json` |

## Uninstall

```bash
rm "$(command -v pegg)"    # remove the binary
rm -rf ~/.pegg             # remove config, sessions, logs, cache, skills, rules
```

Project-local data lives in each project's `.pegg/` directory and can be removed
individually.
