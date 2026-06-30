package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger is a wrapper around zerolog.Logger
type Logger struct {
	zerolog.Logger
}

// NewLogger initializes and returns a structured logger.
func NewLogger(env string) *Logger {
	// Set default time format
	zerolog.TimeFieldFormat = time.RFC3339

	var writer zerolog.LevelWriter
	if env == "development" {
		// Pretty print to console for development
		writer = zerolog.MultiLevelWriter(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "15:04:05"})
	} else {
		// JSON logs for production
		writer = zerolog.MultiLevelWriter(os.Stdout)
	}

	logLevel := zerolog.InfoLevel
	if env == "development" {
		logLevel = zerolog.DebugLevel
	}

	l := zerolog.New(writer).Level(logLevel).With().Timestamp().Logger()

	return &Logger{Logger: l}
}
