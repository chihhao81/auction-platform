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
	// Wait for three full ticks rather than a fixed sleep, so a busy
	// machine cannot make the test flaky.
	deadline := time.Now().Add(5 * time.Second)
	for ok.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after cancel")
	}
	// Jobs run in order (panics, fails, ok), so earlier jobs ran at least as
	// often as the last one: failures never stop the following jobs or ticks.
	if ok.Load() < 3 || failing.Load() < ok.Load() || panicking.Load() < ok.Load() {
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
