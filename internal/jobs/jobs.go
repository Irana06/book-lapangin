// Package jobs berisi pekerjaan rutin yang jalan di background selama server hidup.
package jobs

import (
	"context"
	"log/slog"
	"time"

	"lapangin/internal/store"
)

// Run menjalankan maintenance setiap interval sampai ctx dibatalkan.
func Run(ctx context.Context, logger *slog.Logger, st *store.Store, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		tick(ctx, logger, st)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func tick(ctx context.Context, logger *slog.Logger, st *store.Store) {
	tasks := []struct {
		name string
		fn   func(context.Context) (int64, error)
	}{
		{"expire unpaid bookings", st.ExpireHolds},
		{"complete finished bookings", st.CompleteFinished},
		{"delete expired sessions", st.DeleteExpiredSessions},
	}
	for _, t := range tasks {
		n, err := t.fn(ctx)
		if err != nil {
			if ctx.Err() == nil {
				logger.Error("job failed", "job", t.name, "err", err)
			}
			continue
		}
		if n > 0 {
			logger.Info("job done", "job", t.name, "rows", n)
		}
	}
}
