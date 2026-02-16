// docker-shim: Intercepts docker and docker-compose commands to:
// 1. Inject BuildKit cache flags for shared caching across sessions
// 2. Handle Docker Compose project naming per session
//
// The shim detects its mode based on argv[0] or the first argument:
// - "docker" or argv[0] ends with "docker": act as docker wrapper
// - "compose" or argv[0] ends with "docker-compose": act as compose wrapper
//
// For docker CLI plugin mode (docker compose), the first arg is "compose"

package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	// DockerRealPath is the actual docker binary
	DockerRealPath = "/usr/bin/docker.real"

	// ComposeRealPath is the actual docker-compose plugin binary
	ComposeRealPath = "/usr/libexec/docker/cli-plugins/docker-compose.real"
)

// BuildKitCacheDir is where the shared cache is mounted (if available).
// This is a var (not const) so tests can override it for isolated testing.
var BuildKitCacheDir = "/buildkit-cache"

// Mode represents the operating mode of the shim
type Mode int

const (
	ModeDocker Mode = iota
	ModeCompose
)

func main() {
	// Configure logging - minimal by default, debug if DOCKER_SHIM_DEBUG is set
	zerolog.SetGlobalLevel(zerolog.WarnLevel)
	if os.Getenv("DOCKER_SHIM_DEBUG") != "" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	// Determine mode from how we were invoked
	mode := detectMode(os.Args)
	log.Debug().
		Str("argv0", os.Args[0]).
		Int("mode", int(mode)).
		Strs("args", os.Args[1:]).
		Msg("docker-shim starting")

	var exitCode int
	switch mode {
	case ModeDocker:
		exitCode = runDocker(os.Args[1:])
	case ModeCompose:
		exitCode = runCompose(os.Args[1:])
	}

	os.Exit(exitCode)
}

// detectMode determines whether we're acting as docker or docker-compose
func detectMode(args []string) Mode {
	if len(args) == 0 {
		return ModeDocker
	}

	// Check argv[0] - the binary name
	base := filepath.Base(args[0])
	if strings.Contains(base, "compose") {
		return ModeCompose
	}

	// Check if first argument is "compose" (docker CLI plugin mode)
	if len(args) > 1 && args[1] == "compose" {
		return ModeCompose
	}

	return ModeDocker
}
