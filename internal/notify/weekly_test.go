package notify

import (
	"testing"
	"time"
)

func TestWeekDue(t *testing.T) {
	loc := time.FixedZone("CST", -6*3600)
	at := func(s string) time.Time { v, _ := time.ParseInLocation("2006-01-02 15:04", s, loc); return v }
	// 2026-11-16 is a Monday.
	if _, _, _, due := WeekDue(at("2026-11-16 07:59"), "2026-11-09"); due {
		t.Error("before 08:00 on Monday is too early")
	}
	key, from, to, due := WeekDue(at("2026-11-16 08:00"), "2026-11-09")
	if !due || key != "2026-11-16" {
		t.Fatalf("Monday 08:00: key %q due %v", key, due)
	}
	if from.Format(weekLayout) != "2026-11-09" || to.Format(weekLayout) != "2026-11-16" {
		t.Errorf("week = %s..%s, want the Monday-to-Monday before", from.Format(weekLayout), to.Format(weekLayout))
	}
	if _, _, _, due := WeekDue(at("2026-11-16 09:00"), "2026-11-16"); due {
		t.Error("a week already sent is not sent again")
	}
	// The box was off on Monday: Wednesday still owes this week's message.
	if key, _, _, due := WeekDue(at("2026-11-18 14:00"), "2026-11-09"); !due || key != "2026-11-16" {
		t.Errorf("Wednesday catch-up: key %q due %v", key, due)
	}
	// Sunday belongs to the week that began the Monday before.
	if key, _, _, _ := WeekDue(at("2026-11-22 23:00"), ""); key != "2026-11-16" {
		t.Errorf("Sunday key = %q", key)
	}
}

func TestWeeklyText(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	body, ok := WeeklyText([]WeeklyShelf{
		{Name: "Shelf 1", Batch: "M-07", Waterings: 3, Low: f(24.4), High: f(41.2)},
		{Name: "Shelf 2"}, // nothing measured: left out
		{Name: "Shelf 3", Waterings: 1},
	})
	want := "Shelf 1 (M-07): 3 waterings, moisture 24–41%. Shelf 3: 1 watering."
	if !ok || body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
	if _, ok := WeeklyText([]WeeklyShelf{{Name: "Shelf 1"}}); ok {
		t.Error("a week with nothing measured sends nothing")
	}
}
