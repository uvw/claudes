// Package interrupt holds the signals that end a claudes run, and the waits
// that end with them.
package interrupt

import (
	"context"
	"os/signal"
	"time"
)

// Context is ctx, canceled by the first of stopSignals, and the function that
// restores the default, so a second signal ends claudes.
func Context(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, stopSignals...)
}

// Sleep waits for d, or until ctx ends, and then returns ctx's error.
func Sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
