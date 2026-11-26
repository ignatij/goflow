package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestComputeBackoff(t *testing.T) {
	base := 50 * time.Millisecond
	maxDelay := 300 * time.Millisecond

	t.Run("exponential growth capped at max with upper jitter bound", func(t *testing.T) {
		full := func() float64 { return 1.0 }
		cases := []struct {
			attempt int
			want    time.Duration
		}{
			{attempt: 0, want: 50 * time.Millisecond},
			{attempt: 1, want: 100 * time.Millisecond},
			{attempt: 2, want: 200 * time.Millisecond},
			{attempt: 3, want: 300 * time.Millisecond},
			{attempt: 4, want: 300 * time.Millisecond},
			{attempt: 100, want: 300 * time.Millisecond}, // overflow-safe
		}
		for _, tc := range cases {
			got := computeBackoff(tc.attempt, base, maxDelay, full)
			if got != tc.want {
				t.Errorf("attempt %d: got %v, want %v", tc.attempt, got, tc.want)
			}
		}
	})

	t.Run("equal jitter lower bound", func(t *testing.T) {
		zero := func() float64 { return 0.0 }
		if got := computeBackoff(0, base, maxDelay, zero); got != 25*time.Millisecond {
			t.Errorf("attempt 0 lower bound: got %v, want 25ms", got)
		}
		if got := computeBackoff(2, base, maxDelay, zero); got != 100*time.Millisecond {
			t.Errorf("attempt 2 lower bound: got %v, want 100ms", got)
		}
	})

	t.Run("defaults applied when zero", func(t *testing.T) {
		full := func() float64 { return 1.0 }
		if got := computeBackoff(0, 0, 0, full); got != DefaultRetryBaseDelay {
			t.Errorf("got %v, want %v", got, DefaultRetryBaseDelay)
		}
		if got := computeBackoff(50, 0, 0, full); got != DefaultRetryMaxDelay {
			t.Errorf("got %v, want %v", got, DefaultRetryMaxDelay)
		}
	})
}

func TestWaitBackoff_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := waitBackoff(ctx, 5*time.Second)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("waitBackoff did not abort promptly on cancel: took %v", elapsed)
	}
}
