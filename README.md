# httpgrace

Small Go module for running HTTP servers with graceful shutdown and sensible defaults.

Requires Go 1.27 or newer.

## Install

```bash
go get github.com/containeroo/httpgrace
```

## API

- `Run(ctx, listenAddr, handler, logger, opts...)` builds an `http.Server` and runs it.
- `RunServer(ctx, server, logger)` runs an already configured `*http.Server`.
- `SignalContext(parent, signals...)` creates a signal-aware context with cancellation causes.
- `Run` and `RunServer` return startup and shutdown errors to the caller.
- `logger` is optional; when `nil`, lifecycle logging is disabled.

`Run` accepts focused functional options for overriding individual defaults:

- `WithReadHeaderTimeout`
- `WithWriteTimeout`
- `WithIdleTimeout`
- `WithMaxHeaderValueCount`
- `WithShutdownTimeout`

`WithOptions` remains available when the complete option set should be replaced.

## Defaults

- `ReadHeaderTimeout`: `10s`
- `WriteTimeout`: `15s`
- `IdleTimeout`: `60s`
- `MaxHeaderValueCount`: `http.DefaultMaxHeaderValueCount`
- `ShutdownTimeout`: `10s`

Go 1.27 defines `http.DefaultMaxHeaderValueCount` as `500`.

## Signals

`SignalContext` creates a context that is canceled when a configured signal is received.
The received signal is stored as the cancellation cause, and shutdown logs include
`context.Cause(ctx)`.

By default, `SignalContext` listens for:

- `os.Interrupt`
- `SIGTERM` on Unix platforms

To treat additional signals, such as `SIGHUP`, as shutdown signals, pass them explicitly:

```go
ctx, stop := server.SignalContext(
	context.Background(),
	os.Interrupt,
	syscall.SIGTERM,
	syscall.SIGHUP,
)
defer stop()
```

## Examples

### Run with default settings

```go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/containeroo/httpgrace/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	ctx, stop := server.SignalContext(context.Background())
	defer stop()

	if err := server.Run(ctx, ":8080", mux, logger); err != nil {
		logger.Error("server stopped with error", "err", err)
	}
}
```

### Override individual settings

Use the focused functional options when only individual defaults should change:

```go
if err := server.Run(
	ctx,
	":8080",
	mux,
	logger,
	server.WithMaxHeaderValueCount(100),
	server.WithShutdownTimeout(15*time.Second),
); err != nil {
	logger.Error("server stopped with error", "err", err)
}
```

Settings that are not explicitly overridden retain their `httpgrace` defaults.

### Replace all options

Use `WithOptions` when the complete configuration should be supplied by the caller:

```go
if err := server.Run(
	ctx,
	":8080",
	mux,
	logger,
	server.WithOptions(server.Options{
		ReadHeaderTimeout:   5 * time.Second,
		WriteTimeout:        10 * time.Second,
		IdleTimeout:         30 * time.Second,
		MaxHeaderValueCount: 100,
		ShutdownTimeout:     15 * time.Second,
	}),
); err != nil {
	logger.Error("server stopped with error", "err", err)
}
```

### RunServer with a custom http.Server

Use `RunServer` when the caller wants full control over `http.Server`:

```go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/containeroo/httpgrace/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:                ":8080",
		Handler:             mux,
		ReadHeaderTimeout:   5 * time.Second,
		WriteTimeout:        10 * time.Second,
		IdleTimeout:         30 * time.Second,
		MaxHeaderValueCount: 100,
	}

	ctx, stop := server.SignalContext(context.Background())
	defer stop()

	if err := server.RunServer(ctx, srv, logger); err != nil {
		logger.Error("server stopped with error", "err", err)
	}
}
```

## Inspiration

This module was inspired by the Grafana blog post:

```text
https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/
```

## License

This project is licensed under the Apache 2.0 License. See the [LICENSE](LICENSE) file for details.
