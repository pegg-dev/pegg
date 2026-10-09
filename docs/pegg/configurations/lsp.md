---
icon: lucide/code
---

# LSP

Pegg can run **language servers** (LSP) and inject their diagnostics into the
content the agent reads and writes, giving the model compile errors and warnings
right where it can act on them.

## How it works

- Language servers start **lazily**: the first file access matching a server's
  `filetypes` triggers the server for that project root.
- Before every file read, cached diagnostics are appended to the returned content:

  ```
  ---
  Diagnostics:
    src/main.go:12:5: undefined: Foo [severity=1]
  ```

- After every file write, diagnostics for the written file are appended the same
  way. When a file is deleted, its diagnostics are dropped.
- Idle servers are stopped after **10 minutes** to save resources.

This requires a language server that matches the file type. Without one, reads and
writes behave normally.

## Configuration

=== "Global"

    The `language_servers` map in `~/.peggco/pegg.json`:

    ```json
    {
      "language_servers": {
        "gopls": {
          "command": "gopls",
          "filetypes": ["go"],
          "rootMarkers": ["go.mod"]
        }
      }
    }
    ```

=== "Project"

    Create `.lsp.json` in the project root:

    ```json
    {
      "languageServers": {
        "gopls": { "command": "gopls", "filetypes": ["go"], "rootMarkers": ["go.mod"] }
      }
    }
    ```

### Server options

| Key | Type | Description |
|---|---|---|
| `command` | string | Language server executable |
| `args` | string[] | Command arguments |
| `env` | object | Extra environment variables |
| `filetypes` | string[] | File extensions this server handles |
| `rootMarkers` | string[] | Files used to find the project root (for example `go.mod`, `.git`) |
| `download` | string | Static binary URL to fetch when `command` is not installed |
| `install` | string | Shell command to install the server when missing |
| `version` | string | Informational version |
| `enabled` | bool | `false` removes a server inherited from a lower-priority layer |

### Precedence

Servers are merged in three layers, later layers overriding earlier ones:

1. **Bundled** — built-in definitions shipped with Pegg.
2. **Global** — `language_servers` in `pegg.json`.
3. **Project** — `languageServers` in `.lsp.json`.

A project entry with `"enabled": false` **removes** a server defined in the bundled
or global layer.

## Bundled servers

Pegg bundles definitions for more than 30 language servers, including: `astro`,
`bash`, `clangd`, `csharp`, `clojure-lsp`, `dart`, `deno`, `elixir-ls`, `eslint`,
`fsharp`, `gleam`, `gopls`, `hls` (Haskell), `jdtls` (Java), `julials`, `kotlin-ls`,
`lua-ls`, `nixd`, `ocaml-lsp`, `oxlint`, `php`, `prisma`, `pyright`, `razor`,
`ruby-lsp`, `rust-analyzer`, `sourcekit-lsp` (Swift), `svelte`, `terraform`,
`tinymist` (Typst), `typescript`, `vue`, `yaml-ls`, and `zls` (Zig).

Each bundled definition includes a `download` URL or `install` command so the server
can be set up automatically when missing.

## Managing servers

```bash
pegg lsp list                  # bundled + global + project
pegg lsp list --global         # global only
pegg lsp list --project        # project only
```

### Adding a server

```bash
pegg lsp add --project --name gopls --command gopls \
  --filetype go --rootmarker go.mod
```

| Flag | Description |
|---|---|
| `--global` / `--project` | Where to write the server |
| `--name` | Server name |
| `--command` | Executable |
| `--arg` | Argument (repeatable) |
| `--filetype` | File extension to serve (repeatable) |
| `--rootmarker` | Root marker file (repeatable) |
| `--download` | Static binary URL for automatic installation |
| `--env` | `KEY=VALUE` environment variable (repeatable) |

Without flags, `lsp add` walks you through the same fields interactively.

### Removing a server

```bash
pegg lsp remove gopls
```

The scope is auto-detected when the server exists in only one place, and prompted
for when it exists in both.

### Diagnostics

```bash
pegg lsp diag --file src/main.go
```

Prints cached diagnostics as `file:line:col message` (1-based line and column), or
`no diagnostics for "<file>"`.

## Binary resolution

When a configured server's command is not on `PATH`, Pegg looks for it in this
order:

1. `~/.pegg/lsps/` — the local download cache.
2. `PATH` and common tool directories (`GOBIN`, `GOPATH/bin`, `~/go/bin`,
   `~/.cargo/bin`, `~/.deno/bin`, rustup toolchains, and similar).
3. The `download` URL (archives up to 256 MiB; zip/tar/tgz are extracted).
4. The `install` shell command (5 minute timeout).

Run `pegg lsp list` and `pegg files` to confirm which servers are present and
where binaries are cached.