package system

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func SetupLogging() {
	SetupLoggingTo(os.Stdout)
}

// SetupLoggingTo configures the global zerolog logger to write to out, using
// the same console format the control plane uses. ANSI colour is enabled only
// when out is a terminal, so logs captured to a file or scraped by docker /
// k8s do not contain raw escape sequences.
func SetupLoggingTo(out *os.File) {
	output := zerolog.ConsoleWriter{
		Out:        out,
		TimeFormat: time.RFC3339,
		NoColor:    !isTerminal(out),
	}
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

// isTerminal reports whether f is a character device (a TTY). When stdout is
// redirected to a file or piped (compose logs, k8s, drone, customer bundles),
// this returns false so ConsoleWriter does not bake ANSI escape codes into
// captured logs.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
