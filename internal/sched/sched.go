package sched

import (
	"context"
	"log/slog"
	"math/rand"
	"time"
)


func Loop(ctx context.Context, name string, interval time.Duration, fn func(ctx context.Context)) {
	jitter := time.Duration(rand.Int63n(int64(interval / 10)))
	t := time.NewTimer(interval + jitter)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}

		start := time.Now()
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					slog.Error("scheduled job panic", "job", name, "err", rec)
				}
			}()
			fn(ctx)
		}()

		next := interval - time.Since(start)
		if next < time.Second {
			next = time.Second
		}
		next += time.Duration(rand.Int63n(int64(next/10 + 1)))

		t.Reset(next)
	}
}

