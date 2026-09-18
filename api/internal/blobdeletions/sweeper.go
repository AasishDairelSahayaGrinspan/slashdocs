// Package blobdeletions runs the background half of blob deletion: the
// objects whose keys a document delete queued but could not remove straight
// away, because blob storage was unavailable.
package blobdeletions

import (
	"context"
	"log/slog"
	"time"
)

// Store claims queued blob deletions, deletes the objects, and clears the rows
// it finished, returning how many it cleared.
type Store interface {
	SweepBlobDeletions(ctx context.Context) (int, error)
}

// defaultInterval is how often a sweep runs when Interval is unset. Queued
// keys are the exception, not the rule, so the sweeper polls gently.
const defaultInterval = time.Minute

type Sweeper struct {
	Store Store
	// Interval overrides defaultInterval; tests use it to sweep promptly.
	Interval       time.Duration
	nextFailureLog time.Time
}

func (s *Sweeper) Run(ctx context.Context) {
	interval := s.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		s.Sweep(ctx)
	}
}

// Sweep clears the due backlog batch by batch, so a queue left behind by a
// blob storage outage does not trickle out one batch per tick. Whatever it
// cannot delete keeps its outbox row and comes back on a later tick.
func (s *Sweeper) Sweep(ctx context.Context) {
	for ctx.Err() == nil {
		cleared, err := s.Store.SweepBlobDeletions(ctx)
		if err != nil {
			s.logSweepFailure(err)
			return
		}
		if cleared == 0 {
			return
		}
	}
}

// logSweepFailure keeps a persistently unreachable blob store to one log line
// a minute, as the notification worker does for delivery failures.
func (s *Sweeper) logSweepFailure(err error) {
	now := time.Now()
	if now.Before(s.nextFailureLog) {
		return
	}
	s.nextFailureLog = now.Add(time.Minute)
	slog.Error("blob deletion sweep failed", slog.Any("error", err))
}
