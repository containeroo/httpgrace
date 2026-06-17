//go:build unix

package server

import (
	"os"
	"syscall"
)

func defaultShutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
