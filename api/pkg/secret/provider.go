package secret

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ProviderAPIKeyDirectory = "/run/secrets"

func ReadProviderAPIKeyFile(path string) ([]byte, error) {
	return readFileUnderRoot(path, ProviderAPIKeyDirectory)
}

func readFileUnderRoot(path, rootDir string) ([]byte, error) {
	rel, err := filepath.Rel(rootDir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("provider API key file must be under %s", rootDir)
	}

	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return nil, fmt.Errorf("open provider API key directory: %w", err)
	}
	defer root.Close()

	b, err := root.ReadFile(rel)
	if err != nil {
		return nil, fmt.Errorf("read provider API key file: %w", err)
	}
	return b, nil
}
