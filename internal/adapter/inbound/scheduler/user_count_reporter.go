package scheduler

import (
	"context"
	"log/slog"
	"time"
)

type UserCounter interface {
	Count(ctx context.Context) (int64, error)
}

type UserCountReporter struct {
	counter  UserCounter
	interval time.Duration
	logger   *slog.Logger
}

func NewUserCountReporter(counter UserCounter, interval time.Duration, logger *slog.Logger) *UserCountReporter {
	return &UserCountReporter{counter: counter, interval: interval, logger: logger}
}

func (r *UserCountReporter) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.logger.Info("user count reporter started", "interval", r.interval.String())
	for {
		select {
		case <-ctx.Done():
			r.logger.Info("user count reporter stopped")
			return
		case <-ticker.C:
			r.report(ctx)
		}
	}
}

func (r *UserCountReporter) report(ctx context.Context) {
	n, err := r.counter.Count(ctx)
	if err != nil {
		if ctx.Err() == nil {
			r.logger.Error("failed to count users", "error", err)
		}
		return
	}
	r.logger.Info("total users in database", "count", n)
}
