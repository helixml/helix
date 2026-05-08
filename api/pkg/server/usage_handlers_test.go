package server

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestMergeSandboxUsageCostsAddsSandboxSpend(t *testing.T) {
	date := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	metrics := []*types.AggregatedUsageMetric{
		{
			Date:           date,
			PromptCost:     1,
			CompletionCost: 2,
			TotalCost:      3,
			TotalRequests:  4,
		},
	}
	sandboxMetrics := []*types.AggregatedUsageMetric{
		{
			Date:          date,
			SandboxCost:   2.5,
			TotalCost:     2.5,
			TotalRequests: 1,
		},
	}

	mergeSandboxUsageCosts(metrics, sandboxMetrics)

	require.Equal(t, 2.5, metrics[0].SandboxCost)
	require.Equal(t, 5.5, metrics[0].TotalCost)
	require.Equal(t, 5, metrics[0].TotalRequests)
}
