//go:build windows

package hydra

import "fmt"

func prepareRootlessPersistentDataMount(string) error {
	return fmt.Errorf("rootless persistent data mounts are not supported on Windows")
}

func prepareAgentCacheMount(string) error {
	return fmt.Errorf("agent cache mounts are not supported on Windows")
}
