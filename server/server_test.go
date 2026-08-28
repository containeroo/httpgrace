package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireNoGoroutineLeaks fails the test when the Go runtime detects
// goroutines that can no longer possibly become unblocked.
func requireNoGoroutineLeaks(t *testing.T) {
	t.Helper()

	profile := pprof.Lookup("goroutineleak")
	require.NotNil(t, profile)

	var output strings.Builder
	require.NoError(t, profile.WriteTo(&output, 1))

	assert.Zero(
		t,
		profile.Count(),
		"leaked goroutines:\n%s",
		output.String(),
	)
}

// checkGoroutineLeaks registers a leak check that runs after the test and all
// other cleanup functions registered later by the test have completed.
func checkGoroutineLeaks(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		requireNoGoroutineLeaks(t)
	})
}

func TestNewServerDefaults(t *testing.T) {
	options := optionsFrom()
	srv := newServer(":8080", http.NewServeMux(), options)

	assert.Equal(t, 10*time.Second, srv.ReadHeaderTimeout)
	assert.Equal(t, 15*time.Second, srv.WriteTimeout)
	assert.Equal(t, 60*time.Second, srv.IdleTimeout)
	assert.Equal(t, http.DefaultMaxHeaderValueCount, srv.MaxHeaderValueCount)
	assert.Equal(t, 10*time.Second, options.ShutdownTimeout)
}

func TestOptionsOverrides(t *testing.T) {
	overrides := Options{
		ReadHeaderTimeout:   2 * time.Second,
		WriteTimeout:        3 * time.Second,
		IdleTimeout:         4 * time.Second,
		MaxHeaderValueCount: 100,
		ShutdownTimeout:     5 * time.Second,
	}

	options := optionsFrom(WithOptions(overrides))
	srv := newServer(":8080", http.NewServeMux(), options)

	assert.Equal(t, 2*time.Second, srv.ReadHeaderTimeout)
	assert.Equal(t, 3*time.Second, srv.WriteTimeout)
	assert.Equal(t, 4*time.Second, srv.IdleTimeout)
	assert.Equal(t, 100, srv.MaxHeaderValueCount)
	assert.Equal(t, 5*time.Second, options.ShutdownTimeout)
}

func TestIndividualOptionsOverrideDefaults(t *testing.T) {
	options := optionsFrom(
		WithReadHeaderTimeout(2*time.Second),
		WithWriteTimeout(3*time.Second),
		WithIdleTimeout(4*time.Second),
		WithMaxHeaderValueCount(100),
		WithShutdownTimeout(5*time.Second),
	)

	assert.Equal(t, 2*time.Second, options.ReadHeaderTimeout)
	assert.Equal(t, 3*time.Second, options.WriteTimeout)
	assert.Equal(t, 4*time.Second, options.IdleTimeout)
	assert.Equal(t, 100, options.MaxHeaderValueCount)
	assert.Equal(t, 5*time.Second, options.ShutdownTimeout)
}

func TestMaxHeaderValueCountPreservesDefaults(t *testing.T) {
	options := optionsFrom(WithMaxHeaderValueCount(100))

	assert.Equal(t, 10*time.Second, options.ReadHeaderTimeout)
	assert.Equal(t, 15*time.Second, options.WriteTimeout)
	assert.Equal(t, 60*time.Second, options.IdleTimeout)
	assert.Equal(t, 100, options.MaxHeaderValueCount)
	assert.Equal(t, 10*time.Second, options.ShutdownTimeout)
}

func TestOptionsIgnoreNil(t *testing.T) {
	options := optionsFrom(
		nil,
		WithShutdownTimeout(2*time.Second),
		nil,
	)

	assert.Equal(t, 2*time.Second, options.ShutdownTimeout)
}

func TestRunServerStopsOnCancel(t *testing.T) {
	checkGoroutineLeaks(t)

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
	checkGoroutineLeaks(t)

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
		require.NoError(
			t,
			err,
			"RunServer returned error: %v",
			err,
		)
	case <-time.After(2 * time.Second):
		require.Fail(
			t,
			"RunServer did not return after context cancel with nil logger",
		)
	}
}

func TestRunServerLogsCancelCause(t *testing.T) {
	checkGoroutineLeaks(t)

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	handler := http.NewServeMux()
	srv := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: handler,
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- RunServer(ctx, srv, logger)
	}()

	cancel(errors.New("received signal: terminated"))

	select {
	case err := <-done:
		require.NoError(t, err, "RunServer returned error: %v", err)
	case <-time.After(2 * time.Second):
		require.Fail(t, "RunServer did not return after context cancel")
	}

	assert.Contains(t, logs.String(), "shutting down server")
	assert.Contains(t, logs.String(), `cause="received signal: terminated"`)
}

func TestSignalContextStopCancels(t *testing.T) {
	checkGoroutineLeaks(t)

	ctx, stop := SignalContext(context.Background())
	stop()

	select {
	case <-ctx.Done():
		assert.ErrorIs(t, context.Cause(ctx), context.Canceled)
	case <-time.After(2 * time.Second):
		require.Fail(t, "SignalContext stop did not cancel context")
	}
}

func TestRunServerListenErrorDoesNotBlockShutdown(t *testing.T) {
	checkGoroutineLeaks(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := http.NewServeMux()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ln.Close()
	})

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
		require.Fail(
			t,
			"runServer did not return after startup listen failure",
		)
	}
}

func TestRunReturnsListenError(t *testing.T) {
	checkGoroutineLeaks(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := http.NewServeMux()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ln.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = Run(ctx, ln.Addr().String(), handler, logger)
	require.Error(t, err, "Run should return startup listen error")
}

func TestRunServerReturnsShutdownTimeoutError(t *testing.T) {
	checkGoroutineLeaks(t)

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

		for range 100 {
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
		require.Error(
			t,
			err,
			"runServer should return shutdown timeout error",
		)
	case <-time.After(2 * time.Second):
		t.Fatal("runServer did not return after shutdown timeout")
	}

	close(release)
	<-clientDone
}
