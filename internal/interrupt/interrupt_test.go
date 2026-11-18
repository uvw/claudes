package interrupt

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSleepEndsEarlyWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if err := Sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("err %v", err)
	}
}

func TestStopCancelsTheContext(t *testing.T) {
	ctx, stop := Context(context.Background())
	stop()
	if ctx.Err() == nil {
		t.Fatal("stop left the context running")
	}
}
