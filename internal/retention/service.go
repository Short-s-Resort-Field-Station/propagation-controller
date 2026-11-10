// Package retention implements spec §14: raw sensor readings roll up into
// hourly aggregates (kept forever); raw rows older than the retention window
// are pruned. Aggregation runs continuously so charts always have fresh
// hours; pruning runs daily and aggregates everything it is about to delete
// first — data is summarized before it is ever discarded.
package retention

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rs/zerolog"

	"propagation-controller/internal/logging"
	"propagation-controller/internal/storage"
)

type Config struct {
	RawDays int `json:"raw_days"`
}

func DefaultConfig() Config { return Config{RawDays: 30} }

type Service struct {
	aggregates *storage.AggregateRepo
	readings   *storage.ReadingRepo
	settings   *storage.SettingsRepo
	events     *logging.EventLogger
	log        zerolog.Logger

	aggregateEvery time.Duration
	pruneEvery     time.Duration
}

func NewService(aggregates *storage.AggregateRepo, readings *storage.ReadingRepo,
	settings *storage.SettingsRepo, events *logging.EventLogger, log zerolog.Logger) *Service {
	return &Service{
		aggregates:     aggregates,
		readings:       readings,
		settings:       settings,
		events:         events,
		log:            log,
		aggregateEvery: 5 * time.Minute,
		pruneEvery:     24 * time.Hour,
	}
}

func (s *Service) Start(ctx context.Context) {
	go func() {
		s.aggregate() // charts have hourly data immediately after boot
		s.prune()
		aggTicker := time.NewTicker(s.aggregateEvery)
		pruneTicker := time.NewTicker(s.pruneEvery)
		defer aggTicker.Stop()
		defer pruneTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-aggTicker.C:
				s.aggregate()
			case <-pruneTicker.C:
				s.prune()
			}
		}
	}()
}

func (s *Service) config() Config {
	cfg := DefaultConfig()
	raw, ok, err := s.settings.Get("retention")
	if err != nil || !ok {
		return cfg
	}
	var v Config
	if json.Unmarshal([]byte(raw), &v) == nil && v.RawDays >= 1 {
		cfg = v
	}
	return cfg
}

// aggregate refreshes the last 48 hours of buckets — wide enough to absorb
// clock skew and late writes, cheap enough to run continuously.
func (s *Service) aggregate() {
	now := time.Now().UTC()
	if _, err := s.aggregates.AggregateRange(now.Add(-48*time.Hour), now.Add(time.Hour)); err != nil {
		s.log.Error().Err(err).Msg("retention: aggregation failed")
	}
}

// prune summarizes then deletes raw readings older than the window.
func (s *Service) prune() {
	cfg := s.config()
	// Hour-aligned so a prune never splits a bucket: the partial hour would
	// be re-aggregated from the surviving rows and overwrite the full one.
	cutoff := time.Now().UTC().AddDate(0, 0, -cfg.RawDays).Truncate(time.Hour)

	// Aggregate everything below the cutoff before deleting it (spec §14.3
	// step 2: verify aggregate completeness).
	if _, err := s.aggregates.AggregateRange(time.Time{}, cutoff); err != nil {
		s.events.Error("retention_aggregate_failed",
			"retention run aborted: pre-delete aggregation failed", "system",
			map[string]any{"error": err.Error()})
		return
	}
	deleted, err := s.readings.DeleteRawBefore(cutoff)
	if err != nil {
		s.events.Error("retention_delete_failed", "retention delete failed", "system",
			map[string]any{"error": err.Error()})
		return
	}
	if deleted > 0 {
		s.events.Info("retention_run",
			"retention job complete: raw readings pruned after aggregation", "system",
			map[string]any{"deleted_rows": deleted, "raw_days": cfg.RawDays})
	}
}
