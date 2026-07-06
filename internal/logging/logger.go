package logging

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

// New builds the process logger. JSON to stderr by default (journald-friendly);
// pretty console output for interactive development.
func New(level string, pretty bool) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	var logger zerolog.Logger
	if pretty {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.TimeOnly})
	} else {
		logger = zerolog.New(os.Stderr)
	}
	return logger.Level(lvl).With().Timestamp().Logger()
}
