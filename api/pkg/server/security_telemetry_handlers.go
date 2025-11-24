package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// TelemetryCounterRule represents a single iptables rule's blocking statistics
type TelemetryCounterRule struct {
	Rule           string `json:"rule"`
	PacketsBlocked int64  `json:"packets_blocked"`
	BytesBlocked   int64  `json:"bytes_blocked"`
	Agent          string `json:"agent"`
	EndpointType   string `json:"endpoint_type"`
}

// TelemetryStatus represents the overall telemetry security status
type TelemetryStatus struct {
	Timestamp              string                 `json:"timestamp"`
	TelemetryBlocked       bool                   `json:"telemetry_blocked"`
	TotalBlockedPackets    int64                  `json:"total_blocked_packets"`
	TotalBlockedBytes      int64                  `json:"total_blocked_bytes"`
	Rules                  []TelemetryCounterRule `json:"rules"`
	AgentConfigurations    map[string]AgentConfig `json:"agent_configurations"`
	PhoneHomeAttempts      []PhoneHomeAttempt     `json:"phone_home_attempts"`
	LastFirewallUpdate     string                 `json:"last_firewall_update"`
	SecurityRecommendations []string              `json:"security_recommendations"`
}

// AgentConfig represents the privacy configuration status for an AI agent
type AgentConfig struct {
	Name               string   `json:"name"`
	ConfigPath         string   `json:"config_path"`
	TelemetryDisabled  bool     `json:"telemetry_disabled"`
	AutoUpdateDisabled bool     `json:"auto_update_disabled"`
	ConfigVerified     bool     `json:"config_verified"`
	Issues             []string `json:"issues"`
}

// PhoneHomeAttempt represents a detected telemetry attempt
type PhoneHomeAttempt struct {
	Timestamp    string `json:"timestamp"`
	Agent        string `json:"agent"`
	Endpoint     string `json:"endpoint"`
	PacketCount  int64  `json:"packet_count"`
	BytesBlocked int64  `json:"bytes_blocked"`
	Severity     string `json:"severity"`
}

// Parse iptables comment to extract agent and endpoint type
func parseIPTablesComment(comment string) (agent, endpointType string) {
	parts := strings.Split(comment, ":")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return "UNKNOWN", comment
}

// getTelemetryCounters reads iptables counters and returns blocking statistics
func (s *HelixAPIServer) getTelemetryCounters() ([]TelemetryCounterRule, error) {
	cmd := exec.Command("iptables", "-L", "OUTPUT", "-n", "-v", "-x")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to read iptables: %w", err)
	}

	rules := []TelemetryCounterRule{}
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		// Look for lines with TELEMETRY_BLOCK target and comment
		if strings.Contains(line, "TELEMETRY_BLOCK") && strings.Contains(line, "/*") {
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}

			// Extract packet and byte counts
			packets := parseInt64Safe(fields[0])
			bytes := parseInt64Safe(fields[1])

			// Extract comment between /* and */
			commentStart := strings.Index(line, "/*")
			commentEnd := strings.Index(line, "*/")
			if commentStart == -1 || commentEnd == -1 {
				continue
			}
			comment := strings.TrimSpace(line[commentStart+2 : commentEnd])

			agent, endpointType := parseIPTablesComment(comment)

			rules = append(rules, TelemetryCounterRule{
				Rule:           comment,
				PacketsBlocked: packets,
				BytesBlocked:   bytes,
				Agent:          agent,
				EndpointType:   endpointType,
			})
		}
	}

	return rules, nil
}

// Helper to parse int64 safely
func parseInt64Safe(s string) int64 {
	var val int64
	fmt.Sscanf(s, "%d", &val)
	return val
}

// checkAgentConfig verifies an agent's configuration file
func checkAgentConfig(configPath string, requiredSettings map[string]interface{}) AgentConfig {
	config := AgentConfig{
		ConfigPath: configPath,
		Issues:     []string{},
	}

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		config.Issues = append(config.Issues, "Configuration file not found")
		return config
	}

	// Read and parse config
	data, err := os.ReadFile(configPath)
	if err != nil {
		config.Issues = append(config.Issues, fmt.Sprintf("Cannot read config: %v", err))
		return config
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		config.Issues = append(config.Issues, fmt.Sprintf("Invalid JSON: %v", err))
		return config
	}

	// Verify required settings
	config.ConfigVerified = true
	for key, expectedValue := range requiredSettings {
		if actualValue, ok := getNestedValue(settings, key); ok {
			if fmt.Sprintf("%v", actualValue) != fmt.Sprintf("%v", expectedValue) {
				config.ConfigVerified = false
				config.Issues = append(config.Issues, fmt.Sprintf("%s should be %v but is %v", key, expectedValue, actualValue))
			}
		} else {
			config.ConfigVerified = false
			config.Issues = append(config.Issues, fmt.Sprintf("%s is not set", key))
		}
	}

	return config
}

// getNestedValue retrieves nested JSON values using dot notation
func getNestedValue(data map[string]interface{}, key string) (interface{}, bool) {
	keys := strings.Split(key, ".")
	current := data

	for i, k := range keys {
		if i == len(keys)-1 {
			val, ok := current[k]
			return val, ok
		}
		if next, ok := current[k].(map[string]interface{}); ok {
			current = next
		} else {
			return nil, false
		}
	}
	return nil, false
}

// detectPhoneHomeAttempts analyzes counters to find recent attempts
func detectPhoneHomeAttempts(rules []TelemetryCounterRule) []PhoneHomeAttempt {
	attempts := []PhoneHomeAttempt{}
	now := time.Now().Format(time.RFC3339)

	for _, rule := range rules {
		if rule.PacketsBlocked > 0 {
			severity := "info"
			if rule.PacketsBlocked > 100 {
				severity = "warning"
			}
			if rule.PacketsBlocked > 1000 {
				severity = "critical"
			}

			attempts = append(attempts, PhoneHomeAttempt{
				Timestamp:    now,
				Agent:        rule.Agent,
				Endpoint:     rule.EndpointType,
				PacketCount:  rule.PacketsBlocked,
				BytesBlocked: rule.BytesBlocked,
				Severity:     severity,
			})
		}
	}

	return attempts
}

// getSecurityTelemetryStatus returns comprehensive security status
func (s *HelixAPIServer) getSecurityTelemetryStatus(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get iptables counters
	rules, err := s.getTelemetryCounters()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to read telemetry counters")
		rules = []TelemetryCounterRule{}
	}

	// Calculate totals
	var totalPackets, totalBytes int64
	for _, rule := range rules {
		totalPackets += rule.PacketsBlocked
		totalBytes += rule.BytesBlocked
	}

	// Check agent configurations
	qwenConfig := checkAgentConfig("/home/retro/.qwen/settings.json", map[string]interface{}{
		"privacy.usageStatisticsEnabled": false,
		"general.disableAutoUpdate":      true,
	})
	qwenConfig.Name = "Qwen Code"

	geminiConfig := checkAgentConfig("/home/retro/.gemini/settings.json", map[string]interface{}{
		"privacy.usageStatisticsEnabled": false,
		"general.disableAutoUpdate":      true,
	})
	geminiConfig.Name = "Gemini CLI"

	claudeConfig := checkAgentConfig("/home/retro/.claude/settings.json", map[string]interface{}{
		"env.CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
	})
	claudeConfig.Name = "Claude Code"

	zedConfig := checkAgentConfig("/home/retro/.config/zed/settings.json", map[string]interface{}{
		"telemetry.diagnostics": false,
		"telemetry.metrics":     false,
	})
	zedConfig.Name = "Zed Editor"

	agentConfigs := map[string]AgentConfig{
		"qwen_code":   qwenConfig,
		"gemini_cli":  geminiConfig,
		"claude_code": claudeConfig,
		"zed":         zedConfig,
	}

	// Detect phone-home attempts
	phoneHomeAttempts := detectPhoneHomeAttempts(rules)

	// Generate security recommendations
	recommendations := []string{}
	if totalPackets > 0 {
		recommendations = append(recommendations, fmt.Sprintf("⚠️ Detected %d telemetry attempts blocked by firewall - investigate agent configuration", totalPackets))
	}
	for name, config := range agentConfigs {
		if !config.ConfigVerified && len(config.Issues) == 0 {
			recommendations = append(recommendations, fmt.Sprintf("⚠️ %s configuration file not found - telemetry may be active", name))
		}
		if len(config.Issues) > 0 {
			recommendations = append(recommendations, fmt.Sprintf("⚠️ %s has configuration issues - review settings", name))
		}
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "✅ All AI agents properly configured with telemetry disabled")
		recommendations = append(recommendations, "✅ Firewall active and blocking phone-home attempts")
	}

	status := TelemetryStatus{
		Timestamp:              time.Now().Format(time.RFC3339),
		TelemetryBlocked:       len(rules) > 0,
		TotalBlockedPackets:    totalPackets,
		TotalBlockedBytes:      totalBytes,
		Rules:                  rules,
		AgentConfigurations:    agentConfigs,
		PhoneHomeAttempts:      phoneHomeAttempts,
		LastFirewallUpdate:     time.Now().Format(time.RFC3339),
		SecurityRecommendations: recommendations,
	}

	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(status); err != nil {
		log.Error().Err(err).Msg("Failed to encode telemetry status")
	}
}

// getSecurityTelemetryLogs returns recent telemetry blocking log entries
func (s *HelixAPIServer) getSecurityTelemetryLogs(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}

	logFile := "/var/log/telemetry-blocks.log"

	// Read log file
	data, err := os.ReadFile(logFile)
	if err != nil {
		response := map[string]interface{}{
			"logs":      []string{},
			"count":     0,
			"log_file":  logFile,
			"available": false,
		}
		rw.Header().Set("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(response)
		return
	}

	lines := strings.Split(string(data), "\n")

	// Get last N lines
	start := 0
	if len(lines) > limit {
		start = len(lines) - limit
	}
	recentLines := lines[start:]

	response := map[string]interface{}{
		"logs":      recentLines,
		"count":     len(recentLines),
		"log_file":  logFile,
		"available": true,
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(response)
}

// postResetTelemetryCounters resets iptables counters (admin only)
func (s *HelixAPIServer) postResetTelemetryCounters(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if user is admin
	if !user.Admin {
		http.Error(rw, "Admin access required", http.StatusForbidden)
		return
	}

	// Reset iptables counters
	cmd := exec.Command("iptables", "-Z", "OUTPUT")
	if err := cmd.Run(); err != nil {
		http.Error(rw, fmt.Sprintf("Failed to reset counters: %v", err), http.StatusInternalServerError)
		return
	}

	log.Info().Str("user", user.ID).Msg("Telemetry counters reset by admin")

	response := map[string]interface{}{
		"success":   true,
		"message":   "Telemetry counters reset successfully",
		"timestamp": time.Now().Format(time.RFC3339),
		"reset_by":  user.ID,
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(response)
}

// registerSecurityRoutes registers security telemetry monitoring endpoints
func (s *HelixAPIServer) registerSecurityRoutes(r *mux.Router) {
	securityRouter := r.PathPrefix("/security").Subrouter()
	securityRouter.HandleFunc("/telemetry-status", s.getSecurityTelemetryStatus).Methods("GET")
	securityRouter.HandleFunc("/telemetry-logs", s.getSecurityTelemetryLogs).Methods("GET")
	securityRouter.HandleFunc("/telemetry-counters/reset", s.postResetTelemetryCounters).Methods("POST")
}
