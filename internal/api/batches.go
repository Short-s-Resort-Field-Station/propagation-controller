package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"propagation-controller/internal/storage"
)

// Propagation batches (2026-11-17): what is on the rack, where it came from,
// how many, and a dated log of what happened to it. The shape is the shared
// two-sheet batch format (docs/BATCH-FORMAT.md) so the same rows export to CSV
// and can be read by Long Watch or kept by hand in a spreadsheet.

// The vocabularies mirror Long Watch's so a file moves between them unchanged.
var (
	batchMethods = map[string]bool{"": true, "water_rooting": true, "perlite_coir": true,
		"direct_stick": true, "flood_tray": true, "cold_moist_strat": true, "warm_strat": true,
		"scarification": true, "direct_sow": true, "other": true}
	batchStatuses = map[string]bool{"active": true, "rooted": true, "hardening": true,
		"planted_out": true, "failed": true, "archived": true}
	// Events a person may log. "started" and "moved" are also written by the
	// controller itself; "conditions" exists only in the export.
	batchEvents = map[string]bool{"sown": true, "germinated": true, "counted": true,
		"potted_up": true, "moved": true, "planted_out": true, "lost": true, "note": true}
	dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

const dateLayout = "2006-01-02"

func validDate(s string) bool {
	if !dateRe.MatchString(s) {
		return false
	}
	_, err := time.Parse(dateLayout, s)
	return err == nil
}

type batchBody struct {
	BatchCode    *string `json:"batch_code"`
	Species      *string `json:"species"`
	SpeciesCode  *string `json:"species_code"`
	Source       *string `json:"source"`
	StartedOn    *string `json:"started_on"`
	Method       *string `json:"method"`
	Container    *string `json:"container"`
	Medium       *string `json:"medium"`
	CountStarted *int    `json:"count_started"`
	CountCurrent *int    `json:"count_current"`
	Status       *string `json:"status"`
	ZoneKey      *string `json:"zone_key"`
	Notes        *string `json:"notes"`
}

// apply copies the fields present in the body onto b and validates the result.
func (s *Server) applyBatchBody(b *storage.Batch, body batchBody) error {
	set := func(dst *string, src *string) {
		if src != nil {
			*dst = strings.TrimSpace(*src)
		}
	}
	set(&b.BatchCode, body.BatchCode)
	set(&b.Species, body.Species)
	set(&b.SpeciesCode, body.SpeciesCode)
	set(&b.Source, body.Source)
	set(&b.StartedOn, body.StartedOn)
	set(&b.Method, body.Method)
	set(&b.Container, body.Container)
	set(&b.Medium, body.Medium)
	set(&b.Status, body.Status)
	set(&b.ZoneKey, body.ZoneKey)
	set(&b.Notes, body.Notes)
	if body.CountStarted != nil {
		b.CountStarted = body.CountStarted
	}
	if body.CountCurrent != nil {
		b.CountCurrent = body.CountCurrent
	}
	switch {
	case b.BatchCode == "":
		return fmt.Errorf("batch_code is required")
	case b.Species == "":
		return fmt.Errorf("species is required")
	case !validDate(b.StartedOn):
		return fmt.Errorf("started_on must be a date written YYYY-MM-DD")
	case !batchMethods[b.Method]:
		return fmt.Errorf("unknown method %q", b.Method)
	case !batchStatuses[b.Status]:
		return fmt.Errorf("unknown status %q", b.Status)
	case b.CountStarted != nil && *b.CountStarted < 0, b.CountCurrent != nil && *b.CountCurrent < 0:
		return fmt.Errorf("counts cannot be negative")
	}
	if b.ZoneKey != "" {
		z, err := s.zones.GetByKey(b.ZoneKey)
		if err != nil {
			return err
		}
		if z == nil {
			return fmt.Errorf("no shelf %q", b.ZoneKey)
		}
	}
	return nil
}

func batchActor(r *http.Request) string {
	if u := currentUser(r); u != nil {
		return u.Username
	}
	return ""
}

type batchView struct {
	storage.Batch
	Log []storage.BatchLogEntry `json:"log,omitempty"`
}

func (s *Server) handleListBatches(w http.ResponseWriter, r *http.Request) {
	list, err := s.batches.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"batches": list})
}

func (s *Server) batchFromPath(w http.ResponseWriter, r *http.Request) *storage.Batch {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "batch id must be a number")
		return nil
	}
	b, err := s.batches.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return nil
	}
	if b == nil {
		writeError(w, http.StatusNotFound, "no such batch")
		return nil
	}
	return b
}

func (s *Server) writeBatch(w http.ResponseWriter, status int, id int64) {
	b, err := s.batches.Get(id)
	if err != nil || b == nil {
		writeError(w, http.StatusInternalServerError, "batch saved but could not be read back")
		return
	}
	log, err := s.batches.Log(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, status, batchView{Batch: *b, Log: log})
}

func (s *Server) handleGetBatch(w http.ResponseWriter, r *http.Request) {
	if b := s.batchFromPath(w, r); b != nil {
		s.writeBatch(w, http.StatusOK, b.ID)
	}
}

func (s *Server) handleCreateBatch(w http.ResponseWriter, r *http.Request) {
	var body batchBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	b := storage.Batch{Status: "active", StartedOn: time.Now().Format(dateLayout)}
	if err := s.applyBatchBody(&b, body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if b.CountCurrent == nil {
		b.CountCurrent = b.CountStarted
	}
	if dup, err := s.batches.GetByCode(b.BatchCode); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if dup != nil {
		writeError(w, http.StatusConflict, fmt.Sprintf("batch code %q is already in use", b.BatchCode))
		return
	}
	actor := batchActor(r)
	// The opening line dates the batch and records where it started.
	opening := []storage.BatchLogEntry{{OccurredOn: b.StartedOn, Event: "started", Count: b.CountStarted, ZoneKey: b.ZoneKey, Actor: actor}}
	id, err := s.batches.Insert(b, opening)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.eventLog.Info("batch_created", fmt.Sprintf("batch %s (%s) started", b.BatchCode, b.Species), "api", nil)
	s.writeBatch(w, http.StatusCreated, id)
}

func (s *Server) handlePatchBatch(w http.ResponseWriter, r *http.Request) {
	b := s.batchFromPath(w, r)
	if b == nil {
		return
	}
	var body batchBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	before := *b
	if err := s.applyBatchBody(b, body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if b.BatchCode != before.BatchCode {
		if dup, err := s.batches.GetByCode(b.BatchCode); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		} else if dup != nil {
			writeError(w, http.StatusConflict, fmt.Sprintf("batch code %q is already in use", b.BatchCode))
			return
		}
	}
	// A change of shelf is history, so it goes in the log, dated today.
	var entries []storage.BatchLogEntry
	if b.ZoneKey != before.ZoneKey {
		entries = append(entries, storage.BatchLogEntry{OccurredOn: time.Now().Format(dateLayout),
			Event: "moved", ZoneKey: b.ZoneKey, Actor: batchActor(r)})
	}
	if err := s.batches.Update(*b, entries); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeBatch(w, http.StatusOK, b.ID)
}

type batchLogBody struct {
	Date  string `json:"date"`
	Event string `json:"event"`
	Count *int   `json:"count"`
	Note  string `json:"note"`
	// ZoneKey is read for "moved" only: the shelf moved to, "" = off the rack.
	ZoneKey *string `json:"zone_key"`
}

// handleBatchLog adds a dated line. count is the number alive AFTER the event,
// for every event, so the newest count in the log is always the batch's count.
func (s *Server) handleBatchLog(w http.ResponseWriter, r *http.Request) {
	b := s.batchFromPath(w, r)
	if b == nil {
		return
	}
	var body batchLogBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if body.Date == "" {
		body.Date = time.Now().Format(dateLayout)
	}
	body.Note = strings.TrimSpace(body.Note)
	switch {
	case !validDate(body.Date):
		writeError(w, http.StatusBadRequest, "date must be written YYYY-MM-DD")
		return
	case !batchEvents[body.Event]:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown event %q", body.Event))
		return
	case body.Count != nil && *body.Count < 0:
		writeError(w, http.StatusBadRequest, "count cannot be negative")
		return
	case body.Event == "note" && body.Note == "":
		writeError(w, http.StatusBadRequest, "a note needs some text")
		return
	}
	entry := storage.BatchLogEntry{OccurredOn: body.Date, Event: body.Event, Count: body.Count,
		ZoneKey: b.ZoneKey, Note: body.Note, Actor: batchActor(r)}
	if body.Count != nil {
		b.CountCurrent = body.Count
	}
	switch body.Event {
	case "moved":
		if body.ZoneKey == nil {
			writeError(w, http.StatusBadRequest, `a move needs zone_key: the shelf moved to, or "" for off the rack`)
			return
		}
		dest := strings.TrimSpace(*body.ZoneKey)
		if dest != "" {
			if z, err := s.zones.GetByKey(dest); err != nil || z == nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("no shelf %q", dest))
				return
			}
		}
		b.ZoneKey, entry.ZoneKey = dest, dest
	case "planted_out":
		// Planted out leaves the rack and closes the batch.
		b.Status, b.ZoneKey, entry.ZoneKey = "planted_out", "", ""
	}
	if err := s.batches.Update(*b, []storage.BatchLogEntry{entry}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeBatch(w, http.StatusCreated, b.ID)
}

func (s *Server) handleDeleteBatch(w http.ResponseWriter, r *http.Request) {
	b := s.batchFromPath(w, r)
	if b == nil {
		return
	}
	if err := s.batches.Delete(b.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.eventLog.Info("batch_deleted", fmt.Sprintf("batch %s deleted", b.BatchCode), "api", nil)
	w.WriteHeader(http.StatusNoContent)
}

// ---- export: the two sheets -------------------------------------------------

var (
	batchSheetHeader = []string{"batch_code", "species", "species_code", "source", "started_on", "method",
		"container", "medium", "count_started", "count_current", "status", "location", "notes"}
	logSheetHeader = []string{"batch_code", "date", "event", "count", "location", "note",
		"waterings", "moisture_low_pct", "moisture_high_pct", "light_hours", "conditions_source"}
)

func intCell(n *int) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(*n)
}

func pctCell(f *float64) string {
	if f == nil {
		return ""
	}
	return strconv.FormatFloat(*f, 'f', 1, 64)
}

// shelfNames maps zone_key to the words a person reads: "Shelf 2".
func (s *Server) shelfNames() map[string]string {
	out := map[string]string{}
	if zs, err := s.zones.List(); err == nil {
		for _, z := range zs {
			out[z.ZoneKey] = z.Name
		}
	}
	return out
}

func (s *Server) exportGate(w http.ResponseWriter, r *http.Request, filename string) *csv.Writer {
	// Seed sources and counts are the grower's records, and the read API is
	// reachable from the internet, so exports are for signed-in people.
	if currentUser(r) == nil {
		writeError(w, http.StatusUnauthorized, "sign in to export")
		return nil
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	return csv.NewWriter(w)
}

func (s *Server) handleExportBatches(w http.ResponseWriter, r *http.Request) {
	list, err := s.batches.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cw := s.exportGate(w, r, "propagation-batches-v1.csv")
	if cw == nil {
		return
	}
	names := s.shelfNames()
	_ = cw.Write(batchSheetHeader)
	for _, b := range list {
		_ = cw.Write([]string{b.BatchCode, b.Species, b.SpeciesCode, b.Source, b.StartedOn, b.Method,
			b.Container, b.Medium, intCell(b.CountStarted), intCell(b.CountCurrent), b.Status,
			names[b.ZoneKey], b.Notes})
	}
	cw.Flush()
}

// shelfSpan is a stretch of days a batch sat on one shelf: [From, To).
type shelfSpan struct {
	ZoneKey  string
	From, To time.Time
}

// shelfSpans reads a batch's log for where it sat and when. A "started" or
// "moved" line opens a span on its shelf and closes the one before; a
// "planted_out" line closes the last. An open span runs to `until`.
func shelfSpans(log []storage.BatchLogEntry, until time.Time) []shelfSpan {
	var spans []shelfSpan
	var cur *shelfSpan
	closeAt := func(t time.Time) {
		if cur != nil {
			if t.After(cur.From) {
				cur.To = t
				spans = append(spans, *cur)
			}
			cur = nil
		}
	}
	for _, e := range log {
		if e.Event != "started" && e.Event != "moved" && e.Event != "planted_out" {
			continue
		}
		day, err := time.ParseInLocation(dateLayout, e.OccurredOn, until.Location())
		if err != nil {
			continue
		}
		closeAt(day)
		if e.Event != "planted_out" && e.ZoneKey != "" {
			cur = &shelfSpan{ZoneKey: e.ZoneKey, From: day}
		}
	}
	closeAt(until)
	return spans
}

// weekChunks cuts a span at Monday midnights, so measured conditions are
// reported a week at a time.
func weekChunks(sp shelfSpan) []shelfSpan {
	var out []shelfSpan
	from := sp.From
	for from.Before(sp.To) {
		daysToMonday := (8 - int(from.Weekday())) % 7
		if daysToMonday == 0 {
			daysToMonday = 7
		}
		to := time.Date(from.Year(), from.Month(), from.Day()+daysToMonday, 0, 0, 0, 0, from.Location())
		if to.After(sp.To) {
			to = sp.To
		}
		out = append(out, shelfSpan{ZoneKey: sp.ZoneKey, From: from, To: to})
		from = to
	}
	return out
}

func (s *Server) handleExportBatchLog(w http.ResponseWriter, r *http.Request) {
	list, err := s.batches.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	all, err := s.batches.Log(0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cw := s.exportGate(w, r, "propagation-log-v1.csv")
	if cw == nil {
		return
	}
	byBatch := map[int64][]storage.BatchLogEntry{}
	for _, e := range all {
		byBatch[e.BatchID] = append(byBatch[e.BatchID], e)
	}
	names := s.shelfNames()
	now := time.Now()
	_ = cw.Write(logSheetHeader)
	for _, b := range list {
		log := byBatch[b.ID]
		for _, e := range log {
			_ = cw.Write([]string{b.BatchCode, e.OccurredOn, e.Event, intCell(e.Count), names[e.ZoneKey], e.Note, "", "", "", "", ""})
		}
		// What the rack measured while the batch sat on it, a week per row,
		// dated the last day of that week. light_hours is in the format but
		// this unit does not yet total it, so the cell is left empty.
		for _, sp := range shelfSpans(log, now) {
			for _, wk := range weekChunks(sp) {
				c, err := s.batches.Conditions(wk.ZoneKey, wk.From, wk.To)
				if err != nil || (c.Waterings == 0 && c.MoistureLow == nil && c.MoistureHigh == nil) {
					continue
				}
				last := wk.To.Add(-time.Second)
				_ = cw.Write([]string{b.BatchCode, last.Format(dateLayout), "conditions", "", names[wk.ZoneKey], "",
					strconv.Itoa(c.Waterings), pctCell(c.MoistureLow), pctCell(c.MoistureHigh), "", "measured"})
			}
		}
	}
	cw.Flush()
}
