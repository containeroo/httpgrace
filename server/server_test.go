package server

import (
	"context"
	"io"
	"log/slog"
	"net"
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

func TestRunServerStopsOnCancelWithNilLogger(t *testing.T) {
	handler := http.NewServeMux()
	srv := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: handler,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- RunServer(ctx, srv, nil)
	}()

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err, "RunServer returned error: %v", err)
	case <-time.After(2 * time.Second):
		require.Fail(t, "RunServer did not return after context cancel with nil logger")
	}
}

func TestRunServerListenErrorDoesNotBlockShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := http.NewServeMux()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	srv := &http.Server{
		Addr:    ln.Addr().String(),
		Handler: handler,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runServer(ctx, srv, logger, 10*time.Millisecond)
	}()

	select {
	case err := <-done:
		require.Error(t, err, "runServer should return startup listen error")
	case <-time.After(2 * time.Second):
		require.Fail(t, "runServer did not return after startup listen failure")
	}
}

func TestRunReturnsListenError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := http.NewServeMux()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = Run(ctx, ln.Addr().String(), handler, logger)
	require.Error(t, err, "Run should return startup listen error")
}

func TestRunServerReturnsShutdownTimeoutError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-release
		_, _ = w.Write([]byte("ok"))
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runServer(ctx, srv, logger, 20*time.Millisecond)
	}()

	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		for i := 0; i < 100; i++ {
			resp, reqErr := http.Get("http://" + addr)
			if reqErr == nil {
				_ = resp.Body.Close()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not start")
	}

	cancel()

	select {
	case err := <-done:
		require.Error(t, err, "runServer should return shutdown timeout error")
	case <-time.After(2 * time.Second):
		t.Fatal("runServer did not return after shutdown timeout")
	}

	close(release)
	<-clientDone
}
