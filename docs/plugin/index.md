---
icon: lucide/puzzle
---

# Plugin System

Pegg's plugin system allows you to extend its functionality by writing standalone
Go binaries. Plugins run in separate processes and communicate with Pegg via RPC,
ensuring isolation and safety.

## Features

- **Process isolation** — Plugins run in their own processes. A crashing plugin
  won't bring down Pegg.
- **Simple interface** — Implement a single `Boot` method to receive application
  configuration.
- **Auto-discovery** — Place executables in `~/.pegg/plugins/` and they're loaded
  automatically.
- **Hot-pluggable** — Add or remove plugins without modifying Pegg's source code.

## Architecture

```
┌─────────────────────────────────┐         ┌─────────────────────────┐
│         Pegg Host             │         │      Plugin Process     │
│                                 │   RPC   │                         │
│  plugin.Module()                │◄───────►│  shared.Plugin.Boot()   │
│    ├─ Scan ~/.pegg/plugins/   │         │    └─ deps.Config       │
│    ├─ Launch plugin subprocess  │         │                         │
│    └─ Call Boot(deps)           │         │  Your plugin logic      │
└─────────────────────────────────┘         └─────────────────────────┘
```

## Quick start

See the [Quickstart Guide](quickstart.md) for a step-by-step tutorial.

## Documentation

| Page | Description |
|---|---|
| [Quickstart](quickstart.md) | Build your first plugin in 5 minutes |
| [Interface](interface.md) | Plugin interface, types, and method signatures |
| [Development Guide](development.md) | Step-by-step plugin development |

## Configuration

See [Plugin Configuration](../pegg/configurations/plugins.md) for configuration
options.
