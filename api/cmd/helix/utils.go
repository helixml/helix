package helix

import (
	"os"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func getCommandLineExecutable() string {
	return os.Args[0]
}

func getDefaultServeOptionString(envName string, defaultValue string) string {
	envValue := os.Getenv(envName)
	if envValue != "" {
		return envValue
	}
	return defaultValue
}

func getDefaultServeOptionBool(envName string, defaultValue bool) bool {
	envValue := os.Getenv(envName)
	if envValue != "" {
		return true
	}
	return defaultValue
}

func getDefaultServeOptionInt(envName string, defaultValue int) int {
	envValue := os.Getenv(envName)
	if envValue != "" {
		i, err := strconv.Atoi(envValue)
		if err == nil {
			return i
		}
	}
	return defaultValue
}

func getDefaultServeOptionFloat(envName string, defaultValue float32) float32 {
	envValue := os.Getenv(envName)
	if envValue != "" {
		f, err := strconv.ParseFloat(envValue, 32)
		if err == nil {
			return float32(f)
		}
	}
	return defaultValue
}

// comma separated strings
func getDefaultServeOptionStringArray(envName string, defaultValue []string) []string {
	envValue := os.Getenv(envName)
	if envValue != "" {
		parts := strings.Split(envValue, ",")
		return parts
	}
	return defaultValue
}

// comma separated key=value pairs e.g. LABELS="name=apples,height=10"
func getDefaultServeOptionMap(envName string, defaultValue map[string]string) map[string]string {
	envValue := os.Getenv(envName)
	if envValue != "" {
		parts := strings.Split(envValue, ",")
		data := make(map[string]string)
		for _, part := range parts {
			kv := strings.Split(part, "=")
			if len(kv) == 2 {
				data[kv[0]] = kv[1]
			} else {
				log.Warn().Msgf("invalid key=value pair: %s", part)
			}
		}
		return data
	}
	return defaultValue
}

func FatalErrorHandler(cmd *cobra.Command, msg string, code int) {
	if len(msg) > 0 {
		// add newline if needed
		if !strings.HasSuffix(msg, "\n") {
			msg += "\n"
		}
		cmd.Print(msg)
	}
	os.Exit(code)
}
