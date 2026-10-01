package main

import (
	"log"
	"os/exec"
	"path/filepath"
)

// resolveContextServerCommands returns the context servers with every stdio
// `command` resolved to an absolute path through this container's PATH.
//
// Zed forwards stdio servers verbatim in ACP session/new, where the command is
// defined as a path. Some agents exec it through PATH and accept `bash` or
// `npx`; DeepSeek Harness requires an absolute path and fails session creation
// otherwise, so the settings must carry the resolved path. An unresolvable
// command is left unchanged: the agent reports it when it tries to start it.
func resolveContextServerCommands(servers map[string]interface{}) map[string]interface{} {
	resolved := make(map[string]interface{}, len(servers))
	for name, entry := range servers {
		server, ok := entry.(map[string]interface{})
		if !ok {
			resolved[name] = entry
			continue
		}
		command, ok := server["command"].(string)
		if !ok || command == "" || filepath.IsAbs(command) {
			resolved[name] = server
			continue
		}
		path, err := exec.LookPath(command)
		if err != nil {
			log.Printf("MCP server %q: command %q not found in PATH: %v", name, command, err)
			resolved[name] = server
			continue
		}
		copied := make(map[string]interface{}, len(server))
		for k, v := range server {
			copied[k] = v
		}
		copied["command"], _ = filepath.Abs(path)
		resolved[name] = copied
	}
	return resolved
}
