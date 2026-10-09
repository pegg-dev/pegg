---
icon: lucide/hammer
---

# Development Guide

Step-by-step guide to building Pegg plugins.

## Project structure

A minimal plugin project:

```
my-plugin/
├── main.go
├── go.mod
└── go.sum
```

A more complex plugin:

```
my-plugin/
├── main.go
├── plugin.go        # Plugin implementation
├── config.go        # Configuration handling
├── go.mod
└── go.sum
```

## Step 1: Initialize the project

```bash
mkdir my-plugin
cd my-plugin
go mod init github.com/username/my-plugin

# Add dependencies
go get github.com/hashicorp/go-plugin@latest
go get github.com/peggco/pegg@latest
go get github.com/hashicorp/go-hclog@latest
```

## Step 2: Implement the Plugin interface

Create `plugin.go`:

```go
package main

import (
    "fmt"

    "github.com/peggco/pegg/internal/plugin/shared"
)

type MyPlugin struct {
    // Store dependencies for later use
    config *config.Config
}

func (p *MyPlugin) Name() string {
    return "my-plugin"
}

func (p *MyPlugin) Version() string {
    return "1.0.0"
}

func (p *MyPlugin) Description() string {
    return "A custom Pegg plugin"
}

func (p *MyPlugin) Boot(deps shared.Deps) error {
    // Store config for later use
    p.config = deps.Config

    // Validate configuration
    if len(deps.Config.Providers) == 0 {
        return fmt.Errorf("no LLM providers configured")
    }

    fmt.Println("Plugin initialized successfully!")
    return nil
}
```

## Step 3: Create the main entry point

Create `main.go`:

```go
package main

import (
    "os"

    "github.com/hashicorp/go-hclog"
    "github.com/hashicorp/go-plugin"
    "github.com/peggco/pegg/internal/plugin/shared"
)

func main() {
    // Create logger
    logger := hclog.New(&hclog.LoggerOptions{
        Name:   "my-plugin",
        Output: os.Stderr,
        Level:  hclog.Info,
    })

    // Serve the plugin
    plugin.Serve(&plugin.ServeConfig{
        HandshakeConfig: shared.Handshake,
        Plugins: map[string]plugin.Plugin{
            "pegg": &shared.PeggPluginRPC{
                Impl: &MyPlugin{},
            },
        },
        Logger: logger,
    })
}
```

## Step 4: Build and test

```bash
# Build
go build -o my-plugin .

# Test locally
./my-plugin --help

# Install
mkdir -p ~/.pegg/plugins
cp my-plugin ~/.pegg/plugins/
chmod +x ~/.pegg/plugins/my-plugin

# Verify
pegg plugin list
pegg plugin info my-plugin
```

## Example: Provider checker plugin

This plugin checks if configured LLM providers are valid.

```go
package main

import (
    "fmt"
    "os"

    "github.com/hashicorp/go-hclog"
    "github.com/hashicorp/go-plugin"
    "github.com/peggco/pegg/internal/core/config"
    "github.com/peggco/pegg/internal/plugin/shared"
)

type ProviderChecker struct {
    config *config.Config
}

func (p *ProviderChecker) Name() string {
    return "provider-checker"
}

func (p *ProviderChecker) Version() string {
    return "1.0.0"
}

func (p *ProviderChecker) Description() string {
    return "Validates LLM provider configuration"
}

func (p *ProviderChecker) Boot(deps shared.Deps) error {
    p.config = deps.Config

    // Check providers
    if len(deps.Config.Providers) == 0 {
        fmt.Println("Warning: No LLM providers configured")
        return nil
    }

    fmt.Printf("Found %d provider(s):\n", len(deps.Config.Providers))
    for i, provider := range deps.Config.Providers {
        fmt.Printf("  %d. %s (driver: %s)\n", i+1, provider.Provider, provider.Driver)
        if provider.APIKey == "" {
            fmt.Printf("     Warning: No API key set\n")
        }
    }

    return nil
}

func main() {
    logger := hclog.New(&hclog.LoggerOptions{
        Name:   "provider-checker",
        Output: os.Stderr,
        Level:  hclog.Info,
    })

    plugin.Serve(&plugin.ServeConfig{
        HandshakeConfig: shared.Handshake,
        Plugins: map[string]plugin.Plugin{
            "pegg": &shared.PeggPluginRPC{
                Impl: &ProviderChecker{},
            },
        },
        Logger: logger,
    })
}
```

## Example: Event logger plugin

This plugin logs all events published on the bus.

!!! note
    Event bus access requires a different architecture since interfaces can't be
    serialized over RPC. This example shows the pattern for plugins that need to
    interact with Pegg's event system.

```go
package main

import (
    "fmt"
    "os"

    "github.com/hashicorp/go-hclog"
    "github.com/hashicorp/go-plugin"
    "github.com/peggco/pegg/internal/plugin/shared"
)

type EventLogger struct{}

func (p *EventLogger) Name() string {
    return "event-logger"
}

func (p *EventLogger) Version() string {
    return "1.0.0"
}

func (p *EventLogger) Description() string {
    return "Logs Pegg events"
}

func (p *EventLogger) Boot(deps shared.Deps) error {
    fmt.Println("Event logger plugin loaded")
    fmt.Printf("Pegg config loaded with %d providers\n", len(deps.Config.Providers))

    // In a real plugin, you would connect to the event bus
    // via a custom RPC service or other mechanism

    return nil
}

func main() {
    logger := hclog.New(&hclog.LoggerOptions{
        Name:   "event-logger",
        Output: os.Stderr,
        Level:  hclog.Info,
    })

    plugin.Serve(&plugin.ServeConfig{
        HandshakeConfig: shared.Handshake,
        Plugins: map[string]plugin.Plugin{
            "pegg": &shared.PeggPluginRPC{
                Impl: &EventLogger{},
            },
        },
        Logger: logger,
    })
}
```

## Best practices

### Error handling

- Return descriptive errors from `Boot`
- Don't panic — return an error instead
- Log errors before returning them

```go
func (p *MyPlugin) Boot(deps shared.Deps) error {
    if deps.Config == nil {
        return fmt.Errorf("config is nil")
    }

    if len(deps.Config.Providers) == 0 {
        return fmt.Errorf("no providers configured: at least one provider is required")
    }

    return nil
}
```

### Graceful degradation

- Don't fail if optional features are missing
- Log warnings for non-critical issues

```go
func (p *MyPlugin) Boot(deps shared.Deps) error {
    if deps.Config.Permission == nil {
        fmt.Println("Warning: Permission config not set, using defaults")
    }

    return nil
}
```

### Configuration validation

Validate configuration early and provide clear error messages:

```go
func (p *MyPlugin) Boot(deps shared.Deps) error {
    // Validate required fields
    if deps.Config.Theme == "" {
        return fmt.Errorf("theme not set in config")
    }

    // Validate ranges
    if deps.Server.Port < 0 || deps.Server.Port > 65535 {
        return fmt.Errorf("invalid port: %d", deps.Server.Port)
    }

    return nil
}
```

### Logging

Use `fmt.Println` or `fmt.Fprintf` for user-facing output. The plugin's stderr
is forwarded to Pegg's logs.

```go
func (p *MyPlugin) Boot(deps shared.Deps) error {
    // User-facing output
    fmt.Println("Plugin initialized")

    // Debug output (goes to logs)
    fmt.Fprintf(os.Stderr, "DEBUG: config loaded with %d providers\n",
        len(deps.Config.Providers))

    return nil
}
```

## Testing

### Unit tests

Test your plugin logic independently:

```go
package main

import (
    "testing"

    "github.com/peggco/pegg/internal/core/config"
    "github.com/peggco/pegg/internal/plugin/shared"
)

func TestBoot(t *testing.T) {
    plugin := &MyPlugin{}

    deps := shared.Deps{
        Config: &config.Config{
            Providers: []config.LLMConfig{
                {Provider: "openai", Driver: "openai"},
            },
            Theme: "dark",
        },
    }

    err := plugin.Boot(deps)
    if err != nil {
        t.Fatalf("Boot failed: %v", err)
    }

    if plugin.config == nil {
        t.Fatal("config not stored")
    }
}

func TestBootNoProviders(t *testing.T) {
    plugin := &MyPlugin{}

    deps := shared.Deps{
        Config: &config.Config{
            Providers: []config.LLMConfig{},
        },
    }

    err := plugin.Boot(deps)
    if err == nil {
        t.Fatal("expected error for no providers")
    }
}
```

### Integration tests

Test the full plugin lifecycle:

```go
package main

import (
    "os"
    "os/exec"
    "testing"

    "github.com/hashicorp/go-plugin"
    "github.com/peggco/pegg/internal/plugin/shared"
)

func TestPluginServe(t *testing.T) {
    // Build the plugin
    cmd := exec.Command("go", "build", "-o", "test-plugin", ".")
    if err := cmd.Run(); err != nil {
        t.Fatalf("build failed: %v", err)
    }
    defer os.Remove("test-plugin")

    // Start the plugin process
    client := plugin.NewClient(&plugin.ClientConfig{
        HandshakeConfig: shared.Handshake,
        Plugins:         shared.PluginMap,
        Cmd:             exec.Command("./test-plugin"),
    })
    defer client.Kill()

    // Connect
    rpcClient, err := client.Client()
    if err != nil {
        t.Fatalf("connect failed: %v", err)
    }

    // Dispense
    raw, err := rpcClient.Dispense("pegg")
    if err != nil {
        t.Fatalf("dispense failed: %v", err)
    }

    // Type assert
    p, ok := raw.(shared.Plugin)
    if !ok {
        t.Fatal("does not implement Plugin")
    }

    // Verify
    if p.Name() != "my-plugin" {
        t.Errorf("name = %q, want %q", p.Name(), "my-plugin")
    }
}
```

## Debugging

### Enable debug logging

Set the plugin's log level to debug:

```go
logger := hclog.New(&hclog.LoggerOptions{
    Name:   "my-plugin",
    Output: os.Stderr,
    Level:  hclog.Debug,  // Enable debug output
})
```

### Run manually

Test your plugin standalone:

```bash
# Run directly
./my-plugin

# Check for errors
echo $?  # Exit code
```

### Check Pegg logs

```bash
pegg logs --search plugin
pegg logs --search my-plugin
```

### Common issues

| Issue | Solution |
|---|---|
| Plugin not appearing | Check file has execute permission in `~/.pegg/plugins/` |
| Boot fails | Check logs for error message |
| Plugin crashes | Check stderr output and logs |
| RPC errors | Ensure `shared.Handshake` is used correctly |
