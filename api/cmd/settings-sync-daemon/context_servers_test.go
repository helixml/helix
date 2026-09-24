package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// DeepSeek Harness fails session/new for a stdio server whose command is not
// absolute, so PATH-relative commands must reach Zed already resolved.
func TestResolveContextServerCommands(t *testing.T) {
	bin := t.TempDir()
	tool := filepath.Join(bin, "fake-mcp")
	assert.NoError(t, os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755))
	t.Setenv("PATH", bin)

	in := map[string]interface{}{
		"relative": map[string]interface{}{"command": "fake-mcp", "args": []interface{}{"-x"}},
		"absolute": map[string]interface{}{"command": "/opt/tool"},
		"missing":  map[string]interface{}{"command": "not-installed"},
		"http":     map[string]interface{}{"url": "http://api:8080/mcp"},
	}
	out := resolveContextServerCommands(in)

	assert.Equal(t, tool, out["relative"].(map[string]interface{})["command"])
	assert.Equal(t, []interface{}{"-x"}, out["relative"].(map[string]interface{})["args"])
	assert.Equal(t, "/opt/tool", out["absolute"].(map[string]interface{})["command"])
	assert.Equal(t, "not-installed", out["missing"].(map[string]interface{})["command"])
	assert.Equal(t, in["http"], out["http"])
	assert.Equal(t, "fake-mcp", in["relative"].(map[string]interface{})["command"], "input must not be mutated")
}
