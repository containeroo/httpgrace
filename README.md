# httpgrace

Small Go module for running HTTP servers with graceful shutdown and sensible defaults.

## Install

```bash
go get github.com/containeroo/httpgrace
```

## API

- `Run(ctx, listenAddr, router, logger, opts...)` builds an `http.Server` and runs it.
- `RunServer(ctx, server, logger)` runs an already configured `*http.Server`.
- Both return startup and shutdown errors to the caller.
- `logger` is optional; when `nil`, lifecycle logging is disabled.

## Defaults

- `ReadHeaderTimeout`: `10s`
- `WriteTimeout`: `15s`
- `IdleTimeout`: `60s`
- `ShutdownTimeout`: `10s`

## Examples

### Run with default timeouts

```go
package main

import (
    "context"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"

    "github.com/containeroo/httpgrace/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx, ":8080", mux, logger); err != nil {
		logger.Error("server stopped with error", "err", err)
	}
}
```

### RunServer with a custom http.Server

```go
package main

import (
  "context"
  "log/slog"
  "net/http"
  "os"
  "os/signal"
  "syscall"
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
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.RunServer(ctx, srv, logger); err != nil {
		logger.Error("server stopped with error", "err", err)
	}
}
```

## Inspiration

This module was inspired by the Grafana blog post:

```
https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/
```

## License

This project is licensed under the Apache 2.0 License. See the [LICENSE](LICENSE) file for details.
