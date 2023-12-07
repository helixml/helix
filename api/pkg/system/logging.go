package system

import (
	"fmt"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func SetupLogging() {
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	logLevelString := os.Getenv("LOG_LEVEL")
	if logLevelString == "" {
		logLevelString = "info"
	}
	logLevel := zerolog.InfoLevel
	if logLevelString == "none" {
		logLevel = zerolog.NoLevel
	}
	parsedLogLevel, err := zerolog.ParseLevel(logLevelString)
	if err == nil {
		logLevel = parsedLogLevel
	}

	// edit to change the level of the stack we report
	// zerolog.CallerSkipFrameCount = 3 // Skip 3 frames (this function, log.Output, log.Logger)
	log.Logger = log.Output(output).With().Caller().Logger().Level(logLevel)
}

func Spew(things []interface{}) {
	fmt.Printf(" --------------------------------------\n")
	spew.Dump(things)
}
