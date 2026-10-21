// Package scheduler runs periodic jobs inside the server process. It is the
// project's only background loop (see AGENTS.md: one tick per interval, no
// queue, no extra process). Jobs must be idempotent.
package scheduler

import (
	"context"
	"log/slog"
	"time"
)

type Job struct {
	Name string
	Run  func(ctx context.Context) error
}

// Run executes every job immediately and then once per interval until ctx is
// cancelled. A tick never overlaps the previous one; each tick is bounded by
// the interval as a timeout, and panics are recovered so one bad run cannot
// stop the loop.
func Run(ctx context.Context, interval time.Duration, jobs ...Job) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		tick(ctx, interval, jobs)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func tick(ctx context.Context, timeout time.Duration, jobs []Job) {
	for _, j := range jobs {
		if ctx.Err() != nil {
			return
		}
		runJob(ctx, timeout, j)
	}
}

func runJob(ctx context.Context, timeout time.Duration, j Job) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			slog.Error("scheduler job panicked", "job", j.Name, "panic", r)
		}
	}()
	start := time.Now()
	if err := j.Run(ctx); err != nil {
		slog.Error("scheduler job failed", "job", j.Name, "err", err, "duration", time.Since(start))
	}
}
