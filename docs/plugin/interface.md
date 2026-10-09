---
icon: lucide/code
---

# Interface Reference

This page documents the plugin interface, types, and method signatures.

## Import

```go
import "github.com/peggco/pegg/internal/plugin/shared"
```

## Plugin Interface

Every plugin must implement this interface:

```go
type Plugin interface {
    Name() string
    Version() string
    Description() string
    Boot(deps Deps) error
}
```

### Methods

#### `Name() string`

Returns the unique name of the plugin. This name is used for:

- Identifying the plugin in `pegg plugin list`
- Excluding plugins via configuration
- Logging

**Requirements:**

- Must be non-empty
- Should be unique across all installed plugins
- Convention: lowercase, hyphen-separated (e.g., `my-cool-plugin`)

#### `Version() string`

Returns the plugin version in semver format.

**Requirements:**

- Must be non-empty
- Should follow semantic versioning (e.g., `1.0.0`)

#### `Description() string`

Returns a human-readable description of what the plugin does.

**Requirements:**

- Must be non-empty
- Should be concise (one line preferred)

#### `Boot(deps Deps) error`

Called when the plugin is loaded. Receives application dependencies and performs
initialization.

**Parameters:**

| Name | Type | Description |
|---|---|---|
| `deps` | `Deps` | Dependencies struct containing application config |

**Returns:**

| Type | Description |
|---|---|
| `error` | Non-nil error causes plugin to fail loading |

**Behavior:**

- Called once when the plugin is first loaded
- Called in the plugin's separate process
- Return `nil` for successful initialization
- Return an error to fail plugin loading (Pegg continues without the plugin)

## Deps Struct

Contains dependencies passed to the plugin's `Boot` method.

```go
type Deps struct {
    Config *config.Config
}
```

### Fields

| Field | Type | Description |
|---|---|---|
| `Config` | `*config.Config` | Application configuration |

### Config Access

The `Config` field provides access to the full Pegg configuration:

```go
func (p *MyPlugin) Boot(deps shared.Deps) error {
    cfg := deps.Config

    // Access providers
    for _, provider := range cfg.Providers {
        fmt.Printf("Provider: %s (driver: %s)\n", provider.Provider, provider.Driver)
    }

    // Access theme
    fmt.Printf("Theme: %s\n", cfg.Theme)

    // Access plugin config
    fmt.Printf("Plugins enabled: %v\n", cfg.Plugins.Enabled)
    fmt.Printf("Excluded plugins: %v\n", cfg.Plugins.Exclude)

    // Access server config
    fmt.Printf("Server: %s:%d\n", cfg.Server.Host, cfg.Server.Port)

    return nil
}
```

### Config Fields

| Field | Type | Description |
|---|---|---|
| `Providers` | `[]LLMConfig` | LLM provider configurations |
| `Logger` | `LoggerConfig` | Logger settings |
| `Cache` | `CacheConfig` | Cache settings |
| `Session` | `SessionConfig` | Session settings |
| `Server` | `ServerConfig` | HTTP server settings |
| `Theme` | `string` | UI theme name |
| `Permission` | `*PermissionConfig` | Permission settings |
| `Plugins` | `PluginConfig` | Plugin system settings |
| `MCPServers` | `map[string]MCPServerConfig` | MCP server configurations |
| `LanguageServers` | `map[string]LanguageServerConfig` | LSP configurations |

## RPC Communication

Plugins communicate with Pegg via RPC using
[HashiCorp go-plugin](https://github.com/hashicorp/go-plugin). The RPC layer is
handled automatically — you only need to implement the `Plugin` interface.

### Handshake

The handshake configuration is pre-defined in the shared package:

```go
var Handshake = plugin.HandshakeConfig{
    ProtocolVersion:  1,
    MagicCookieKey:   "PEGG_PLUGIN",
    MagicCookieValue: "pegg",
}
```

### Plugin Registration

Register your plugin with the Pegg plugin map:

```go
plugin.Serve(&plugin.ServeConfig{
    HandshakeConfig: shared.Handshake,
    Plugins: map[string]plugin.Plugin{
        "pegg": &shared.PeggPluginRPC{
            Impl: &MyPlugin{},
        },
    },
    Logger: logger,
})
```

## Error Handling

### PluginError

A custom error type for plugin-specific errors:

```go
type PluginError struct {
    Message string
}

func (e *PluginError) Error() string {
    return e.Message
}
```

Use this for domain-specific errors:

```go
func (p *MyPlugin) Boot(deps shared.Deps) error {
    if len(deps.Config.Providers) == 0 {
        return &shared.PluginError{
            Message: "no providers configured",
        }
    }
    return nil
}
```

### Boot Errors

If `Boot` returns an error:

1. The plugin process is terminated
2. A warning is logged
3. Pegg continues without the plugin

This ensures plugin failures don't crash the application.
