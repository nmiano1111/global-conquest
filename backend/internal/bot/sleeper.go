package bot

import (
	"context"
	"time"
)

// Sleeper abstracts the wall-clock delay applied between committed bot
// actions in live mode, so tests never depend on real time.
type Sleeper interface {
	Sleep(ctx context.Context, d time.Duration) error
}

// RealSleeper sleeps for the given duration or returns early if ctx is
// canceled.
type RealSleeper struct{}

// Sleep blocks for duration d, or returns ctx.Err() early if ctx is
// canceled first. A non-positive d returns immediately with a nil error.
func (RealSleeper) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
