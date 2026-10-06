//go:build !linux

package hydra

import "errors"

// Hydra runs dev containers on Linux only; this exists so importers
// cross-compile.
func browserSandboxSeccomp() (string, error) {
	return "", errors.New("browser sandbox containers require Linux")
}
