package notify

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Weekly summary (2026-11-17): one short message on Monday morning saying how
// last week went, so a normal week does not need the dashboard opened.

// CatSummary is the switch for it on each phone; on by default.
const CatSummary = "summary"

// WeeklyShelf is one shelf's week as measured by the rack.
type WeeklyShelf struct {
	Name      string // "Shelf 2"
	Batch     string // batch code on the shelf now, "" if none recorded
	Waterings int
	Low, High *float64 // moisture range in percent; nil = no readings
}

const weekLayout = "2006-01-02"

// WeekDue reports whether the summary for the week before `now` should go
// out: it is Monday 08:00 or later in the current week and that week has not
// been sent. A box that was off on Monday morning sends when it comes back.
// from/to bound the week being summarised, Monday to Monday.
func WeekDue(now time.Time, lastSent string) (key string, from, to time.Time, due bool) {
	back := (int(now.Weekday()) + 6) % 7 // days since Monday
	monday := time.Date(now.Year(), now.Month(), now.Day()-back, 0, 0, 0, 0, now.Location())
	key = monday.Format(weekLayout)
	from, to = monday.AddDate(0, 0, -7), monday
	due = key != lastSent && !now.Before(monday.Add(8*time.Hour))
	return
}

// WeeklyText writes the message body. ok is false when the rack measured
// nothing all week (a new unit), in which case nothing is sent.
func WeeklyText(shelves []WeeklyShelf) (body string, ok bool) {
	var parts []string
	for _, s := range shelves {
		if s.Waterings == 0 && s.Low == nil && s.High == nil {
			continue
		}
		p := s.Name
		if s.Batch != "" {
			p += " (" + s.Batch + ")"
		}
		w := "waterings"
		if s.Waterings == 1 {
			w = "watering"
		}
		p += fmt.Sprintf(": %d %s", s.Waterings, w)
		if s.Low != nil && s.High != nil {
			p += fmt.Sprintf(", moisture %.0f–%.0f%%", *s.Low, *s.High)
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, ". ") + ".", true
}

// StartWeekly checks every ten minutes whether the weekly summary is due and
// sends it once. build reads the week from the database; lastSent/markSent
// keep the sent week across restarts.
func (s *Service) StartWeekly(ctx context.Context, build func(from, to time.Time) ([]WeeklyShelf, error),
	lastSent func() string, markSent func(string)) {
	check := func() {
		key, from, to, due := WeekDue(time.Now(), lastSent())
		if !due {
			return
		}
		shelves, err := build(from, to)
		if err != nil {
			s.log.Warn().Err(err).Msg("weekly summary: could not read the week")
			return // try again next tick
		}
		markSent(key)
		body, ok := WeeklyText(shelves)
		if !ok {
			return
		}
		s.Deliver(ctx, "weekly:"+key, Message{Category: CatSummary,
			Title: "Last week on the rack", Body: body})
	}
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		check()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				check()
			}
		}
	}()
}
