# AGENTS.md - Developer Guidelines for AI/Agentic Coding

This document provides essential information for agents operating in the Leaf game server framework repository.

## Build, Lint, and Test Commands

### Testing
```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run a single test package
go test -v ./chanrpc

# Run a specific test (Example tests are the main test format)
go test -v ./chanrpc -run Example

# Run tests in a specific package with output
go test -v ./timer -run ExampleTimer

# Run all Example-based tests across the entire project
go test -v ./... -run Example
```

### Building
```bash
# Build the leaf package
go build ./...

# Build and check for issues
go build -v ./...
```

### Formatting and Linting
```bash
# Format code (standard Go format)
go fmt ./...

# Check code with go vet
go vet ./...

# Run both formatting and vet
go fmt ./... && go vet ./...
```

## Testing Approach

- **Primary Test Format**: Example tests (functions named `Example*` in `*_test.go` files)
- **Test Packages**: Tests use `packagename_test` to test the public API
- **Output Validation**: Example tests validate output via comments at the end:
  ```go
  // Output:
  // expected line 1
  // expected line 2
  ```

## Code Style Guidelines

### Import Organization
```go
// Standard library imports first, followed by blank line, then third-party
import (
	"fmt"
	"sync"
	
	"github.com/gorilla/websocket"
	"github.com/name5566/leaf/module"
)
```
- Group imports in this order: standard library, blank line, third-party packages
- Use absolute import paths (e.g., `github.com/name5566/leaf/...`)
- Imports within the same project use full paths, not relative paths

### Naming Conventions
- **Exported types/functions**: PascalCase (e.g., `WSConn`, `Register`, `LocalAddr`)
- **Unexported types/functions**: camelCase (e.g., `newWSConn`, `doDestroy`)
- **Constants**: Standard Go convention (typically UPPER_CASE for package-level)
- **Interface types**: Describe what they do, often ending with `-er` (e.g., `Reader`, `Writer`)
- **Method receivers**: Keep short, usually 1-2 character abbreviations (e.g., `wsConn`, `s`)

### Type Definitions
- Use `type` keyword for named types and interfaces
- Structure layouts: embed fields logically (mutex first for thread-safe types)
- Pointer receivers for methods that modify state; value receivers otherwise
- Example of thread-safe type structure:
  ```go
  type WSConn struct {
      sync.Mutex              // embedded mutex for locks
      conn    *websocket.Conn // connection field
      closeFlag int32         // atomic flag for thread-safe checks
  }
  ```

### Error Handling
- Return errors as the last return value
- Use `errors.New()` for simple error messages
- Check errors with `if err != nil` pattern immediately after operations
- Provide context in error messages (e.g., "message too long", "connection closed")
- Avoid panic unless absolutely necessary (initialization time only)

### Concurrency Patterns
- Use `sync.Mutex` for critical sections; embed in struct if protecting entire type
- Use `sync/atomic` for simple flag checks and updates (e.g., `atomic.LoadInt32`)
- Lock-free checks can be done with atomic operations before acquiring lock
- IMPORTANT: Avoid TOCTOU (Time-of-Check-Time-of-Use) race conditions
  - Always re-check protected state after acquiring lock
  - Example: Check `closeFlag` again inside the lock before using the connection

### Function Design
- Keep functions focused and single-purpose
- Use variadic parameters for flexible input (`args ...[]byte`)
- Defer cleanup operations when acquiring locks: `defer wsConn.Unlock()`
- Compute expensive operations outside locks when safe

### Comments
- Explain the "why", not just the "what"
- Document exported functions/types (start with function/type name)
- Use comments for algorithm explanations or non-obvious code
- Example: `// goroutine not safe` documents usage restrictions

### Module Structure
```
Each package should have:
- Clear public API (exported types/functions)
- Example tests demonstrating usage (in *_test.go files)
- Internal helpers (unexported functions)
- Proper error handling with context-specific messages
```

## Key Framework Patterns

### Module System
- Modules implement the `module.Module` interface
- Lifecycle: `Register()` → `Init()` → `Destroy()`
- Pass modules to `leaf.Run(mods ...Module)`

### Chan-RPC System
- Synchronous calls: `Call0()`, `Call1()`, `CallN()`
- Asynchronous calls: `AsynCall()` with callback pattern
- Server registers handlers and executes with `s.Exec(<-s.ChanCall)`

### Network Layer
- WebSocket support via gorilla/websocket
- Thread-safe message writing with `WriteMsg()`
- Read/write separation in usage patterns

## Dependencies
- Go 1.17 or later
- `github.com/gorilla/websocket` for WebSocket connections
- `github.com/golang/protobuf` for Protocol Buffer serialization
- `gopkg.in/mgo.v2` for MongoDB support

## Project Structure
```
leaf/
├── chanrpc/        # Channel-based RPC system
├── cluster/        # Clustering support
├── conf/          # Configuration management
├── console/       # Console interface
├── db/            # Database layer (MongoDB)
├── gate/          # Server gateway
├── go/            # Concurrent goroutine management
├── log/           # Logging utilities
├── module/        # Module system
├── network/       # Network layer (WebSocket, JSON, Protobuf)
├── recordfile/    # Record file handling
├── timer/         # Timer and cron scheduling
└── util/          # Utility functions
```

## Additional Notes
- No Cursor rules or Copilot instructions found; this document is the primary guideline
- The project follows standard Go conventions and best practices
- Thread-safety is critical, especially in network and RPC layers
- Example tests serve as both documentation and verification
