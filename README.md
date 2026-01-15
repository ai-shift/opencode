# OpenCode Go Library

A Go library for programmatically controlling OpenCode. Provides a type-safe client for starting OpenCode instances, managing sessions, sending messages, and receiving streaming responses with strongly-typed events based on the OpenAPI specification.

Enables building AI-powered applications that leverage Claude's capabilities through isolated OpenCode sessions with full control over configuration and session persistence.

## Example

See [`cmd/example/main.go`](cmd/example/main.go) for a complete working example demonstrating session management, message streaming, and typed event handling.

## Available Methods

- **`Start()`** - Start an isolated OpenCode server instance
- **`Stop()`** - Stop the OpenCode server
- **`Addr()`** - Get the server address
- **`WaitForReady(maxAttempts int)`** - Wait for the server to become ready
- **`ListSessions()`** - List all sessions in the current directory
- **`CreateSession(title string)`** - Create a new session
- **`SendMessage(sessionID, text string)`** - Send a message to a session
- **`StreamEvents(callback func(Event))`** - Stream events with real-time updates

## Event Types

Strongly-typed events based on OpenCode's OpenAPI specification:
- **`ServerConnectedEvent`** - Server connection established
- **`MessageUpdatedEvent`** - Message state changes (with token usage, cost, finish status)
- **`MessagePartUpdatedEvent`** - Streaming text and content updates
- **`SessionUpdatedEvent`** - Session metadata changes
- **`SessionStatusEvent`** - Session status (idle/busy)
- **`UnknownEvent`** - Fallback for unhandled event types
