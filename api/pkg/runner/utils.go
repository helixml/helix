package runner

import (
	"fmt"
)

func URL(options RunnerOptions, path string) string {
	return fmt.Sprintf("%s%s", options.ApiURL, path)
}
