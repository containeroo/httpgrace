package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
)

// SignalContext returns a copy of parent that is canceled when one of the
// configured signals is received. The cancellation cause records the signal
// name, allowing Run and RunServer to include it in shutdown logs.
//
// If no signals are provided, SignalContext listens for the platform defaults.
// On Unix platforms those are os.Interrupt, syscall.SIGTERM, and syscall.SIGHUP.
//
// The returned stop function unregisters signal delivery and cancels the
// context. Callers should defer stop after creating the context.
func SignalContext(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
	if len(signals) == 0 {
		signals = defaultShutdownSignals()
	}

	ctx, cancel := context.WithCancelCause(parent)
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, signals...)

	var once sync.Once
	cancelWithCause := func(cause error) {
		once.Do(func() {
			signal.Stop(signalCh)
			cancel(cause)
		})
	}

	go func() {
		select {
		case sig := <-signalCh:
			cancelWithCause(signalCause(sig))
		case <-ctx.Done():
			cancelWithCause(context.Cause(ctx))
		}
	}()

	return ctx, func() {
		cancelWithCause(nil)
	}
}

func signalCause(sig os.Signal) error {
	return fmt.Errorf("received signal: %s", sig)
}
