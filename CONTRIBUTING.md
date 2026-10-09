# Contributing to Pegg

Thanks for your interest in contributing to Pegg. This guide covers everything you need to know to get up and running, make changes, and submit a pull request that gets merged.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Getting the Code](#getting-the-code)
- [Project Layout](#project-layout)
- [Build & Run](#build--run)
- [Making Changes](#making-changes)
- [Testing](#testing)
- [Linting & Formatting](#linting--formatting)
- [Documentation](#documentation)
- [Submitting a Pull Request](#submitting-a-pull-request)
- [Reporting Issues](#reporting-issues)
- [Style Guide](#style-guide)

---

## Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| **Go** | 1.26.5+ | Check with `go version` |
| **golangci-lint** | latest | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` |
| **gofmt** | bundled | Ships with Go |
| **git** | 2.x+ | Standard |

No runtime dependencies. The binary is fully self-contained (`CGO_ENABLED=0`).

---

## Getting the Code

```bash
# Fork the repository on GitHub, then:
git clone https://github.com/<your-username>/pegg.git
cd pegg
git remote add upstream https://github.com/peggco/pegg.git
```

Keep your fork in sync:

```bash
git fetch upstream
git rebase upstream/main
```

---

## Project Layout

```
pegg/
├── cmd/pegg/          # Main binary entry point
├── internal/
│   ├── agent/           # Orchestrator, subagents, hooks
│   ├── llm/             # Provider drivers, circuit breakers, rate limits
│   ├── tools/           # Bash, file ops, web search/fetch, LSP/MCP
│   ├── skill/           # Dynamic skill engine
│   ├── session/         # SQLite persistence, context manager
│   └── ...
├── pkg/sdk/             # Embeddable Go SDK (public API)
├── docs/                # Documentation site (zensical)
├── assets/              # Static assets (icons, etc.)
├── .github/workflows/   # CI/CD pipelines
├── .pegg/             # Project config, rules, subagent state
├── Makefile             # Dev commands
└── go.mod               # Module definition
```

- **`internal/`** — core packages, not importable outside this module.
- **`pkg/sdk/`** — public Go SDK, stable API surface.
- **`docs/`** — documentation source. Built with [zensical](https://zensical.dev).

---

## Build & Run

```bash
# Build the binary
make build

# Run the TUI
make run

# Run in debug mode
PEGG_DEBUG=1 ./bin/pegg
```

---

## Making Changes

1. **Create a branch** from `main`:

   ```bash
   git checkout -b your-branch-name main
   ```

   Use a descriptive name: `fix/rate-limit-retry`, `feat/mcp-timeout`, `docs/sdk-examples`.

2. **Make your changes.** Keep commits focused — one logical change per commit.

3. **Run the checks** before pushing:

   ```bash
   make fmt     # Format code
   make vet     # Static analysis
   make lint    # Lint with golangci-lint
   make test    # Run full test suite
   ```

   All four must pass. CI will run them on your PR automatically.

4. **Update documentation** if your change affects user-facing behavior:
   - CLI flags or commands → update `docs/pegg/usage/cli.md`
   - Provider/model changes → update `docs/pegg/providers-and-models.md`
   - New tool or feature → update the relevant doc under `docs/`
   - SDK API changes → update `docs/sdk/`

5. **Write tests** for new functionality. See [Testing](#testing).

---

## Testing

```bash
# Run all tests
make test

# Run a specific test
go test -v ./internal/agent/... -run TestAgentCreation

# Run tests in a single package
go test -v ./internal/llm/...

# Run tests with race detection
go test -race ./...
```

### Writing Tests

- Place test files next to the code they test: `foo.go` → `foo_test.go`.
- Use table-driven tests where multiple cases apply.
- Mock external dependencies (LLM APIs, filesystem) — never call real APIs in tests.
- Use `t.Helper()` in test helper functions.
- Aim for meaningful coverage, not 100% line coverage. Test behavior, not implementation.

### What CI Checks

| Job | What it does |
|-----|-------------|
| **Test** | `go test -v ./...` |
| **Format** | `gofmt -w .` — auto-commits if files need formatting |
| **Vet** | `go vet ./...` |

All three run on every push and pull request to `main`.

---

## Linting & Formatting

```bash
# Auto-format all Go files
make fmt

# Run static analysis
make vet

# Run golangci-lint
make lint
```

### Rules

- **`gofmt`** is the canonical formatter. Do not fight it.
- **`go vet`** catches suspicious constructs (unused variables, bad format strings, etc.).
- **`golangci-lint`** runs additional checks. Fix everything it reports.

Format before every commit. If your PR has formatting changes mixed with logic changes, reviewers can't see what actually changed.

---

## Documentation

The docs live in `docs/` and are built with [zensical](https://zensical.dev). The site is published to [docs.pegg.dev](https://docs.pegg.dev).

### When to Update Docs

Update documentation when your change:

- Adds, removes, or renames a CLI flag or command
- Changes default behavior
- Adds a new provider, tool, or feature
- Changes configuration format or options
- Modifies SDK public API

### How to Preview Locally

```bash
pip install zensical markdown-exec
zensical build --clean
# Open the generated site/ directory
```

### Doc File Conventions

- Each file has YAML frontmatter with an icon field:
  ```yaml
  ---
  icon: lucide/rocket
  ---
  ```
- Use `===` blocks for tabbed content (e.g., install methods).
- Reference commands and code in fenced blocks with language tags.
- Cross-link between docs using relative markdown paths.

---

## Submitting a Pull Request

1. **Push your branch** to your fork:

   ```bash
   git push origin your-branch-name
   ```

2. **Open a pull request** against `main` on the upstream repository.

3. **PR description** — tell us:
   - What changed and why
   - How to test it
   - Related issues (use `Fixes #123` to auto-close)

4. **Checklist** — make sure you've done the basics:

   - [ ] `make fmt` — code is formatted
   - [ ] `make vet` — no static analysis warnings
   - [ ] `make lint` — golangci-lint passes
   - [ ] `make test` — all tests pass
   - [ ] New code has tests (if applicable)
   - [ ] Docs are updated (if applicable)
   - [ ] Commit messages are clear

5. **Respond to review feedback.** Push additional commits to your branch — they'll appear in the PR automatically.

### Commit Messages

Use clear, descriptive commit messages:

```
fix: retry on provider rate limit instead of failing immediately

The agent was returning ErrProviderRateLimit without retrying.
Added exponential backoff with 3 retries before giving up.

Fixes #42
```

Format:
- **`feat:`** — new feature
- **`fix:`** — bug fix
- **`docs:`** — documentation only
- **`test:`** — adding or updating tests
- **`chore:`** — maintenance, dependencies, CI config
- **`refactor:`** — code change that neither fixes a bug nor adds a feature
- **`perf:`** — performance improvement

---

## Reporting Issues

Open an issue on [GitHub Issues](https://github.com/peggco/pegg/issues). Include:

- **What you expected** to happen
- **What actually** happened
- **Steps to reproduce** the problem
- **Your environment**: OS, terminal, Go version, Pegg version

For feature requests, use the [Discussions](https://github.com/peggco/pegg/discussions) board.

---

## Style Guide

### Go

- Follow [Effective Go](https://go.dev/doc/effective_go) conventions.
- PascalCase for exported names, camelCase for unexported.
- Keep functions short and focused. If a function does two things, it should be two functions.
- Return errors, don't panic. Use domain-specific error types where appropriate.
- Document exported functions and types with godoc comments.
- No unused imports. No unnecessary variables.

### Architecture

- Agents communicate through the event bus (WordPress-style hooks).
- Each tool is self-contained in its own package under `internal/tools/`.
- LLM providers implement the common `Driver` interface — adding a new provider means implementing one interface.
- Middleware runs in a chain: loop detection → secret redaction → retry → permissions.
- Sessions persist state in SQLite. Don't store mutable state in global variables.

### Files

- Keep package-level state minimal.
- Prefer dependency injection over global access.
- Test files live next to the code they test.
- Internal packages stay internal — don't export things from `internal/` that don't need to be exported.

---

## License

By contributing to Pegg, you agree that your contributions will be licensed under the [FSL-1.1-MIT](./LICENSE.md).

---

## Questions?

Join the [Discord](https://discord.gg/pegg) → `#contributors` channel. We're happy to help you get started.
