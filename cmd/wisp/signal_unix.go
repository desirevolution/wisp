//go:build !windows

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func shutdownContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}
