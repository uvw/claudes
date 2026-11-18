package cli

import (
	"context"
	"os/signal"
	"time"
)

// signalContext is ctx, canceled by the first of stopSignals, and the function
// that restores the default, so a second signal ends claudes.
func signalContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, stopSignals...)
}

// sleep waits for d, or returns ctx's error when ctx ends first.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
