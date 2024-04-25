package helix

import (
	"fmt"
	"os"
	"reflect"
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
	envValue, ok := os.LookupEnv(envName)
	if ok && envValue != "" {
		parts := strings.Split(envValue, ",")
		return parts
	}

	if ok {
		// Explicitly set to empty
		return []string{}
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

func generateEnvHelpText(cfg interface{}, prefix string) string {
	var helpTextBuilder strings.Builder

	t := reflect.TypeOf(cfg)
	if t.Kind() == reflect.Ptr {
		t = t.Elem() // Get the type that the pointer refers to
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldType := field.Type
		if fieldType.Kind() == reflect.Struct {
			helpTextBuilder.WriteString(fmt.Sprintf("\n%s - %s\n\n", prefix, field.Name))
			helpTextBuilder.WriteString(generateEnvHelpText(reflect.New(fieldType).Interface(), prefix+" "))
		} else {
			// It's a leaf field
			envVar := field.Tag.Get("envconfig")
			description := field.Tag.Get("description")
			defaultValue := field.Tag.Get("default")

			if envVar != "" { // Ensure it's tagged for envconfig
				helpTextBuilder.WriteString(fmt.Sprintf("%s  %s: %s (default: \"%s\")\n", prefix, envVar, description, defaultValue))
			}
		}
	}

	return helpTextBuilder.String()
}
