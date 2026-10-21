package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunExecutesImmediatelyAndPeriodicallySurvivingFailures(t *testing.T) {
	var ok, failing, panicking atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, 20*time.Millisecond,
			Job{"panics", func(context.Context) error { panicking.Add(1); panic("boom") }},
			Job{"fails", func(context.Context) error { failing.Add(1); return errors.New("x") }},
			Job{"ok", func(context.Context) error { ok.Add(1); return nil }},
		)
		close(done)
	}()
	time.Sleep(110 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after cancel")
	}
	if ok.Load() < 3 || failing.Load() != ok.Load() || panicking.Load() != ok.Load() {
		t.Fatalf("runs: ok=%d failing=%d panicking=%d", ok.Load(), failing.Load(), panicking.Load())
	}
}

func TestJobTimeoutIsBoundedByInterval(t *testing.T) {
	var deadline time.Time
	runJob(context.Background(), 50*time.Millisecond, Job{"d", func(ctx context.Context) error {
		deadline, _ = ctx.Deadline()
		return nil
	}})
	if time.Until(deadline) > 50*time.Millisecond || deadline.IsZero() {
		t.Fatalf("deadline %v not bounded", deadline)
	}
}
