package config

import (
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/require"
)

func TestAuditLogDefaults(t *testing.T) {
	var cfg ServerConfig
	require.NoError(t, envconfig.Process("", &cfg))

	require.True(t, cfg.AuditLog.Enabled)
	require.True(t, cfg.AuditLog.Stdout)
	require.True(t, cfg.AuditLog.Database)
	require.Equal(t, 180, cfg.AuditLog.RetentionDays)
	require.Equal(t, 8192, cfg.AuditLog.BufferSize)
	require.True(t, cfg.AuditLog.SkipRunnerReads)
}

func TestAuditLogOverrides(t *testing.T) {
	t.Setenv("HELIX_AUDIT_LOG_ENABLED", "false")
	t.Setenv("HELIX_AUDIT_LOG_STDOUT", "false")
	t.Setenv("HELIX_AUDIT_LOG_DB", "false")
	t.Setenv("HELIX_AUDIT_LOG_RETENTION_DAYS", "30")
	t.Setenv("HELIX_AUDIT_LOG_BUFFER_SIZE", "256")
	t.Setenv("HELIX_AUDIT_LOG_SKIP_RUNNER_READS", "false")

	var cfg ServerConfig
	require.NoError(t, envconfig.Process("", &cfg))

	require.False(t, cfg.AuditLog.Enabled)
	require.False(t, cfg.AuditLog.Stdout)
	require.False(t, cfg.AuditLog.Database)
	require.Equal(t, 30, cfg.AuditLog.RetentionDays)
	require.Equal(t, 256, cfg.AuditLog.BufferSize)
	require.False(t, cfg.AuditLog.SkipRunnerReads)
}
