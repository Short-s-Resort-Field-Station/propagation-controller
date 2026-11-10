package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // photoperiod must not fall back to UTC on a box without zoneinfo

	"propagation-controller/internal/api"
	"propagation-controller/internal/auth"
	"propagation-controller/internal/automation"
	"propagation-controller/internal/bus"
	"propagation-controller/internal/config"
	"propagation-controller/internal/devices"
	"propagation-controller/internal/devices/modbus"
	"propagation-controller/internal/devices/relay"
	"propagation-controller/internal/devices/sensors"
	"propagation-controller/internal/irrigation"
	"propagation-controller/internal/lighting"
	"propagation-controller/internal/logging"
	"propagation-controller/internal/retention"
	"propagation-controller/internal/safety"
	"propagation-controller/internal/storage"
	"propagation-controller/internal/version"
)

func main() {
	cfg := config.Load()
	log := logging.New(cfg.LogLevel, cfg.LogPretty)

	log.Info().
		Str("version", version.Version).
		Bool("simulation", cfg.Simulation).
		Str("listen", cfg.ListenAddr).
		Msg("propagationd starting")

	if err := cfg.EnsureDataDir(); err != nil {
		log.Fatal().Err(err).Str("dir", cfg.DataDir).Msg("cannot create data directory")
	}

	// Startup sequence per spec §12.2: database, logger, config, relay
	// controller, safe relay state, then API.
	db, err := storage.Open(cfg.DatabasePath())
	if err != nil {
		log.Fatal().Err(err).Str("path", cfg.DatabasePath()).Msg("cannot open database")
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		log.Fatal().Err(err).Msg("database migration failed")
	}

	events := storage.NewEventRepo(db)
	actions := storage.NewRelayActionRepo(db)
	deviceRepo := storage.NewDeviceRepo(db)
	readingRepo := storage.NewReadingRepo(db)
	settingsRepo := storage.NewSettingsRepo(db)
	hub := bus.New()
	eventLog := logging.NewEventLogger(log, events)
	eventLog.AttachHub(hub)

	var ctrl relay.Controller
	driverName := "numato_usb"
	if cfg.Simulation {
		ctrl = relay.NewMock()
		driverName = "mock"
	} else {
		port := cfg.RelayPort
		if port == "" {
			port, err = relay.DetectNumatoPort()
			if err != nil {
				log.Error().Err(err).Msg("relay port autodetection failed; running degraded (set -relay-port)")
				port = "/dev/ttyACM0"
			}
		}
		log.Info().Str("port", port).Msg("using relay serial port")
		ctrl = relay.NewNumatoUSB(port)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mgr := devices.NewManager(ctrl, driverName, actions, deviceRepo, eventLog, cfg.Simulation)
	mgr.AttachHub(hub)
	if err := mgr.Start(ctx); err != nil {
		// Degraded but alive: the API and diagnostics still work so the
		// operator can see what is wrong (spec: relay offline suspends
		// automation, not the whole appliance).
		log.Error().Err(err).Msg("relay controller unavailable; API running degraded")
	}

	var modbusBus modbus.Bus
	if cfg.Simulation {
		modbusBus = modbus.NewMockBus()
	} else {
		port := cfg.ModbusPort
		if port == "" {
			port, err = modbus.DetectRS485Port()
			if err != nil {
				log.Error().Err(err).Msg("RS485 port autodetection failed; running degraded (set -modbus-port)")
				port = "/dev/ttyUSB0"
			}
		}
		log.Info().Str("port", port).Int("baud", cfg.ModbusBaud).Msg("using RS485 serial port")
		modbusBus = modbus.NewRTUBus(port, cfg.ModbusBaud)
	}
	if err := modbusBus.Open(ctx); err != nil {
		log.Error().Err(err).Msg("modbus bus unavailable; sensor polling degraded")
	}
	defer modbusBus.Close()

	sensorSvc := sensors.NewService(modbusBus, readingRepo, deviceRepo, eventLog, log, cfg.Simulation)
	sensorSvc.AttachHub(hub)
	if err := sensorSvc.EnsureDefaults(); err != nil {
		log.Error().Err(err).Msg("failed to seed default sensors")
	}
	if err := sensorSvc.Start(ctx); err != nil {
		log.Error().Err(err).Msg("sensor service failed to start")
	}

	// Safety supervisor starts before the API accepts commands (spec §12.2).
	supervisor := safety.NewSupervisor(mgr, readingRepo, settingsRepo, eventLog, log, time.Second)
	if err := supervisor.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("safety supervisor failed to start")
	}

	zoneRepo := storage.NewZoneRepo(db)
	cycleRepo := storage.NewCycleRepo(db)
	engine := irrigation.NewEngine(mgr, supervisor, zoneRepo, cycleRepo, readingRepo,
		eventLog, log, "reservoir_level_level")
	engine.AttachHub(hub)
	if err := engine.SeedDefaults(); err != nil {
		log.Error().Err(err).Msg("failed to seed irrigation zones")
	}
	// E-stop aborts the active cycle and clears the queue before the relays
	// are forced, so the engine never sits "draining" against a closed
	// valve (audit 2026-11-10, 2.6).
	supervisor.OnEmergencyStop(func() { engine.StopAll("safety_supervisor") })
	engine.Start(ctx)

	scheduleRepo := storage.NewScheduleRepo(db)
	lightSvc := lighting.NewService(mgr, supervisor, scheduleRepo, settingsRepo, eventLog, log, 15*time.Second)
	if err := lightSvc.EnsureDefault("America/Chicago"); err != nil {
		log.Error().Err(err).Msg("failed to seed lighting schedule")
	}
	lightSvc.Start(ctx)

	moistureRepo := storage.NewMoistureRuleRepo(db)
	autoSvc := automation.NewService(engine, zoneRepo, cycleRepo, moistureRepo, readingRepo,
		scheduleRepo, settingsRepo, eventLog, log, 10*time.Second)
	autoSvc.Start(ctx)

	aggregateRepo := storage.NewAggregateRepo(db)
	retentionSvc := retention.NewService(aggregateRepo, readingRepo, settingsRepo, eventLog, log)
	retentionSvc.Start(ctx)

	authSvc := auth.NewService(storage.NewUserRepo(db), storage.NewSessionRepo(db), eventLog)

	router := api.NewRouter(api.Deps{
		Manager:    mgr,
		Sensors:    sensorSvc,
		Safety:     supervisor,
		Irrigation: engine,
		Lighting:   lightSvc,
		Auth:       authSvc,
		Bus:        hub,
		Moisture:   moistureRepo,
		Schedules:  scheduleRepo,
		Aggregates: aggregateRepo,
		Events:     events,
		Actions:    actions,
		Readings:   readingRepo,
		Devices:    deviceRepo,
		Settings:   settingsRepo,
		Zones:      zoneRepo,
		Cycles:     cycleRepo,
		EventLog:   eventLog,
		DB:         db,
		Log:        log,
		Simulation: cfg.Simulation,
	})
	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: router,
		// Slow-loris protection on a box with port 80 open to the subnet;
		// no WriteTimeout because /events/stream is long-lived SSE.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("http server failed")
		}
	}()

	eventLog.Info("startup_complete", "propagationd started", "system", map[string]any{
		"version":    version.Version,
		"simulation": cfg.Simulation,
	})

	<-ctx.Done()
	log.Info().Msg("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mgr.Shutdown(shutdownCtx)
	srv.Shutdown(shutdownCtx)
	eventLog.Info("shutdown_complete", "propagationd stopped cleanly", "system", nil)
}
