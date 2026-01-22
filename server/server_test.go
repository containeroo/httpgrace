package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServerDefaults(t *testing.T) {
	options := optionsFrom()
	srv := newServer(":8080", http.NewServeMux(), options)
	assert.Equal(t, 10*time.Second, srv.ReadHeaderTimeout)
	assert.Equal(t, 15*time.Second, srv.WriteTimeout)
	assert.Equal(t, 60*time.Second, srv.IdleTimeout)
	assert.Equal(t, 10*time.Second, options.ShutdownTimeout)
}

func TestOptionsOverrides(t *testing.T) {
	overrides := Options{
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       4 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
	options := optionsFrom(WithOptions(overrides))
	srv := newServer(":8080", http.NewServeMux(), options)
	assert.Equal(t, options.ReadHeaderTimeout, srv.ReadHeaderTimeout)
	assert.Equal(t, options.WriteTimeout, srv.WriteTimeout)
	assert.Equal(t, options.IdleTimeout, srv.IdleTimeout)
	assert.Equal(t, options.ShutdownTimeout, options.ShutdownTimeout)
}

func TestOptionsIgnoreNil(t *testing.T) {
	options := optionsFrom(nil, WithOptions(Options{ShutdownTimeout: 2 * time.Second}), nil)
	assert.Equal(t, 2*time.Second, options.ShutdownTimeout)
}

func TestRunServerStopsOnCancel(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := http.NewServeMux()
	srv := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: handler,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- RunServer(ctx, srv, logger)
	}()

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err, "RunServer returned error: %v", err)
	case <-time.After(2 * time.Second):
		require.Fail(t, "RunServer did not return after context cancel")
	}
}

func TestRunServerListenErrorDoesNotBlockShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := http.NewServeMux()
	srv := &http.Server{
		Addr:    "127.0.0.1:1",
		Handler: handler,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runServer(ctx, srv, logger, 10*time.Millisecond)
	}()

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err, "runServer returned error: %v", err)
	case <-time.After(2 * time.Second):
		require.Fail(t, "runServer did not return after context cancel")
	}
}
