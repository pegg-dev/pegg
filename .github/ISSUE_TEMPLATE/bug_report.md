---
name: Bug Report
about: Report a bug in Pegg
title: "[BUG] "
labels: bug
assignees: "omerfdmrl"
---

<!--- Please use this template for reporting bugs. Search existing issues first. -->

## Environment

<!--- Paste the output of `pegg version` below. -->

- Pegg version: [e.g. `pegg version 0.1.5`]
- Install method: [install script / npm / `go install` / build from source / release binary]
- OS & architecture: [e.g. macOS 15 arm64, Ubuntu 24.04 amd64, Windows 11 amd64]
- Interface: [TUI / `pegg run` / `pegg serve` (HTTP API) / ACP server / Go SDK]
- Terminal (TUI or `pegg run` only): [e.g. iTerm2, Ghostty, Windows Terminal — include version]
- Go version (source builds only): [e.g. go1.26.5]

## Severity / Priority

<!--- How critical is this bug? -->

- [ ] Low — cosmetic or minor annoyance
- [ ] Medium — degrades a feature, but a workaround exists
- [ ] High — the feature is unusable, no workaround
- [ ] Critical — crash, data loss, workspace/sandbox escape, or leaked secrets

## Expected Behavior

<!--- Tell us what should happen -->

## Current Behavior

<!--- Tell us what happens instead of the expected behavior. Paste the exact error text. -->
<!--- Command failures are printed to stderr as `pegg: <error>`. -->

## Steps to Reproduce

<!--- Provide an unambiguous set of steps. Include the exact command or prompt that triggers it. -->

1.
2.
3.
4.

## Logs / Output

<!--- Attach whatever is relevant and available. -->
<!--- `pegg logs --level ERROR` (`--level DEBUG` for more detail) -->
<!--- `pegg doctor` for provider connectivity -->
<!--- `pegg config show` for the resolved config — API keys are masked, but redact -->
<!--- anything else you consider private before posting. -->

## Context

<!--- How has this issue affected you? What are you trying to accomplish? -->
<!--- Providing context helps us come up with a solution that is most useful in the real world. -->
<!--- Does it reproduce every time? Is there a workaround? -->

## Possible Solution

<!--- Not obligatory, but suggest a fix or point at the code you think is responsible. -->

## Additional Context

<!--- Screenshots, recordings, related issues, or a minimal repository that reproduces it. -->