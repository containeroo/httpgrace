package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Options configures the HTTP server timeouts applied by Run.
type Options struct {
	// ReadHeaderTimeout limits how long to read request headers.
	ReadHeaderTimeout time.Duration
	// WriteTimeout limits the time spent writing the response.
	WriteTimeout time.Duration
	// IdleTimeout sets how long to keep idle connections open.
	IdleTimeout time.Duration
	// ShutdownTimeout bounds how long to wait for graceful shutdown.
	ShutdownTimeout time.Duration
}

// Option mutates Options used by Run.
type Option func(*Options)

// WithOptions overwrites the server timeout options used by Run.
func WithOptions(opts Options) Option {
	return func(target *Options) {
		*target = opts
	}
}

// defaultOptions returns sensible defaults for server timeouts.
func defaultOptions() Options {
	return Options{
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ShutdownTimeout:   10 * time.Second,
	}
}

// Run builds an http.Server with sensible defaults and runs it until ctx is canceled.
// Use options to override the default timeout values.
func Run(ctx context.Context, listenAddr string, router http.Handler, logger *slog.Logger, opts ...Option) error {
	options := optionsFrom(opts...)
	server := newServer(listenAddr, router, options)
	return runServer(ctx, server, logger, options.ShutdownTimeout)
}

// RunServer manages an existing http.Server and shuts it down when ctx is canceled.
// The server is started with ListenAndServe in a background goroutine.
func RunServer(ctx context.Context, server *http.Server, logger *slog.Logger) error {
	return runServer(ctx, server, logger, defaultOptions().ShutdownTimeout)
}

func runServer(ctx context.Context, server *http.Server, logger *slog.Logger, shutdownTimeout time.Duration) error {
	// Start the server in the background.
	go func() {
		logger.Info("starting server", "listenAddr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
		}
	}()

	// Graceful shutdown once the context is canceled.
	var wg sync.WaitGroup
	wg.Go(func() {
		<-ctx.Done() // wait for cancel/timeout

		logger.Info("shutting down server")

		// Use a bounded timeout to finish in-flight requests.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown error", "err", err)
		}
	})

	// Block until the shutdown goroutine finishes.
	wg.Wait()
	return nil
}

// newServer returns a new http.Server with sensible defaults and the provided options
func newServer(listenAddr string, router http.Handler, options Options) *http.Server {
	// Create server with sensible timeouts.
	return &http.Server{
		Addr:              listenAddr,
		Handler:           router,
		ReadHeaderTimeout: options.ReadHeaderTimeout,
		WriteTimeout:      options.WriteTimeout,
		IdleTimeout:       options.IdleTimeout,
	}
}

// optionsFrom returns sensible defaults for server timeouts.
func optionsFrom(opts ...Option) Options {
	options := defaultOptions()
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&options)
	}
	return options
}
