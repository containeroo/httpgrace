package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
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

// defaultOptions returns the default timeout configuration used by Run.
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

// runServer starts server and blocks until either startup fails or ctx is canceled.
// After cancellation, it performs graceful shutdown bounded by shutdownTimeout.
func runServer(
	ctx context.Context,
	server *http.Server,
	logger *slog.Logger,
	shutdownTimeout time.Duration,
) error {
	var eg errgroup.Group
	// Closed when ListenAndServe exits, regardless of success or failure.
	serveDone := make(chan struct{})

	// Serve loop: returns startup/runtime errors, but treats ErrServerClosed as normal.
	eg.Go(func() error {
		defer close(serveDone)
		logger.Info("starting server", "listenAddr", server.Addr)
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	// Shutdown loop: waits for caller cancellation, unless serving already finished.
	eg.Go(func() error {
		select {
		case <-serveDone: // Serve loop ended before cancellation; nothing to shut down.
			return nil
		case <-ctx.Done(): // Requested graceful shutdown.
		}

		logger.Info("shutting down server")

		// Use a bounded timeout to finish in-flight requests.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			// Force close to unblock ListenAndServe if graceful shutdown times out.
			_ = server.Close()
			return err
		}
		return nil
	})

	return eg.Wait()
}

// newServer builds an http.Server from listenAddr, router, and resolved options.
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

// optionsFrom resolves options by applying opts on top of defaultOptions.
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
