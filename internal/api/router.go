package api

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"propagation-controller/internal/auth"
	"propagation-controller/internal/bus"
	"propagation-controller/internal/devices"
	"propagation-controller/internal/devices/sensors"
	"propagation-controller/internal/irrigation"
	"propagation-controller/internal/lighting"
	"propagation-controller/internal/logging"
	"propagation-controller/internal/safety"
	"propagation-controller/internal/storage"
)

type Server struct {
	mgr          *devices.Manager
	sensors      *sensors.Service
	safety       *safety.Supervisor
	irrigation   *irrigation.Engine
	lighting     *lighting.Service
	auth         *auth.Service
	bus          *bus.Hub
	moisture     *storage.MoistureRuleRepo
	schedules    *storage.ScheduleRepo
	aggregates   *storage.AggregateRepo
	events       *storage.EventRepo
	actions      *storage.RelayActionRepo
	readings     *storage.ReadingRepo
	deviceRepo   *storage.DeviceRepo
	settingsRepo *storage.SettingsRepo
	zones        *storage.ZoneRepo
	cycles       *storage.CycleRepo
	profiles     *storage.SpeciesProfileRepo
	eventLog     *logging.EventLogger
	db           *sql.DB
	log          zerolog.Logger
	startedAt    time.Time
	simulation   bool
}

type Deps struct {
	Manager    *devices.Manager
	Sensors    *sensors.Service
	Safety     *safety.Supervisor
	Irrigation *irrigation.Engine
	Lighting   *lighting.Service
	Auth       *auth.Service
	Bus        *bus.Hub
	Moisture   *storage.MoistureRuleRepo
	Schedules  *storage.ScheduleRepo
	Aggregates *storage.AggregateRepo
	Events     *storage.EventRepo
	Actions    *storage.RelayActionRepo
	Readings   *storage.ReadingRepo
	Devices    *storage.DeviceRepo
	Settings   *storage.SettingsRepo
	Zones      *storage.ZoneRepo
	Cycles     *storage.CycleRepo
	EventLog   *logging.EventLogger
	DB         *sql.DB
	Log        zerolog.Logger
	Simulation bool
}

func NewRouter(d Deps) http.Handler {
	s := &Server{
		mgr:          d.Manager,
		sensors:      d.Sensors,
		safety:       d.Safety,
		irrigation:   d.Irrigation,
		lighting:     d.Lighting,
		auth:         d.Auth,
		bus:          d.Bus,
		moisture:     d.Moisture,
		schedules:    d.Schedules,
		aggregates:   d.Aggregates,
		events:       d.Events,
		actions:      d.Actions,
		readings:     d.Readings,
		deviceRepo:   d.Devices,
		settingsRepo: d.Settings,
		zones:        d.Zones,
		cycles:       d.Cycles,
		// change 3: species profiles repo derives from the shared DB handle —
		// no new Deps wiring needed at the cmd layer.
		profiles: storage.NewSpeciesProfileRepo(d.DB),
		eventLog: d.EventLog,
		db:           d.DB,
		log:          d.Log,
		startedAt:    time.Now(),
		simulation:   d.Simulation,
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(s.requestLogger)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.authMiddleware)

		r.Get("/auth/session", s.handleAuthSession)
		r.Post("/auth/setup", s.handleAuthSetup)
		r.Post("/auth/login", s.handleAuthLogin)
		r.Post("/auth/logout", s.handleAuthLogout)
		r.Get("/users", s.handleListUsers)
		r.Post("/users", s.handleCreateUser)
		r.Patch("/users/{username}", s.handleUpdateUser)
		r.Delete("/users/{username}", s.handleDeleteUser)

		r.Get("/health", s.handleHealth)

		r.Get("/relays", s.handleListRelays)
		r.Get("/relays/{channel}", s.handleGetRelay)
		r.Post("/relays/{channel}/set", s.handleSetRelay)
		r.Post("/relays/all-off", s.handleAllOff)

		r.Get("/devices", s.handleListDevices)

		r.Get("/sensors", s.handleListSensors)
		r.Get("/sensors/{key}/latest", s.handleSensorLatest)
		r.Get("/sensors/{key}/readings", s.handleSensorReadings)
		r.Post("/sensors/{key}/poll", s.handleSensorPoll)
		r.Patch("/sensors/{key}/config", s.handleSensorConfig)

		r.Get("/diagnostics/modbus", s.handleModbusDiagnostics)
		r.Post("/diagnostics/modbus/scan", s.handleModbusScan)

		r.Get("/irrigation/zones", s.handleListZones)
		r.Get("/irrigation/zones/{zone_key}", s.handleGetZone)
		r.Patch("/irrigation/zones/{zone_key}", s.handlePatchZone)
		r.Post("/irrigation/zones/{zone_key}/run", s.handleRunZone)
		r.Post("/irrigation/zones/{zone_key}/stop", s.handleStopZone)
		r.Get("/irrigation/cycles", s.handleListCycles)
		r.Get("/irrigation/cycles/{cycle_id}", s.handleGetCycle)
		r.Get("/irrigation/zones/{zone_key}/moisture-control", s.handleGetMoistureControl)
		r.Put("/irrigation/zones/{zone_key}/moisture-control", s.handlePutMoistureControl)

		r.Get("/schedules", s.handleListSchedules)
		r.Get("/schedules/{schedule_key}", s.handleGetSchedule)
		r.Post("/schedules", s.handleCreateSchedule)
		r.Put("/schedules/{schedule_key}", s.handlePutSchedule)
		r.Delete("/schedules/{schedule_key}", s.handleDeleteSchedule)

		r.Get("/lighting/status", s.handleLightingStatus)
		r.Post("/lighting/on", s.handleLightingOn)
		r.Post("/lighting/off", s.handleLightingOff)
		r.Post("/lighting/override", s.handleLightingOverride)
		r.Get("/lighting/schedule", s.handleGetLightingSchedule)
		r.Put("/lighting/schedule", s.handlePutLightingSchedule)

		r.Get("/species", s.handleListSpecies)
		// change 3: species profiles CRUD (per-species, per-stage saved settings)
		r.Get("/species-profiles", s.handleListSpeciesProfiles)
		r.Post("/species-profiles", s.handleCreateSpeciesProfile)
		r.Get("/species-profiles/{id}", s.handleGetSpeciesProfile)
		r.Put("/species-profiles/{id}", s.handlePutSpeciesProfile)
		r.Delete("/species-profiles/{id}", s.handleDeleteSpeciesProfile)

		r.Get("/safety/status", s.handleSafetyStatus)
		r.Post("/safety/emergency-stop", s.handleEmergencyStop)
		r.Post("/safety/reset", s.handleSafetyReset)
		r.Get("/safety/lockouts", s.handleListLockouts)
		r.Post("/safety/lockouts/{lockout_key}/clear", s.handleClearLockout)

		r.Get("/settings", s.handleGetSettings)
		r.Put("/settings", s.handlePutSettings)

		r.Get("/events", s.handleListEvents)
		r.Get("/events/stream", s.handleEventStream)
		r.Get("/relay-actions", s.handleListRelayActions)
	})

	s.uiRoutes(r)
	return r
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.log.Debug().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", ww.Status()).
			Dur("duration", time.Since(start)).
			Msg("http request")
	})
}
