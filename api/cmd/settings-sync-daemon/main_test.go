package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncInitialConfigReportsFatalConfigError(t *testing.T) {
	var configRequests int
	var startupErrorRequests int
	var reportedError string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/sessions/ses_1/zed-config":
			configRequests++
			http.Error(w, `provider "openai" is not enabled for coding-agent harness "codex_cli" in this organization`, http.StatusUnprocessableEntity)
		case "/api/v1/sessions/ses_1/agent-startup-error":
			startupErrorRequests++
			assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))
			var body struct {
				Error string `json:"error"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			reportedError = body.Error
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok","transitioned":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	daemon := &SettingsDaemon{
		httpClient: server.Client(),
		apiURL:     server.URL,
		apiToken:   "token",
		sessionID:  "ses_1",
	}

	err := daemon.syncInitialConfig(5, 0)
	require.Error(t, err)
	assert.Equal(t, 1, configRequests, "stable client errors must not be retried")
	assert.Equal(t, 1, startupErrorRequests)
	assert.True(t, strings.Contains(reportedError, "status 422"))
	assert.True(t, strings.Contains(reportedError, `provider "openai" is not enabled`))
}

func TestSyncInitialConfigDoesNotReportTransientError(t *testing.T) {
	var configRequests int
	var startupErrorRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/sessions/ses_1/zed-config":
			configRequests++
			http.Error(w, "temporary failure", http.StatusInternalServerError)
		case "/api/v1/sessions/ses_1/agent-startup-error":
			startupErrorRequests++
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	daemon := &SettingsDaemon{
		httpClient: server.Client(),
		apiURL:     server.URL,
		sessionID:  "ses_1",
	}

	err := daemon.syncInitialConfig(2, 0)
	require.Error(t, err)
	assert.Equal(t, 2, configRequests)
	assert.Zero(t, startupErrorRequests, "transient failures must not fail the interaction")
}

func TestIsFatalZedConfigError(t *testing.T) {
	tests := []struct {
		status int
		fatal  bool
	}{
		{status: http.StatusUnauthorized, fatal: true},
		{status: http.StatusForbidden, fatal: true},
		{status: http.StatusUnprocessableEntity, fatal: true},
		{status: http.StatusNotFound, fatal: false},
		{status: http.StatusConflict, fatal: false},
		{status: http.StatusTooManyRequests, fatal: false},
		{status: http.StatusInternalServerError, fatal: false},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			err := &zedConfigFetchError{StatusCode: tt.status, Message: "test"}
			assert.Equal(t, tt.fatal, isFatalZedConfigError(err))
		})
	}
}

func TestInjectAvailableModels(t *testing.T) {
	tests := []struct {
		name            string
		codeAgentConfig *CodeAgentConfig
		helixSettings   map[string]interface{}
		wantModel       string
		wantProvider    string
		wantEffort      string
		wantSkipped     bool // true if injection should be skipped (e.g. anthropic built-in)
	}{
		{
			name: "adds model to existing openai provider",
			codeAgentConfig: &CodeAgentConfig{
				Model:   "helix/qwen3:8b",
				APIType: "openai",
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"openai": map[string]interface{}{
						"api_url": "http://localhost:8080/v1",
					},
				},
			},
			wantModel:    "helix/qwen3:8b",
			wantProvider: "openai",
		},
		{
			name: "adds explicit none reasoning effort for zed agent",
			codeAgentConfig: &CodeAgentConfig{
				Model:           "openai/gpt-5.6-sol",
				APIType:         "openai",
				Runtime:         "zed_agent",
				ReasoningEffort: "none",
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"openai": map[string]interface{}{},
				},
			},
			wantModel:    "openai/gpt-5.6-sol",
			wantProvider: "openai",
			wantEffort:   "none",
		},
		{
			name: "creates provider config if missing",
			codeAgentConfig: &CodeAgentConfig{
				Model:   "helix/qwen3:8b",
				APIType: "openai",
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{},
			},
			wantModel:    "helix/qwen3:8b",
			wantProvider: "openai",
		},
		{
			name: "defaults to openai provider when APIType is empty",
			codeAgentConfig: &CodeAgentConfig{
				Model:   "custom-model",
				APIType: "",
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{},
			},
			wantModel:    "custom-model",
			wantProvider: "openai",
		},
		{
			name: "skips injection for anthropic provider — Zed has built-in definitions",
			codeAgentConfig: &CodeAgentConfig{
				Model:   "claude-opus-4-6",
				APIType: "anthropic",
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"anthropic": map[string]interface{}{
						"api_url": "http://localhost:8080",
					},
				},
			},
			wantSkipped: true,
		},
		{
			name:            "does nothing when codeAgentConfig is nil",
			codeAgentConfig: nil,
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"openai": map[string]interface{}{},
				},
			},
			wantModel:    "",
			wantProvider: "",
		},
		{
			name: "does nothing when model is empty",
			codeAgentConfig: &CodeAgentConfig{
				Model:   "",
				APIType: "openai",
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"openai": map[string]interface{}{},
				},
			},
			wantModel:    "",
			wantProvider: "",
		},
		{
			name: "does not duplicate model if already exists",
			codeAgentConfig: &CodeAgentConfig{
				Model:   "existing-model",
				APIType: "openai",
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"openai": map[string]interface{}{
						"available_models": []interface{}{
							map[string]interface{}{
								"name":              "existing-model",
								"display_name":      "existing-model",
								"max_tokens":        131072,
								"max_output_tokens": 16384,
							},
						},
					},
				},
			},
			wantModel:    "existing-model",
			wantProvider: "openai",
		},
		{
			name: "uses 200K fallback when MaxTokens is 0",
			codeAgentConfig: &CodeAgentConfig{
				Model:     "nebius/some-model",
				APIType:   "openai",
				MaxTokens: 0,
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"openai": map[string]interface{}{},
				},
			},
			wantModel:    "nebius/some-model",
			wantProvider: "openai",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &SettingsDaemon{
				codeAgentConfig: tt.codeAgentConfig,
				helixSettings:   tt.helixSettings,
			}

			d.injectAvailableModels()

			if tt.wantSkipped {
				// Anthropic models should NOT be injected — verify no available_models added
				languageModels, ok := d.helixSettings["language_models"].(map[string]interface{})
				if ok {
					if providerConfig, ok := languageModels["anthropic"].(map[string]interface{}); ok {
						availableModels, exists := providerConfig["available_models"]
						if exists {
							assert.Nil(t, availableModels, "available_models should not be set for anthropic provider")
						}
					}
				}
				return
			}

			// Expected no changes
			if tt.wantModel == "" || tt.wantProvider == "" {
				return
			}

			languageModels := d.helixSettings["language_models"].(map[string]interface{})
			providerConfig := languageModels[tt.wantProvider].(map[string]interface{})
			availableModels := providerConfig["available_models"].([]interface{})

			// Helper to get model name from either map or struct
			getModelName := func(m interface{}) string {
				if model, ok := m.(map[string]interface{}); ok {
					return model["name"].(string)
				}
				if model, ok := m.(AvailableModel); ok {
					return model.Name
				}
				return ""
			}

			// Helper to get max_tokens from either map or struct
			getMaxTokens := func(m interface{}) int {
				if model, ok := m.(map[string]interface{}); ok {
					if v, ok := model["max_tokens"].(int); ok {
						return v
					}
				}
				if model, ok := m.(AvailableModel); ok {
					return model.MaxTokens
				}
				return 0
			}

			// Find the model
			found := false
			for _, m := range availableModels {
				if getModelName(m) == tt.wantModel {
					found = true
					// Check fields based on type
					if model, ok := m.(AvailableModel); ok {
						assert.Equal(t, tt.wantModel, model.DisplayName, "display_name should match model name")
						assert.NotZero(t, model.MaxTokens, "max_tokens should be set")
						assert.Equal(t, tt.wantEffort, model.ReasoningEffort, "reasoning_effort should match")
					} else if model, ok := m.(map[string]interface{}); ok {
						assert.Equal(t, tt.wantModel, model["display_name"], "display_name should match model name")
						assert.NotNil(t, model["max_tokens"], "max_tokens should be set")
					}
					break
				}
			}
			assert.True(t, found, "model %s should be in available_models", tt.wantModel)

			// For the duplicate test, ensure there's only one entry
			if tt.name == "does not duplicate model if already exists" {
				count := 0
				for _, m := range availableModels {
					if getModelName(m) == tt.wantModel {
						count++
					}
				}
				assert.Equal(t, 1, count, "should not duplicate model")
			}

			// For the 200K fallback test, verify the default is applied
			if tt.name == "uses 200K fallback when MaxTokens is 0" {
				for _, m := range availableModels {
					if getModelName(m) == tt.wantModel {
						maxTokens := getMaxTokens(m)
						assert.Equal(t, 200000, maxTokens, "should use 200K fallback when MaxTokens is 0")
						break
					}
				}
			}
		})
	}
}

func TestInjectAvailableModelsPropagatesProviderContextWindowToZed(t *testing.T) {
	const providerContextWindow = 258_400
	d := &SettingsDaemon{
		codeAgentConfig: &CodeAgentConfig{
			Model:     "deepseek/deepseek-v4-flash",
			APIType:   "openai",
			Runtime:   "zed_agent",
			MaxTokens: providerContextWindow,
		},
		helixSettings: map[string]interface{}{
			"language_models": map[string]interface{}{
				"openai": map[string]interface{}{},
			},
		},
	}

	d.injectAvailableModels()

	languageModels := d.helixSettings["language_models"].(map[string]interface{})
	provider := languageModels["openai"].(map[string]interface{})
	models := provider["available_models"].([]interface{})
	require.Len(t, models, 1)
	model := models[0].(AvailableModel)
	assert.Equal(t, providerContextWindow, model.MaxTokens)
}

// TestMergeAgentBlock_HelixManagedFieldsProtected verifies that the daemon's
// client-side merge drops user-side overrides for helix-managed agent fields.
func TestMergeAgentBlock_HelixManagedFieldsProtected(t *testing.T) {
	helixAgent := map[string]interface{}{
		"default_model":          map[string]interface{}{"provider": "openai", "model": "numpty/openai/gpt-oss-120b"},
		"inline_assistant_model": map[string]interface{}{"provider": "openai", "model": "numpty/openai/gpt-oss-120b"},
		"commit_message_model":   map[string]interface{}{"provider": "openai", "model": "numpty/openai/gpt-oss-120b"},
		"thread_summary_model":   map[string]interface{}{"provider": "openai", "model": "numpty/openai/gpt-oss-120b"},
		"auto_open_panel":        true,
		"show_onboarding":        false,
		"sandbox_permissions":    map[string]interface{}{"allow_unsandboxed": true},
	}

	t.Run("user override of default_model is dropped", func(t *testing.T) {
		userAgent := map[string]interface{}{
			"default_model": map[string]interface{}{
				"provider":        "anthropic",
				"model":           "claude-sonnet-4-6-latest",
				"effort":          "high",
				"enable_thinking": true,
			},
		}
		merged := mergeAgentBlock(helixAgent, userAgent).(map[string]interface{})

		dm := merged["default_model"].(map[string]interface{})
		assert.Equal(t, "openai", dm["provider"])
		assert.Equal(t, "numpty/openai/gpt-oss-120b", dm["model"])
		assert.NotContains(t, dm, "effort")
		assert.NotContains(t, dm, "enable_thinking")
	})

	t.Run("all four model fields are protected", func(t *testing.T) {
		userAgent := map[string]interface{}{
			"default_model":          map[string]interface{}{"provider": "anthropic", "model": "claude"},
			"inline_assistant_model": map[string]interface{}{"provider": "anthropic", "model": "claude"},
			"commit_message_model":   map[string]interface{}{"provider": "anthropic", "model": "claude"},
			"thread_summary_model":   map[string]interface{}{"provider": "anthropic", "model": "claude"},
		}
		merged := mergeAgentBlock(helixAgent, userAgent).(map[string]interface{})

		for _, field := range []string{"default_model", "inline_assistant_model", "commit_message_model", "thread_summary_model"} {
			dm := merged[field].(map[string]interface{})
			assert.Equal(t, "openai", dm["provider"], "%s.provider", field)
			assert.Equal(t, "numpty/openai/gpt-oss-120b", dm["model"], "%s.model", field)
		}
	})

	t.Run("sandbox permissions are protected", func(t *testing.T) {
		userAgent := map[string]interface{}{
			"sandbox_permissions": map[string]interface{}{"allow_unsandboxed": false},
		}
		merged := mergeAgentBlock(helixAgent, userAgent).(map[string]interface{})

		permissions := merged["sandbox_permissions"].(map[string]interface{})
		assert.Equal(t, true, permissions["allow_unsandboxed"])
	})

	t.Run("non-model agent fields can still be user-overridden", func(t *testing.T) {
		userAgent := map[string]interface{}{
			"default_model":              map[string]interface{}{"provider": "anthropic", "model": "claude"},
			"play_sound_when_agent_done": true,
			"button":                     false,
		}
		merged := mergeAgentBlock(helixAgent, userAgent).(map[string]interface{})

		assert.Equal(t, "numpty/openai/gpt-oss-120b", merged["default_model"].(map[string]interface{})["model"])
		assert.Equal(t, true, merged["play_sound_when_agent_done"])
		assert.Equal(t, false, merged["button"])
	})

	t.Run("non-object user agent keeps helix verbatim", func(t *testing.T) {
		merged := mergeAgentBlock(helixAgent, "not-an-object")
		assert.Equal(t, helixAgent, merged)
	})
}

func TestInjectAgentPermissions(t *testing.T) {
	settings := map[string]interface{}{
		"agent": map[string]interface{}{
			"sandbox_permissions": map[string]interface{}{"allow_unsandboxed": false},
		},
	}

	injectAgentPermissions(settings)

	agent := settings["agent"].(map[string]interface{})
	assert.Equal(t, map[string]interface{}{"allow_unsandboxed": true}, agent["sandbox_permissions"])
	assert.Equal(t, map[string]interface{}{"default": "allow"}, agent["tool_permissions"])
}

func TestRewriteHelixConfigURLs(t *testing.T) {
	d := &SettingsDaemon{apiURL: "http://helix-api.internal:18080"}
	config := &helixConfigResponse{
		CodeAgentConfig: &CodeAgentConfig{BaseURL: "http://api:8080/v1"},
		LanguageModels: map[string]interface{}{
			"openai":    map[string]interface{}{"api_url": "http://api:8080/v1"},
			"anthropic": map[string]interface{}{"api_url": "https://control.example.test"},
		},
		ContextServers: map[string]interface{}{
			"helix-session": map[string]interface{}{
				"url": "http://api:8080/api/v1/mcp/session?session_id=ses_test",
			},
			"helix-native": map[string]interface{}{
				"url": "http://api:8080/api/v1/mcp",
			},
			"organization-tools": map[string]interface{}{
				"url": "https://control.example.test/api/v1/mcp/external/org-tools/sse",
			},
			"user-tools": map[string]interface{}{
				"url": "https://control.example.test/api/v1/mcp/external/user-tools/sse",
			},
			"custom": map[string]interface{}{
				"url": "https://mcp.example.test/sse",
			},
		},
	}

	d.rewriteHelixConfigURLs(config)

	assert.Equal(t, "http://helix-api.internal:18080/v1", config.CodeAgentConfig.BaseURL)
	assert.Equal(t, "http://helix-api.internal:18080/v1",
		config.LanguageModels["openai"].(map[string]interface{})["api_url"])
	assert.Equal(t, "http://helix-api.internal:18080",
		config.LanguageModels["anthropic"].(map[string]interface{})["api_url"])
	assert.Equal(t, "http://helix-api.internal:18080/api/v1/mcp/session?session_id=ses_test",
		config.ContextServers["helix-session"].(map[string]interface{})["url"])
	assert.Equal(t, "http://helix-api.internal:18080/api/v1/mcp",
		config.ContextServers["helix-native"].(map[string]interface{})["url"])
	assert.Equal(t, "http://helix-api.internal:18080/api/v1/mcp/external/org-tools/sse",
		config.ContextServers["organization-tools"].(map[string]interface{})["url"])
	assert.Equal(t, "http://helix-api.internal:18080/api/v1/mcp/external/user-tools/sse",
		config.ContextServers["user-tools"].(map[string]interface{})["url"])
	assert.Equal(t, "https://mcp.example.test/sse",
		config.ContextServers["custom"].(map[string]interface{})["url"])
}

func TestIsHelixMCPURL(t *testing.T) {
	assert.True(t, isHelixMCPURL("http://api:8080/api/v1/mcp"))
	assert.True(t, isHelixMCPURL("https://example.test/api/v1/mcp/external/user-server/sse"))
	assert.False(t, isHelixMCPURL("https://example.test/mcp"))
	assert.False(t, isHelixMCPURL("not a url"))
}

// TestExtractUserOverrides_AgentDiffSkipsManagedFields verifies that the daemon
// does not upload changes to helix-managed agent fields.
func TestExtractUserOverrides_AgentDiffSkipsManagedFields(t *testing.T) {
	helix := map[string]interface{}{
		"agent": map[string]interface{}{
			"default_model":       map[string]interface{}{"provider": "openai", "model": "numpty/openai/gpt-oss-120b"},
			"auto_open_panel":     true,
			"sandbox_permissions": map[string]interface{}{"allow_unsandboxed": true},
		},
	}

	t.Run("does not upload claude default_model", func(t *testing.T) {
		current := map[string]interface{}{
			"agent": map[string]interface{}{
				"default_model":   map[string]interface{}{"provider": "anthropic", "model": "claude-sonnet-4-6-latest"},
				"auto_open_panel": true,
			},
		}
		got := extractUserOverrides(current, helix)
		assert.NotContains(t, got, "agent")
	})

	t.Run("does not upload sandbox permission changes", func(t *testing.T) {
		current := map[string]interface{}{
			"agent": map[string]interface{}{
				"default_model":       map[string]interface{}{"provider": "openai", "model": "numpty/openai/gpt-oss-120b"},
				"auto_open_panel":     true,
				"sandbox_permissions": map[string]interface{}{"allow_unsandboxed": false},
			},
		}
		got := extractUserOverrides(current, helix)
		assert.NotContains(t, got, "agent")
	})

	t.Run("uploads non-model agent diffs only", func(t *testing.T) {
		current := map[string]interface{}{
			"agent": map[string]interface{}{
				"default_model":              map[string]interface{}{"provider": "anthropic", "model": "claude"},
				"auto_open_panel":            true,
				"play_sound_when_agent_done": true,
			},
		}
		got := extractUserOverrides(current, helix)
		agent := got["agent"].(map[string]interface{})
		assert.Equal(t, true, agent["play_sound_when_agent_done"])
		assert.NotContains(t, agent, "default_model")
		assert.NotContains(t, agent, "auto_open_panel")
	})
}

// TestMergeSettings_HelixOwnedContextServersWin is the regression test for the
// stale-MCP-config bug documented in
// helix/design/2026-05-13-mcp-cache-contention-and-duplicate-claude-spawn.md.
//
// Sequence pre-fix:
//  1. Old API code generated `chrome-devtools` config with `command: "npx"` and
//     wrote it to disk via the daemon.
//  2. PR #2418 changed `chrome-devtools` to use `/usr/bin/chrome-devtools-mcp`.
//  3. On the next daemon poll the API returned the NEW config — but the
//     deep-merge in `mergeSettings` treated the on-disk OLD entry as a
//     "user override" and let it win, pinning the broken `npx` config
//     forever and producing 180s `chrome-devtools context server failed
//     to start: Context server request timeout` errors.
//
// To verify regression power: comment out the
// `if HELIX_OWNED_CONTEXT_SERVERS[name] { continue }` guard in the
// `mergeSettings` deep-merge of `context_servers` and re-run; the
// "force-overwrite" sub-tests below will fail because the user's stale
// `npx`-based entry will win.
func TestMergeSettings_HelixOwnedContextServersWin(t *testing.T) {
	d := &SettingsDaemon{}

	// Helix base — what zed_config.go produces post-fix
	helix := map[string]interface{}{
		"context_servers": map[string]interface{}{
			"chrome-devtools": map[string]interface{}{
				"command": "/usr/bin/chrome-devtools-mcp",
				"args":    []interface{}{"--viewport", "1280x800"},
			},
			"helix-session": map[string]interface{}{
				"url":     "http://api:8080/api/v1/mcp/session?session_id=ses_new",
				"headers": map[string]interface{}{"Authorization": "Bearer fresh"},
			},
			"helix-desktop": map[string]interface{}{
				"url": "http://api:8080/api/v1/mcp/desktop?session_id=ses_new",
			},
		},
	}

	t.Run("force-overwrite chrome-devtools when user has stale npx version", func(t *testing.T) {
		user := map[string]interface{}{
			"context_servers": map[string]interface{}{
				"chrome-devtools": map[string]interface{}{
					// THIS is the bug — the persisted on-disk entry from
					// before PR #2418. Without the guard, this wins.
					"command": "npx",
					"args":    []interface{}{"chrome-devtools-mcp@latest"},
				},
			},
		}
		merged := d.mergeSettings(helix, user)
		got := merged["context_servers"].(map[string]interface{})["chrome-devtools"].(map[string]interface{})
		assert.Equal(t, "/usr/bin/chrome-devtools-mcp", got["command"],
			"chrome-devtools must use Helix's hardcoded path, not the user's stale npx entry")
	})

	t.Run("force-overwrite helix-session when user has stale session_id", func(t *testing.T) {
		user := map[string]interface{}{
			"context_servers": map[string]interface{}{
				"helix-session": map[string]interface{}{
					"url":     "http://api:8080/api/v1/mcp/session?session_id=ses_OLD",
					"headers": map[string]interface{}{"Authorization": "Bearer STALE"},
				},
			},
		}
		merged := d.mergeSettings(helix, user)
		got := merged["context_servers"].(map[string]interface{})["helix-session"].(map[string]interface{})
		assert.Equal(t, "http://api:8080/api/v1/mcp/session?session_id=ses_new", got["url"])
		assert.Equal(t, "Bearer fresh", got["headers"].(map[string]interface{})["Authorization"])
	})

	t.Run("user-configured MCP (e.g. drone-ci) still wins", func(t *testing.T) {
		// drone-ci is a user/project-configured MCP, NOT in
		// HELIX_OWNED_CONTEXT_SERVERS. Users editing their on-disk
		// settings.json to customize it must round-trip.
		user := map[string]interface{}{
			"context_servers": map[string]interface{}{
				"drone-ci": map[string]interface{}{
					"command": "drone-ci-mcp",
					"args":    []interface{}{},
					"env":     map[string]interface{}{"DRONE_ACCESS_TOKEN": "user-token"},
				},
			},
		}
		merged := d.mergeSettings(helix, user)
		got := merged["context_servers"].(map[string]interface{})["drone-ci"].(map[string]interface{})
		assert.Equal(t, "drone-ci-mcp", got["command"])
		assert.Equal(t, "user-token", got["env"].(map[string]interface{})["DRONE_ACCESS_TOKEN"])
	})

	t.Run("strips helix-owned names even when helix has no servers", func(t *testing.T) {
		// Defensive: if Helix temporarily emits no context_servers (e.g.
		// during a transient API state) we shouldn't accidentally
		// resurrect a user's stale chrome-devtools from disk.
		emptyHelix := map[string]interface{}{}
		user := map[string]interface{}{
			"context_servers": map[string]interface{}{
				"chrome-devtools": map[string]interface{}{
					"command": "npx",
					"args":    []interface{}{"chrome-devtools-mcp@latest"},
				},
				"my-custom-mcp": map[string]interface{}{
					"command": "my-custom-mcp",
				},
			},
		}
		merged := d.mergeSettings(emptyHelix, user)
		cs := merged["context_servers"].(map[string]interface{})
		assert.NotContains(t, cs, "chrome-devtools",
			"helix-owned name must be stripped even when helix has no servers")
		assert.Contains(t, cs, "my-custom-mcp",
			"non-helix-owned user MCP must survive")
	})
}

// TestExtractUserOverrides_SkipsHelixOwnedContextServers verifies the round-trip
// half of the fix: extractUserOverrides must NOT capture helix-owned names as
// user overrides. Otherwise a stale on-disk chrome-devtools entry is uploaded
// to the API, the API treats it as the canonical user customization, the next
// sync re-writes it to disk — and Helix's force-overwrite is permanently
// nullified one round-trip later.
func TestExtractUserOverrides_SkipsHelixOwnedContextServers(t *testing.T) {
	helix := map[string]interface{}{
		"context_servers": map[string]interface{}{
			"chrome-devtools": map[string]interface{}{
				"command": "/usr/bin/chrome-devtools-mcp",
			},
		},
	}

	t.Run("does not upload stale chrome-devtools as user override", func(t *testing.T) {
		current := map[string]interface{}{
			"context_servers": map[string]interface{}{
				"chrome-devtools": map[string]interface{}{
					"command": "npx",
					"args":    []interface{}{"chrome-devtools-mcp@latest"},
				},
			},
		}
		got := extractUserOverrides(current, helix)
		assert.NotContains(t, got, "context_servers",
			"stale on-disk helix-owned entry must not be captured as user override")
	})

	t.Run("does upload non-helix user MCP overrides", func(t *testing.T) {
		current := map[string]interface{}{
			"context_servers": map[string]interface{}{
				"chrome-devtools": map[string]interface{}{
					"command": "npx",
					"args":    []interface{}{"chrome-devtools-mcp@latest"},
				},
				"my-custom-mcp": map[string]interface{}{
					"command": "/opt/my-custom-mcp/run",
				},
			},
		}
		got := extractUserOverrides(current, helix)
		cs, ok := got["context_servers"].(map[string]interface{})
		assert.True(t, ok, "user override for my-custom-mcp must be captured")
		assert.NotContains(t, cs, "chrome-devtools")
		assert.Contains(t, cs, "my-custom-mcp")
	})
}

// TestQwenCodeAgentServerHasYoloDefaultMode pins the fix for the
// "qwen-code agents prompt for permission on every edit" bug. Without
// default_mode: "yolo" on the qwen entry, qwen-code's Session.setMode
// defaults to ApprovalMode.DEFAULT and every tool call round-trips a
// session/request_permission to Zed — which nobody clicks in a headless
// spec-task sandbox, so the agent stalls. claude_code has the equivalent
// default_mode: "bypassPermissions" entry; this test keeps the two in
// step. If you remove the default_mode field, this test fails.
func TestQwenCodeAgentServerHasYoloDefaultMode(t *testing.T) {
	oldQwenSettingsPath := QwenSettingsPath
	QwenSettingsPath = filepath.Join(t.TempDir(), "qwen", "settings.json")
	t.Cleanup(func() { QwenSettingsPath = oldQwenSettingsPath })

	d := &SettingsDaemon{
		codeAgentConfig: &CodeAgentConfig{
			Runtime:         "qwen_code",
			BaseURL:         "http://outer-api:8080/v1",
			Model:           "node-6/qwen3.8-27b",
			ReasoningEffort: "xhigh",
		},
		userAPIKey: "hl-test-key",
	}

	cfg := d.generateAgentServerConfig()
	qwen, ok := cfg["qwen"].(map[string]interface{})
	assert.True(t, ok, "agent_servers must contain a qwen entry for qwen_code runtime")

	mode, ok := qwen["default_mode"].(string)
	assert.True(t, ok, "qwen entry must have a default_mode string")
	assert.Equal(t, "yolo", mode,
		"qwen default_mode must be \"yolo\" so qwen-code auto-approves tool calls (mirrors claude_code bypassPermissions)")

	// --yolo must also be on the command line: default_mode alone relies on the
	// IDE issuing session/set_mode, which the pinned Zed builds don't do for
	// custom agent servers. --yolo guarantees YOLO at qwen startup regardless.
	args, ok := qwen["args"].([]string)
	assert.True(t, ok, "qwen entry must have args")
	assert.Contains(t, args, "--yolo",
		"qwen args must include --yolo so the ACP session starts in YOLO mode without depending on the IDE")
	assert.Contains(t, args, "--acp")
	assert.NotContains(t, args, "--experimental-acp")

	env, ok := qwen["env"].(map[string]interface{})
	assert.True(t, ok, "qwen entry must have env")
	assert.Equal(t, "/home/retro/work/.qwen-state", env["QWEN_HOME"])
	assert.Equal(t, "/home/retro/work/.qwen-state", env["QWEN_RUNTIME_DIR"])
	assert.Equal(t, "false", env["QWEN_TELEMETRY_ENABLED"])
	assert.Equal(t, "false", env["QWEN_USAGE_STATISTICS_ENABLED"])
	assert.NotContains(t, env, "QWEN_DATA_DIR")

	defaults, ok := qwen["default_config_options"].(map[string]string)
	assert.True(t, ok, "qwen entry must have default ACP config options")
	assert.Equal(t, "xhigh", defaults["reasoning_effort"])

	data, err := os.ReadFile(QwenSettingsPath)
	require.NoError(t, err)
	var settings map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &settings))
	model := settings["model"].(map[string]interface{})
	generationConfig := model["generationConfig"].(map[string]interface{})
	extraBody := generationConfig["extra_body"].(map[string]interface{})
	assert.Equal(t, "xhigh", extraBody["reasoning_effort"])
}

func TestEnsureQwenSettingsPreservesUserConfigAndClearsEffort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "qwen", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(`{
  "general": {"vimMode": true},
  "model": {"generationConfig": {"timeout": 60000, "extra_body": {"custom": true}}}
}`), 0644))

	require.NoError(t, ensureQwenSettings(path, "medium"))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var settings map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &settings))
	assert.Equal(t, true, settings["general"].(map[string]interface{})["vimMode"])
	model := settings["model"].(map[string]interface{})
	generationConfig := model["generationConfig"].(map[string]interface{})
	assert.Equal(t, float64(60000), generationConfig["timeout"])
	extraBody := generationConfig["extra_body"].(map[string]interface{})
	assert.Equal(t, true, extraBody["custom"])
	assert.Equal(t, "medium", extraBody["reasoning_effort"])

	require.NoError(t, ensureQwenSettings(path, ""))
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &settings))
	model = settings["model"].(map[string]interface{})
	generationConfig = model["generationConfig"].(map[string]interface{})
	extraBody = generationConfig["extra_body"].(map[string]interface{})
	assert.NotContains(t, extraBody, "reasoning_effort")
	assert.Equal(t, true, extraBody["custom"])
}

func TestEnsureCodexConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codex", "config.toml")
	existing := []byte("model = \"gpt-5.6-sol\"\nopenai_base_url = \"https://user-proxy.example/v1\"\n\n[model_providers.user_proxy]\nname = \"User proxy\"\nbase_url = \"https://user-proxy.example/v1\"\nenv_key = \"USER_PROXY_KEY\"\nwire_api = \"responses\"\n\n[projects.\"/workspace\"]\ntrust_level = \"trusted\"\n")
	assert.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	assert.NoError(t, os.WriteFile(path, existing, 0644))

	assert.NoError(t, ensureCodexConfig(path, "http://api:8080/v1", "gpt-5.6-terra"))
	data, err := os.ReadFile(path)
	assert.NoError(t, err)
	var config map[string]interface{}
	assert.NoError(t, toml.Unmarshal(data, &config))
	assert.Equal(t, "never", config["approval_policy"])
	assert.Equal(t, "danger-full-access", config["sandbox_mode"])
	assert.Equal(t, "gpt-5.6-terra", config["model"])
	assert.Equal(t, "https://user-proxy.example/v1", config["openai_base_url"])
	assert.Equal(t, "helix", config["model_provider"])
	providers, ok := config["model_providers"].(map[string]interface{})
	assert.True(t, ok)
	assert.Contains(t, providers, "user_proxy")
	helixProvider, ok := providers["helix"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "http://api:8080/v1", helixProvider["base_url"])
	assert.Equal(t, "OPENAI_API_KEY", helixProvider["env_key"])
	assert.Equal(t, "responses", helixProvider["wire_api"])
	assert.Equal(t, false, helixProvider["supports_websockets"])
	projects, ok := config["projects"].(map[string]interface{})
	assert.True(t, ok)
	assert.Contains(t, projects, "/workspace")

	assert.NoError(t, ensureCodexConfig(path, "", ""))
	data, err = os.ReadFile(path)
	assert.NoError(t, err)
	config = map[string]interface{}{}
	assert.NoError(t, toml.Unmarshal(data, &config))
	assert.Equal(t, "https://user-proxy.example/v1", config["openai_base_url"])
	assert.NotContains(t, config, "model")
	assert.NotContains(t, config, "model_provider")
	providers, ok = config["model_providers"].(map[string]interface{})
	assert.True(t, ok)
	assert.Contains(t, providers, "user_proxy")
	assert.NotContains(t, providers, "helix")
}

func TestCodexAgentServerUsesFullAccess(t *testing.T) {
	originalPath := CodexConfigPath
	CodexConfigPath = filepath.Join(t.TempDir(), "config.toml")
	t.Cleanup(func() { CodexConfigPath = originalPath })

	d := &SettingsDaemon{codeAgentConfig: &CodeAgentConfig{Runtime: "codex_cli", BaseURL: "http://api/v1", Model: "gpt-5.6-terra"}}
	cfg := d.generateAgentServerConfig()
	codex, ok := cfg["codex-acp"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "agent-full-access", codex["default_mode"])
	env, ok := codex["env"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "agent-full-access", env["INITIAL_AGENT_MODE"])
	assert.NotContains(t, env, "OPENAI_BASE_URL")

	data, err := os.ReadFile(CodexConfigPath)
	assert.NoError(t, err)
	var persisted map[string]interface{}
	assert.NoError(t, toml.Unmarshal(data, &persisted))
	assert.Equal(t, "never", persisted["approval_policy"])
	assert.Equal(t, "danger-full-access", persisted["sandbox_mode"])
	assert.Equal(t, "gpt-5.6-terra", persisted["model"])
	assert.Equal(t, "helix", persisted["model_provider"])
	providers, ok := persisted["model_providers"].(map[string]interface{})
	assert.True(t, ok)
	helixProvider, ok := providers["helix"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "http://api/v1", helixProvider["base_url"])
	assert.Equal(t, false, helixProvider["supports_websockets"])
}

func TestClaudeAgentServerUsesConfiguredReasoningEffort(t *testing.T) {
	d := &SettingsDaemon{codeAgentConfig: &CodeAgentConfig{
		Runtime:         "claude_code",
		Model:           "claude-opus-4-6",
		BaseURL:         "http://api",
		ReasoningEffort: "max",
	}}

	cfg := d.generateAgentServerConfig()
	claude, ok := cfg["claude-acp"].(map[string]interface{})
	assert.True(t, ok)
	env, ok := claude["env"].(map[string]string)
	assert.True(t, ok)
	assert.Equal(t, "max", env["CLAUDE_CODE_EFFORT_LEVEL"])
}

func TestCodexAgentServerUsesConfiguredReasoningEffort(t *testing.T) {
	originalPath := CodexConfigPath
	CodexConfigPath = filepath.Join(t.TempDir(), "config.toml")
	t.Cleanup(func() { CodexConfigPath = originalPath })

	d := &SettingsDaemon{codeAgentConfig: &CodeAgentConfig{
		Runtime:         "codex_cli",
		BaseURL:         "http://api/v1",
		ReasoningEffort: "ultra",
		ServiceTier:     "fast",
	}}

	cfg := d.generateAgentServerConfig()
	codex, ok := cfg["codex-acp"].(map[string]interface{})
	assert.True(t, ok)
	env, ok := codex["env"].(map[string]interface{})
	assert.True(t, ok)
	codexConfig, ok := env["CODEX_CONFIG"].(string)
	assert.True(t, ok)
	assert.JSONEq(t, `{"model_reasoning_effort":"ultra","service_tier":"fast"}`, codexConfig)
}

// TestComputeEffectiveTheme exercises every branch of the helper that decides
// whether the daemon should write the API-supplied theme or preserve the user's
// on-disk Zed-UI choice. Covers the structured-theme case (the 002056 hypothesis
// H1) explicitly — a Zed UI ToggleMode can leave settings.json with
//
//	"theme": {"mode":"system","light":"One Light","dark":"Ayu Dark"}
//
// which must be replaced with the bare string the API chose; otherwise Zed's
// in-memory Dynamic{mode:System} state would keep resolving theme via the OS
// appearance and the user's explicit Helix toggle would never apply.
func TestComputeEffectiveTheme(t *testing.T) {
	tests := []struct {
		name           string
		apiTheme       string
		writeFile      bool   // create settings.json
		fileContent    string // contents (only if writeFile)
		wantResult     string
		wantBranch     string
		wantOnDiskHint string // substring expected in the onDiskRepr log field
	}{
		{
			name:           "empty api theme skips assignment",
			apiTheme:       "",
			writeFile:      true,
			fileContent:    `{"theme":"Ayu Dark"}`,
			wantResult:     "",
			wantBranch:     "no_api_theme",
			wantOnDiskHint: "not_read",
		},
		{
			name:           "missing settings file writes api theme",
			apiTheme:       "Ayu Dark",
			writeFile:      false,
			wantResult:     "Ayu Dark",
			wantBranch:     "no_existing_file",
			wantOnDiskHint: "missing",
		},
		{
			name:           "unparseable settings file writes api theme",
			apiTheme:       "Ayu Dark",
			writeFile:      true,
			fileContent:    "{not valid json",
			wantResult:     "Ayu Dark",
			wantBranch:     "unparseable",
			wantOnDiskHint: "unparseable",
		},
		{
			name:           "no theme key writes api theme",
			apiTheme:       "Ayu Dark",
			writeFile:      true,
			fileContent:    `{"other":"value"}`,
			wantResult:     "Ayu Dark",
			wantBranch:     "no_theme_key",
			wantOnDiskHint: "absent",
		},
		{
			name:           "structured theme is replaced with api string",
			apiTheme:       "Ayu Dark",
			writeFile:      true,
			fileContent:    `{"theme":{"mode":"system","light":"One Light","dark":"Ayu Dark"}}`,
			wantResult:     "Ayu Dark",
			wantBranch:     "structured_replace",
			wantOnDiskHint: "mode",
		},
		{
			name:           "empty string theme writes api theme",
			apiTheme:       "Ayu Dark",
			writeFile:      true,
			fileContent:    `{"theme":""}`,
			wantResult:     "Ayu Dark",
			wantBranch:     "empty_string",
			wantOnDiskHint: `""`,
		},
		{
			name:           "managed theme is overwritten on toggle",
			apiTheme:       "Ayu Dark",
			writeFile:      true,
			fileContent:    `{"theme":"One Light"}`,
			wantResult:     "Ayu Dark",
			wantBranch:     "managed_overwrite",
			wantOnDiskHint: "One Light",
		},
		{
			name:           "managed theme matching api still goes through managed branch",
			apiTheme:       "Ayu Dark",
			writeFile:      true,
			fileContent:    `{"theme":"Ayu Dark"}`,
			wantResult:     "Ayu Dark",
			wantBranch:     "managed_overwrite",
			wantOnDiskHint: "Ayu Dark",
		},
		{
			name:           "custom theme is preserved",
			apiTheme:       "Ayu Dark",
			writeFile:      true,
			fileContent:    `{"theme":"Solarized Dark"}`,
			wantResult:     "Solarized Dark",
			wantBranch:     "preserve_custom",
			wantOnDiskHint: "Solarized Dark",
		},
	}

	origSettingsPath := SettingsPath
	t.Cleanup(func() { SettingsPath = origSettingsPath })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			SettingsPath = filepath.Join(dir, "settings.json")
			if tt.writeFile {
				err := os.WriteFile(SettingsPath, []byte(tt.fileContent), 0644)
				assert.NoError(t, err)
			}

			d := &SettingsDaemon{}
			gotResult, gotBranch, gotOnDiskRepr := d.computeEffectiveTheme(tt.apiTheme)
			assert.Equal(t, tt.wantResult, gotResult, "result value")
			assert.Equal(t, tt.wantBranch, gotBranch, "branch label")
			assert.Contains(t, gotOnDiskRepr, tt.wantOnDiskHint, "on-disk repr")
		})
	}
}

// TestWriteSettingsPreservesInode is the regression test for the dark<->light
// "doesn't change back" bug. Zed watches settings.json by inode; the daemon used
// to write via temp-file + rename, which replaces the inode on every write and
// kills Zed's inotify watch after the first change. writeSettings must now keep
// the inode stable across repeated writes so Zed keeps reloading on every toggle.
func TestWriteSettingsPreservesInode(t *testing.T) {
	origSettingsPath := SettingsPath
	t.Cleanup(func() { SettingsPath = origSettingsPath })

	dir := t.TempDir()
	SettingsPath = filepath.Join(dir, "settings.json")

	inodeOf := func(path string) uint64 {
		info, err := os.Stat(path)
		assert.NoError(t, err)
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			t.Fatalf("Sys() is not *syscall.Stat_t")
		}
		return st.Ino
	}

	d := &SettingsDaemon{}

	// First write creates the file.
	assert.NoError(t, d.writeSettings(map[string]interface{}{"theme": "Ayu Dark"}))
	firstInode := inodeOf(SettingsPath)

	// Repeated writes (simulating dark<->light<->dark toggles) must keep the SAME
	// inode and write the correct contents each time.
	for _, theme := range []string{"One Light", "Ayu Dark", "One Light"} {
		assert.NoError(t, d.writeSettings(map[string]interface{}{"theme": theme}))

		assert.Equal(t, firstInode, inodeOf(SettingsPath),
			"settings.json inode must stay stable across writes (theme=%s)", theme)

		data, err := os.ReadFile(SettingsPath)
		assert.NoError(t, err)
		var got map[string]interface{}
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, theme, got["theme"], "written theme should be readable")
	}
}
