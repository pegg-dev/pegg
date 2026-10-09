# Release notes — additional instructions

Appended to peggbot's default release-notes prompt. Pegg formatting rules:

- Sections: `## New Features & Enhancements`, `## Bug Fixes`,
  `## Refactors & Under the Hood`, `## Documentation & Chores`, separated by
  `---`.
- Use `###` subsections by area: `### Terminal User Interface (TUI)`,
  `### Core & Providers`, `### Architecture & Cleanup`, `### CI & Releases`.
- Bullets start with a bold label, e.g.
  `* **Rendering Optimization:** Introduced incremental chat rendering.`
- Group version-tag commits under Documentation & Chores → Version Bumps,
  listing the exact versions (e.g. `0.1.2.1` and `0.1.3`).
- End with a short "Full changelog" compare link:
  `https://github.com/peggco/pegg/compare/{prev}...{new}`.