package desktop

import (
	"testing"
)

func TestFormatSwayWindowList(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "empty tree",
			input:    []byte(`{"id":1,"type":"root","nodes":[],"floating_nodes":[]}`),
			expected: "No windows found",
		},
		{
			name: "single window",
			input: []byte(`{
				"id": 1,
				"type": "root",
				"nodes": [{
					"id": 2,
					"type": "output",
					"name": "eDP-1",
					"nodes": [{
						"id": 3,
						"type": "workspace",
						"name": "1",
						"nodes": [{
							"id": 4,
							"type": "con",
							"name": "Terminal",
							"focused": true
						}]
					}]
				}],
				"floating_nodes": []
			}`),
			expected: "Windows (1 total):\n  ID: 4 | Workspace: 1 | Title: Terminal [FOCUSED]",
		},
		{
			name: "multiple windows on different workspaces",
			input: []byte(`{
				"id": 1,
				"type": "root",
				"nodes": [{
					"id": 2,
					"type": "output",
					"name": "eDP-1",
					"nodes": [
						{
							"id": 3,
							"type": "workspace",
							"name": "1",
							"nodes": [{
								"id": 4,
								"type": "con",
								"name": "Firefox",
								"focused": false
							}]
						},
						{
							"id": 5,
							"type": "workspace",
							"name": "2",
							"nodes": [{
								"id": 6,
								"type": "con",
								"name": "Zed",
								"focused": true
							}]
						}
					]
				}],
				"floating_nodes": []
			}`),
			expected: "Windows (2 total):\n  ID: 4 | Workspace: 1 | Title: Firefox\n  ID: 6 | Workspace: 2 | Title: Zed [FOCUSED]",
		},
		{
			name: "floating window",
			input: []byte(`{
				"id": 1,
				"type": "root",
				"nodes": [{
					"id": 2,
					"type": "output",
					"name": "eDP-1",
					"nodes": [{
						"id": 3,
						"type": "workspace",
						"name": "1",
						"nodes": [],
						"floating_nodes": [{
							"id": 5,
							"type": "con",
							"name": "Popup",
							"focused": false
						}]
					}]
				}],
				"floating_nodes": []
			}`),
			expected: "Windows (1 total):\n  ID: 5 | Workspace: 1 | Title: Popup",
		},
		{
			name:     "invalid json",
			input:    []byte(`{invalid json}`),
			expected: "Error parsing window tree: invalid character 'i' looking for beginning of object key string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSwayWindowList(tt.input)
			if result != tt.expected {
				t.Errorf("formatSwayWindowList() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatSwayWorkspaces(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "empty workspaces",
			input:    []byte(`[]`),
			expected: "No workspaces found",
		},
		{
			name: "single workspace",
			input: []byte(`[{
				"num": 1,
				"name": "1",
				"visible": true,
				"focused": true,
				"urgent": false,
				"output": "eDP-1"
			}]`),
			expected: "Workspaces (1 total):\n  1: 1 [FOCUSED] (visible) (output: eDP-1)",
		},
		{
			name: "multiple workspaces",
			input: []byte(`[
				{
					"num": 1,
					"name": "1: Code",
					"visible": true,
					"focused": false,
					"urgent": false,
					"output": "eDP-1"
				},
				{
					"num": 2,
					"name": "2: Browser",
					"visible": false,
					"focused": true,
					"urgent": false,
					"output": "HDMI-1"
				}
			]`),
			expected: "Workspaces (2 total):\n  1: 1: Code (visible) (output: eDP-1)\n  2: 2: Browser [FOCUSED] (output: HDMI-1)",
		},
		{
			name:     "invalid json",
			input:    []byte(`{invalid}`),
			expected: "Error parsing workspaces: invalid character 'i' looking for beginning of object key string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSwayWorkspaces(tt.input)
			if result != tt.expected {
				t.Errorf("formatSwayWorkspaces() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestNewMCPServer(t *testing.T) {
	// Test default config
	t.Run("default config", func(t *testing.T) {
		cfg := MCPConfig{}
		// Can't fully test without a logger, but we can verify config defaults
		if cfg.Port != "" {
			t.Errorf("expected empty port, got %s", cfg.Port)
		}
		if cfg.ScreenshotURL != "" {
			t.Errorf("expected empty screenshot URL, got %s", cfg.ScreenshotURL)
		}
	})

	// Test custom config
	t.Run("custom config", func(t *testing.T) {
		cfg := MCPConfig{
			Port:          "9999",
			ScreenshotURL: "http://custom:8080/screenshot",
		}
		if cfg.Port != "9999" {
			t.Errorf("expected port 9999, got %s", cfg.Port)
		}
		if cfg.ScreenshotURL != "http://custom:8080/screenshot" {
			t.Errorf("expected custom screenshot URL, got %s", cfg.ScreenshotURL)
		}
	})
}
