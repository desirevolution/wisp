//go:build windows

package main

import (
	"context"
	"os"
	"os/signal"
)

func shutdownContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt)
}
