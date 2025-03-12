package util

import (
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

// SyncMapping represents a mapping between a local directory and a knowledge source
type SyncMapping struct {
	LocalDir      string
	RemotePath    string
	KnowledgeName string
}

// ParseSyncMappings parses the sync mappings from command line arguments
func ParseSyncMappings(syncArgs []string, appConfig *types.AppHelixConfig) ([]SyncMapping, error) {
	mappings := []SyncMapping{}

	// Get all knowledge sources from the app config
	knowledgeSources := []struct {
		name       string
		remotePath string
	}{}

	for _, assistant := range appConfig.Assistants {
		for _, knowledge := range assistant.Knowledge {
			if knowledge.Source.Filestore != nil && knowledge.Source.Filestore.Path != "" {
				knowledgeSources = append(knowledgeSources, struct {
					name       string
					remotePath string
				}{
					name:       knowledge.Name,
					remotePath: knowledge.Source.Filestore.Path,
				})
			}
		}
	}

	if len(knowledgeSources) == 0 {
		return nil, fmt.Errorf("no filestore knowledge sources found in the app config")
	}

	// Process each sync argument
	for _, syncArg := range syncArgs {
		localDir := syncArg
		targetKnowledge := ""

		// Check if there's a colon in the argument
		if strings.Contains(syncArg, ":") {
			parts := strings.SplitN(syncArg, ":", 2)
			localDir = parts[0]
			targetKnowledge = parts[1]
		}

		// If no target knowledge specified, use the first one
		if targetKnowledge == "" {
			mappings = append(mappings, SyncMapping{
				LocalDir:      localDir,
				RemotePath:    knowledgeSources[0].remotePath,
				KnowledgeName: knowledgeSources[0].name,
			})
			continue
		}

		// Find the matching knowledge source
		found := false
		for _, source := range knowledgeSources {
			if source.name == targetKnowledge {
				mappings = append(mappings, SyncMapping{
					LocalDir:      localDir,
					RemotePath:    source.remotePath,
					KnowledgeName: source.name,
				})
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("knowledge source '%s' not found in app config", targetKnowledge)
		}
	}

	return mappings, nil
}
