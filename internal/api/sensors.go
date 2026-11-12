package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"propagation-controller/internal/devices/sensors"
	"propagation-controller/internal/storage"
)

func (s *Server) handleListSensors(w http.ResponseWriter, r *http.Request) {
	list, err := s.sensors.ListSensors(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []sensors.SensorStatus{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sensors": list,
		"bus":     s.sensors.BusHealth(r.Context()),
	})
}

// handleSensorLatest accepts either a sensor device key (returns all of its
// measurements) or a measurement sensor_key like soil_zone_1_moisture.
func (s *Server) handleSensorLatest(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	dev, err := s.sensors.GetSensor(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dev != nil {
		writeJSON(w, http.StatusOK, dev)
		return
	}
	latest, err := s.readings.Latest(key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if latest == nil {
		writeError(w, http.StatusNotFound, "unknown sensor key "+key)
		return
	}
	writeJSON(w, http.StatusOK, latest)
}

func (s *Server) handleSensorReadings(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	q := r.URL.Query()
	from, ok := parseTimeParam(w, q.Get("from"), "from")
	if !ok {
		return
	}
	to, ok := parseTimeParam(w, q.Get("to"), "to")
	if !ok {
		return
	}

	// interval=hour serves the hourly aggregates (spec §15.3's interval
	// param) — the long-term series that outlives raw retention.
	switch q.Get("interval") {
	case "", "raw":
		readings, err := s.readings.List(key, from, to, parseIntOr(q.Get("limit"), 1000))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if readings == nil {
			readings = []storage.StoredReading{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"sensor_key": key, "interval": "raw", "readings": readings})
	case "hour", "hourly":
		aggs, err := s.aggregates.List(key, from, to, parseIntOr(q.Get("limit"), 720))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if aggs == nil {
			aggs = []storage.HourlyAggregate{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"sensor_key": key, "interval": "hour", "aggregates": aggs})
	default:
		// interval=5m / 15m / 30m…: raw readings summarised into fixed buckets
		// (min/avg/max) for the dashboard's 24-hour timeline (2026-11-11).
		// Sub-hour buckets scan raw rows, so the window is capped at 7 days.
		bucket, err := time.ParseDuration(q.Get("interval"))
		if err != nil || bucket < time.Minute || bucket > 6*time.Hour {
			writeError(w, http.StatusBadRequest, `interval must be "raw", "hour", or a bucket width between 1m and 6h (e.g. 5m)`)
			return
		}
		end := time.Now().UTC()
		if to != nil {
			end = *to
		}
		start := end.Add(-24 * time.Hour)
		if from != nil {
			start = *from
		}
		if end.Sub(start) > 7*24*time.Hour {
			writeError(w, http.StatusBadRequest, "bucketed readings cover at most 7 days; use interval=hour for longer ranges")
			return
		}
		aggs, err := s.readings.Bucketed(key, start, end, bucket)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sensor_key": key, "interval": q.Get("interval"), "bucket_seconds": int(bucket.Seconds()), "aggregates": aggs})
	}
}

func (s *Server) handleSensorPoll(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	readings, err := s.sensors.PollNow(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"readings": readings})
}

func (s *Server) handleSensorConfig(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var cfg sensors.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if err := s.sensors.UpdateConfig(key, cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleModbusDiagnostics(w http.ResponseWriter, r *http.Request) {
	list, err := s.sensors.ListSensors(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bus":     s.sensors.BusHealth(r.Context()),
		"sensors": list,
	})
}

func (s *Server) handleModbusScan(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	start := parseIntOr(q.Get("start"), 1)
	end := parseIntOr(q.Get("end"), 32)
	if start < 1 || end > 247 || end < start {
		writeError(w, http.StatusBadRequest, "scan range must satisfy 1 <= start <= end <= 247")
		return
	}
	// A scan holds the bus for up to ~50 s; the reservoir reading goes stale
	// and the supervisor aborts a running flood (audit 2026-11-10, 2.8).
	if active := s.irrigation.Status().Active; active != nil {
		writeError(w, http.StatusConflict,
			"irrigation cycle active on "+active.ZoneKey+" — wait for it to finish before scanning the bus")
		return
	}
	found, err := s.sensors.ScanBus(r.Context(), uint8(start), uint8(end))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	// []uint8 is []byte to encoding/json (base64), so widen to ints.
	addresses := make([]int, 0, len(found))
	for _, a := range found {
		addresses = append(addresses, int(a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"responding_addresses": addresses, "start": start, "end": end})
}
