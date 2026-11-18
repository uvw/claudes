package cli

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSleepEndsEarlyWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if err := sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("err %v", err)
	}
}
