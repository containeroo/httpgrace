//go:build !unix

package server

import "os"

func defaultShutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
