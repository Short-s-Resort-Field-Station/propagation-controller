package api

import (
	"testing"
	"time"

	"propagation-controller/internal/auth"
	"propagation-controller/internal/storage"
)

func day(s string) time.Time {
	t, _ := time.ParseInLocation(dateLayout, s, time.Local)
	return t
}

func TestShelfSpansFollowTheLog(t *testing.T) {
	log := []storage.BatchLogEntry{
		{OccurredOn: "2026-11-02", Event: "started", ZoneKey: "zone_1"},
		{OccurredOn: "2026-11-05", Event: "counted"},
		{OccurredOn: "2026-11-10", Event: "moved", ZoneKey: "zone_3"},
		{OccurredOn: "2026-11-14", Event: "moved", ZoneKey: ""}, // off the rack
		{OccurredOn: "2026-11-16", Event: "moved", ZoneKey: "zone_2"},
		{OccurredOn: "2026-11-20", Event: "planted_out"},
	}
	got := shelfSpans(log, day("2026-12-01"))
	want := []shelfSpan{
		{"zone_1", day("2026-11-02"), day("2026-11-10")},
		{"zone_3", day("2026-11-10"), day("2026-11-14")},
		{"zone_2", day("2026-11-16"), day("2026-11-20")},
	}
	if len(got) != len(want) {
		t.Fatalf("spans = %v, want %v", got, want)
	}
	for i := range want {
		if got[i].ZoneKey != want[i].ZoneKey || !got[i].From.Equal(want[i].From) || !got[i].To.Equal(want[i].To) {
			t.Errorf("span %d = %v, want %v", i, got[i], want[i])
		}
	}
	// A batch still on its shelf runs to "until".
	open := shelfSpans(log[:1], day("2026-11-09"))
	if len(open) != 1 || !open[0].To.Equal(day("2026-11-09")) {
		t.Fatalf("open span = %v", open)
	}
}

func TestWeekChunksCutAtMondays(t *testing.T) {
	// 2026-11-04 is a Wednesday; 2026-11-18 is a Wednesday.
	chunks := weekChunks(shelfSpan{"zone_1", day("2026-11-04"), day("2026-11-18")})
	want := [][2]string{{"2026-11-04", "2026-11-09"}, {"2026-11-09", "2026-11-16"}, {"2026-11-16", "2026-11-18"}}
	if len(chunks) != len(want) {
		t.Fatalf("chunks = %v", chunks)
	}
	for i, w := range want {
		if !chunks[i].From.Equal(day(w[0])) || !chunks[i].To.Equal(day(w[1])) {
			t.Errorf("chunk %d = %s..%s, want %s..%s", i, chunks[i].From.Format(dateLayout), chunks[i].To.Format(dateLayout), w[0], w[1])
		}
	}
}

func TestBatchRoles(t *testing.T) {
	for _, c := range []struct{ method, path, want string }{
		{"POST", "/api/v1/batches", auth.RoleOperator},
		{"PATCH", "/api/v1/batches/3", auth.RoleOperator},
		{"POST", "/api/v1/batches/3/log", auth.RoleOperator},
		{"DELETE", "/api/v1/batches/3", auth.RoleAdmin},
	} {
		if got := mutationRole(c.method, c.path); got != c.want {
			t.Errorf("%s %s needs %q, want %q", c.method, c.path, got, c.want)
		}
	}
}
