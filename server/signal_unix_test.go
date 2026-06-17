//go:build unix

package server

import (
	"context"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignalContextCancelsWithSignalCause(t *testing.T) {
	ctx, stop := SignalContext(context.Background(), syscall.SIGUSR1)
	defer stop()

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGUSR1))

	select {
	case <-ctx.Done():
		cause := context.Cause(ctx)
		require.Error(t, cause)
		assert.True(t, strings.HasPrefix(cause.Error(), "received signal: "))
		assert.False(t, errors.Is(cause, context.Canceled))
	case <-time.After(2 * time.Second):
		require.Fail(t, "SignalContext did not cancel after signal")
	}
}
