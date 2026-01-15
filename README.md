# OpenCode Go Library

A minimal Go library for managing OpenCode server instances. Provides a simple API for starting, stopping, and monitoring isolated OpenCode servers with custom configurations.

## Example

See [`cmd/example/main.go`](cmd/example/main.go) for a complete working example demonstrating server lifecycle management.

## Available Methods

- **`New(cfg Config)`** - Create a new OpenCode instance
- **`Start()`** - Start an isolated OpenCode server instance
- **`Stop()`** - Stop the OpenCode server
- **`Addr()`** - Get the server address (host:port)
- **`WaitForReady(maxAttempts int)`** - Wait for the server to become ready

## Configuration

```go
type Config struct {
    ConfigDir string  // Directory for OpenCode config and data
    Addr      string  // Server address (auto-allocated if empty)
    APIKey    string  // API key for authentication
}
```
