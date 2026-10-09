# Solve issue — additional instructions

Appended to peggbot's default solve-issue prompt. Repository rules for pegg:

- Build and verify with `make build`, `make test`, `make lint`, `make vet`.
- Keep code `gofmt`-clean and conventional-commit messages (`fix: ...`,
  `feat: ...`).
- Architecture: agent logic lives in `internal/agent/`, LLM providers in
  `internal/llm/`, tools in `internal/tools/`, SDK surface in `pkg/sdk/`.
  Keep changes in the right layer.
- Changes to `internal/agent/` or `internal/llm/` are sensitive: keep them
  minimal and add tests (`go test -v ./internal/agent/...`).
- Never edit `.pegg/`, `.env` files, or any `secrets` files.
- If a fix needs a new config field, add the default to the config package
  and document it under `docs/`.