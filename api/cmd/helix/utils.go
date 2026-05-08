package helix

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/spf13/cobra"
)

func getCommandLineExecutable() string {
	return os.Args[0]
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
