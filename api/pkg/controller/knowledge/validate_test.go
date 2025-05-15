package knowledge

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	basicContent := "Hello, world!"

	tests := []struct {
		name        string
		knowledge   *types.AssistantKnowledge
		expectError bool
	}{
		{
			name: "Empty name",
			knowledge: &types.AssistantKnowledge{
				Name: "",
				Source: types.KnowledgeSource{
					Filestore: &types.KnowledgeSourceHelixFilestore{
						Path: "/test",
					},
				},
			},
			expectError: true,
		},
		{
			name: "Empty source",
			knowledge: &types.AssistantKnowledge{
				Name: "Test",
			},
			expectError: true,
		},
		{
			name: "Valid cron schedule",
			knowledge: &types.AssistantKnowledge{
				Name:            "Test",
				RefreshSchedule: "0 0 * * *", // Every 24 hours
				Source: types.KnowledgeSource{
					Filestore: &types.KnowledgeSourceHelixFilestore{
						Path: "/test",
					},
				},
			},
			expectError: false,
		},
		{
			name: "Invalid cron schedule - too frequent",
			knowledge: &types.AssistantKnowledge{
				Name:            "Test",
				RefreshSchedule: "*/5 * * * *", // Every 5 minutes
				Source: types.KnowledgeSource{
					Filestore: &types.KnowledgeSourceHelixFilestore{
						Path: "/test",
					},
				},
			},
			expectError: true,
		},
		{
			name: "Invalid humanized schedule - too frequent",
			knowledge: &types.AssistantKnowledge{
				Name:            "Test",
				RefreshSchedule: "@every 5m",
				Source: types.KnowledgeSource{
					Filestore: &types.KnowledgeSourceHelixFilestore{
						Path: "/test",
					},
				},
			},
			expectError: true,
		},
		{
			name: "Valid humanized schedule",
			knowledge: &types.AssistantKnowledge{
				Name:            "Test",
				RefreshSchedule: "@every 15m",
				Source: types.KnowledgeSource{
					Filestore: &types.KnowledgeSourceHelixFilestore{
						Path: "/test",
					},
				},
			},
			expectError: false,
		},
		{
			name: "Invalid cron syntax",
			knowledge: &types.AssistantKnowledge{
				Name:            "Test",
				RefreshSchedule: "invalid cron",
				Source: types.KnowledgeSource{
					Filestore: &types.KnowledgeSourceHelixFilestore{
						Path: "/test",
					},
				},
			},
			expectError: true,
		},
		{
			name: "Empty schedule",
			knowledge: &types.AssistantKnowledge{
				Name:            "Test",
				RefreshSchedule: "", Source: types.KnowledgeSource{
					Filestore: &types.KnowledgeSourceHelixFilestore{
						Path: "/test",
					},
				},
			},
			expectError: false,
		},
		{
			name: "Valid URL",
			knowledge: &types.AssistantKnowledge{
				Name: "Test",
				Source: types.KnowledgeSource{
					Web: &types.KnowledgeSourceWeb{
						URLs: []string{"https://foo.com"},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Invalid URL",
			knowledge: &types.AssistantKnowledge{
				Name: "Test",
				Source: types.KnowledgeSource{
					Web: &types.KnowledgeSourceWeb{
						URLs: []string{"invalid-url"},
					},
				},
			},
			expectError: true,
		},
		{
			name: "Invalid URL starts with https://",
			knowledge: &types.AssistantKnowledge{
				Name: "Test",
				Source: types.KnowledgeSource{
					Web: &types.KnowledgeSourceWeb{
						URLs: []string{"https://foo.com https://bar.com"},
					},
				},
			},
			expectError: true,
		},
		{
			name: "Valid content",
			knowledge: &types.AssistantKnowledge{
				Name: "Test",
				Source: types.KnowledgeSource{
					Text: &basicContent,
				},
			},
			expectError: false,
		},
	}

	serverConfig := config.ServerConfig{}
	serverConfig.RAG.Crawler.MaxFrequency = 10 * time.Minute
	serverConfig.RAG.Crawler.MaxDepth = 30

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(&serverConfig, tt.knowledge)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
