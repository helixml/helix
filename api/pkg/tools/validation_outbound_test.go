package tools

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/stretchr/testify/require"
)

// API tool URLs the outbound policy would block at call time are refused.
func TestCheckAPIToolDestination_RejectsReservedDestinations(t *testing.T) {
	for _, url := range []string{
		"http://127.0.0.1:8080",
		"http://localhost/api",
		"http://10.0.0.5",
		"http://192.168.1.10:9000",
		"http://169.254.169.254/latest/meta-data",
		"http://[::1]:8080",
		"http://[fd00:ec2::254]",
		"file:///etc/passwd",
	} {
		require.Error(t, CheckAPIToolDestination(context.Background(), netguard.Policy{}, url), url)
	}
}

func TestCheckAPIToolDestination_AllowsPermittedDestinations(t *testing.T) {
	ctx := context.Background()
	// A relative OpenAPI server URL names no host and was always accepted.
	require.NoError(t, CheckAPIToolDestination(ctx, netguard.Policy{}, "/api/v1"))
	require.NoError(t, CheckAPIToolDestination(ctx, netguard.Policy{}, "https://93.184.215.14"))
	// Self-hosted deployments that allow private networks keep working...
	require.NoError(t, CheckAPIToolDestination(ctx, netguard.Policy{AllowPrivate: true}, "http://10.0.0.5"))
	// ...but link-local and metadata stay blocked.
	require.Error(t, CheckAPIToolDestination(ctx, netguard.Policy{AllowPrivate: true}, "http://169.254.169.254"))
}
