---
icon: lucide/bot-message-square
---

# Providers & Models

Pegg talks directly to LLM APIs using your own credentials. A **provider** is a
named, preconfigured endpoint; a **driver** is the wire protocol it speaks.

## Providers

36 providers are registered out of the box. Every provider exposes a model list that
Pegg fetches at startup and caches locally.

| Provider | Driver | Default base URL |
|---|---|---|
| `ai21` | openai | `https://api.ai21.com/studio/v1` |
| `anthropic` | claude | `https://api.anthropic.com` |
| `anyscale` | openai | `https://api.endpoints.anyscale.com/v1` |
| `baichuan` | openai | `https://api.baichuan-ai.com/v1` |
| `cerebras` | openai | `https://api.cerebras.ai/v1` |
| `cohere` | openai | `https://api.cohere.com/compatibility/v1` |
| `deepinfra` | openai | `https://api.deepinfra.com/v1/openai` |
| `deepseek` | openai | `https://api.deepseek.com/v1` |
| `fireworks` | openai | `https://api.fireworks.ai/inference/v1` |
| `google` | gemini | `https://generativelanguage.googleapis.com/v1beta` |
| `groq` | openai | `https://api.groq.com/openai/v1` |
| `lepton` | openai | `https://api.lepton.ai/v1` |
| `minimax` | openai | `https://api.minimax.chat/v1` |
| `mistral` | openai | `https://api.mistral.ai/v1` |
| `moonshot` | openai | `https://api.moonshot.cn/v1` |
| `novita` | openai | `https://api.novita.ai/v3/openai` |
| `nvidia` | openai | `https://integrate.api.nvidia.com/v1` |
| `openai` | openai | `https://api.openai.com/v1` |
| `opencode-go` | openai | `https://opencode.ai/zen/go/v1` |
| `opencode-zen` | openai | `https://opencode.ai/zen/v1` |
| `openrouter` | openai | `https://openrouter.ai/api/v1` |
| `perplexity` | openai | `https://api.perplexity.ai` |
| `sambanova` | openai | `https://api.sambanova.ai/v1` |
| `stepfun` | openai | `https://api.stepfun.com/v1` |
| `tencent` | openai | `https://hunyuan.cloud.tencent.com/v1` |
| `together` | openai | `https://api.together.xyz/v1` |
| `upstage` | openai | `https://api.upstage.ai/v1/solar` |
| `voyage` | openai | `https://api.voyageai.com/v1` |
| `writer` | openai | `https://api.writer.com/v1` |
| `xai` | openai | `https://api.x.ai/v1` |
| `yi` | openai | `https://api.lingyiwanwu.com/v1` |
| `zhipu` | openai | `https://open.bigmodel.cn/api/paas/v4` |

### Authentication

| Driver | Auth mechanism |
|---|---|
| openai | `Authorization: Bearer <api_key>` |
| claude | `x-api-key: <api_key>` plus `anthropic-version: 2023-06-01` |
| gemini | `?key=<api_key>` query parameter |

Requests also send `HTTP-Referer: https://github.com/peggco/pegg` and
`X-Title: pegg` unless you override the headers in config.

### Subscriptions

Some providers use an existing **subscription** instead of an API key, reusing the
OAuth credentials that the official CLI already stored after you signed in with it.
Pegg never performs the OAuth login itself and never writes to those files except
to refresh an expiring token.

| Provider | Signs in with | Credentials |
|---|---|---|
| `claude` | `claude` (Claude Code) | `~/.claude/.credentials.json` (or `$CLAUDE_CONFIG_DIR`) |
| `codex` | `codex` | `~/.codex/auth.json` (or `$CODEX_HOME`) |
| `cline` | `cline auth` | `~/.cline/data/settings/providers.json` (or `$CLINE_DATA_DIR`) |
| `cline-pass` | `cline auth` (ClinePass) | `~/.cline/data/settings/providers.json` |

Cline exposes two separate providers: `cline` is pay-as-you-go (billed against
your account balance) and `cline-pass` uses your ClinePass subscription. The
ClinePass models are prefixed `cline-pass/` and are listed by the `cline-pass`
provider.

```bash
# sign in with the official CLI first
claude        # or: codex   /   cline auth

# then enable the provider in pegg
pegg login --provider claude

# remove it from pegg (the official CLI credentials are left untouched)
pegg logout --provider claude
```

In the TUI, open **Settings → General → Provider**, pick a subscription provider,
and follow the prompt. If you are not signed in, pegg shows the exact command to
run and lets you retry. Tokens are refreshed automatically before they expire and
the rotated tokens are written back to the official CLI's file.

## Adding a provider

### Interactive

```bash
pegg login
```

Pick a provider from the list and paste the API key (input is masked). The key may be
left empty for endpoints that do not need one.

### Non-interactive

```bash
pegg login --provider groq --api-key gsk_...
```

The provider is synced first — Pegg fetches its model list and only saves the
provider if that succeeds (35 second timeout). This guarantees a freshly added
provider has usable models immediately.

### CLI with full options

`pegg providers add` exposes every config field (driver, base_url, headers,
timeout, max_retries):

```bash
pegg providers add --name groq --api-key gsk_...
pegg providers add --driver openai --base-url http://localhost:11434/v1 --api-key ollama --timeout 120
```

Registered names use built-in endpoints; `base_url`/`driver` are ignored for
them. For custom endpoints, use a bare driver entry or combine a custom name
with a driver:

```bash
pegg providers add --driver openai --base-url http://localhost:11434/v1 --api-key ollama --timeout 120
pegg providers add --name my-gw --driver openai --base-url http://localhost:11434/v1 --api-key ollama
```

`providers add` runs without prompts when any of `--name`, `--driver`, or
`--base-url` is given; headers default to empty and the API key may be left
empty. With no flags it starts the interactive picker, which also offers a
"Custom endpoint" option. Models are fetched in the background after saving —
a failed fetch does not block the add, run `pegg providers refresh` to retry.

### TUI

Open Settings with `Ctrl+P` → **General** → **Provider**. Existing providers show
*Use existing configuration* / *Configure again*; new ones prompt for an API key.

### Config file

Keys are stored in plaintext in `~/.peggco/pegg.json`:

```json
{
  "providers": [
    { "provider": "openai", "api_key": "sk-..." },
    { "provider": "groq", "api_key": "gsk_...", "timeout": 60 }
  ]
}
```

Keys are masked in `pegg providers list` and `pegg config show`.

## Custom and OpenAI-compatible endpoints

To use a gateway, proxy, or local runtime, configure a raw **driver** instead of a
named provider. `base_url` is required for drivers:

```json
{
  "providers": [
    {
      "driver": "openai",
      "base_url": "http://localhost:11434/v1",
      "api_key": "ollama",
      "timeout": 120,
      "headers": { "X-Custom": "value" }
    }
  ]
}
```

The same works for `"driver": "claude"` and `"driver": "gemini"` endpoints that
implement those wire protocols.

The same entry can be added from the CLI:

```bash
pegg providers add --driver openai --base-url http://localhost:11434/v1 --api-key ollama
```

## Managing providers

```bash
pegg providers add --name anthropic --api-key your-api-key # add or replace a provider
pegg providers list                    # name, masked key, cached model count
pegg providers refresh                 # re-fetch models for every provider
pegg providers refresh --provider xai  # re-fetch one provider
pegg providers remove groq             # remove provider and its cached models
pegg models                            # list cached models
pegg models --provider deepseek        # filter by provider
```

Model lists are cached in `~/.pegg/cache.db` (or `cache.json` with the JSON cache
driver). `pegg cache clear` empties the cache; `pegg providers refresh` bypasses
it and fetches from the network.

## Decision models

Alongside chat models, Pegg can use **decision models** — models that answer
typed questions (yes/no, choice, or an ordered score) about a piece of state
instead of generating text. The first supported decision model is
**JEV** (`typesafe/jev-1.13`) from TypeSafe, served through OpenRouter's
Decisions API (`https://openrouter.ai/api/alpha/decisions`).

Decision models reuse the regular provider configuration: the `openrouter`
provider entry and its API key are shared, so one login covers both chat and
decisions. There is no separate model list to fetch — the model ID is passed
per request.

Pegg currently uses decision models for the **permission judge**: when an
OpenRouter API key is configured, the judge uses JEV by default (even without
`judge_provider` set), and falls back to the judge LLM on any error. Configure
the behavior with `judge_provider`, `judge_model`, and `judge_threshold` — see
[Permissions](features/permissions.md#the-judge-flow) and
[Config](config.md#permission).

```json
{
  "providers": [{ "provider": "openrouter", "api_key": "sk-or-..." }],
  "permission": {
    "judge_provider": "openrouter",
    "judge_model": "typesafe/jev-1.13",
    "judge_threshold": 0.8
  }
}
```

## Smart router

The **smart router** is a system model that picks the best model for each agent
and task at run time, instead of using one model for everything. It is backed by a
decision model — JEV by default, or your preferred LLM if no decision provider has
an API key — and falls back to the preferred model when nothing is available.

Enable it in config (`smart_router.enabled`), then pick **Smart Router** in the
TUI at Settings → General → Model (listed above the regular models) or pass
`--model smart-router` on the CLI. When enabled, subagents (planner, explorer,
developer, ...) are also routed automatically.

Per-agent preferences under `smart_router.agents` support three layers:
`default` model lists, per-`difficulty` lists (`trivial`/`moderate`/`complex` —
the decision model scores the task first and routes to the matching bucket), and
`images` lists for vision-capable variants. Models without image support are
excluded automatically when the run has image attachments. The decision model's
pick is always used — there is no confidence threshold. See
[Config](config.md#smart_router).

## Model selection

When a run starts, Pegg resolves a provider/model in this order:

1. **Explicit** — `--provider` and `--model` must match a known model (by ID or
   display name), otherwise the run fails.
2. **Model only** — the model is searched across all providers. A single match is
   used automatically; multiple matches open a picker.
3. **Provider only** — a picker lists that provider's models.
4. **Preferred** — the provider/model of the most recent non-subagent session in the
   current project directory, falling back to the first model of the first provider.
   Resolution has a 30 second timeout.

```bash
pegg run --provider openai --model gpt-4o "Explain this error"
pegg run --model deepseek-chat "Refactor this function"
pegg run --select-model "Do something"
```

The same resolution logic backs the HTTP `/api/run` endpoint and the ACP server.

## Model metadata

Model lists are enriched with metadata fetched and cached under the `models_cache` key. For
each model this includes:

| Field | Used for |
|---|---|
| `max_input_tokens`, `max_output_tokens`, `max_tokens` | Context-window usage indicator and request limits |
| `input_cost_per_token`, `output_cost_per_token`, `cache_read_cost_per_token` | Cost display in the TUI status bar and session usage |
| `supports_reasoning`, `reasoning_options` | The TUI **Reasoning** setting (`default`, `low`, `medium`, `high`) |
| `modalities.input` | Whether image/audio attachments are allowed |
| `tool_call`, `structured_output`, `temperature` | Capability checks |

Lookup tries `provider/model` first, then the bare model ID, then a substring
fallback. Unknown models still work; they simply have no metadata.

## Reasoning effort

Models that support reasoning expose effort levels. Set it per run in the TUI
(Settings → General → Reasoning) or via the SDK's `ReasoningEffort` field. The value
is persisted with the session and shown in the status bar.

## Attachments

Image and audio attachments are validated against the model's input modalities before
sending. If the model does not support them, Pegg refuses the attachment with an
error instead of failing at the API. See
[Adding Context](features/adding-context.md).

## Failures and retries

The built-in `retry` middleware wraps every LLM call in all agents:

- Temporary provider errors (HTTP `429` and `5xx`) are retried with a backoff of
  **1s, 3s, 10s, 30s, 1m, 5m**, then every 5 minutes.
- Other errors are retried too, unless the run was cancelled.
- Streaming calls are only retried before any content, reasoning, or tool call has
  been emitted.
- While waiting, the CLI and TUI show a transient
  `Request failed (attempt N): ... retrying in Xs` message.

There is currently no automatic failover to a secondary provider — configure retries
and switch models manually if a provider is down.

## Troubleshooting

```bash
pegg doctor                            # connectivity + model count per provider
pegg logs --level ERROR                # recent errors
pegg providers refresh --provider X    # force a fresh model fetch
```

See [Troubleshooting](troubleshooting.md) for details.
