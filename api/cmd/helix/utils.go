package helix

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"errors"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"
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

func createDataPrepOpenAIClient(cfg *config.ServerConfig, helixInference openai.Client) (openai.Client, error) {
	switch cfg.FineTuning.Provider {
	case types.ProviderOpenAI:
		if cfg.Providers.OpenAI.APIKey == "" {
			return nil, errors.New("OpenAI API key (OPENAI_API_KEY) is required")
		}
		log.Info().
			Str("base_url", cfg.Providers.OpenAI.BaseURL).
			Msg("using OpenAI provider for controller inference")

		return openai.New(
			cfg.Providers.OpenAI.APIKey,
			cfg.Providers.OpenAI.BaseURL,
			cfg.Stripe.BillingEnabled,
		), nil

	case types.ProviderTogetherAI:
		if cfg.Providers.TogetherAI.APIKey == "" {
			// Fallback to Helix if no TogetherAI key
			log.Warn().Msg("TogetherAI API key not set, falling back to Helix for fine-tuning")
			return helixInference, nil
		}
		log.Info().
			Str("base_url", cfg.Providers.TogetherAI.BaseURL).
			Msg("using TogetherAI provider for controller inference")

		return openai.New(
			cfg.Providers.TogetherAI.APIKey,
			cfg.Providers.TogetherAI.BaseURL,
			cfg.Stripe.BillingEnabled,
		), nil

	case types.ProviderHelix:
		log.Info().Msg("using Helix provider for inference")
		return helixInference, nil

	default:
		return nil, errors.New("unknown inference provider")
	}
}
