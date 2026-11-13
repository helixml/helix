package server

import (
	"testing"
	"time"

	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestActivationDeliveryOutputUsesRequestedInitialInteraction(t *testing.T) {
	_, store, client, _, ctx := newInProcTestSetup(t)
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	_, err := store.CreateInteraction(ctx, &types.Interaction{
		ID: "int_activation_initial", SessionID: "ses_activation", Created: base,
		State: types.InteractionStateComplete, ResponseMessage: "initial output",
	})
	require.NoError(t, err)
	_, err = store.CreateInteraction(ctx, &types.Interaction{
		ID: "int_activation_unrelated", SessionID: "ses_activation", Created: base.Add(time.Minute),
		State: types.InteractionStateWaiting,
	})
	require.NoError(t, err)

	output, err := client.GetDeliveryOutput(ctx, runtimehelix.DeliveryResult{
		SessionID: "ses_activation", InteractionID: "int_activation_initial",
	})
	require.NoError(t, err)
	require.Equal(t, "int_activation_initial", output.InteractionID)
	require.Equal(t, string(types.InteractionStateComplete), output.Status)
	require.Equal(t, "initial output", output.Output)
}

func TestActivationDeliveryOutputUsesLatestPromptAttempt(t *testing.T) {
	_, store, client, _, ctx := newInProcTestSetup(t)
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	interactions := []*types.Interaction{
		{ID: "int_activation_old", SessionID: "ses_activation", PromptID: "pmt_activation", Created: base,
			State: types.InteractionStateError, Error: "old failure"},
		{ID: "int_activation_retry", SessionID: "ses_activation", PromptID: "pmt_activation", Created: base.Add(time.Minute),
			State: types.InteractionStateComplete, ResponseMessage: "retry output"},
		{ID: "int_activation_unrelated", SessionID: "ses_activation", PromptID: "pmt_other", Created: base.Add(2 * time.Minute),
			State: types.InteractionStateWaiting},
	}
	for _, interaction := range interactions {
		_, err := store.CreateInteraction(ctx, interaction)
		require.NoError(t, err)
	}

	output, err := client.GetDeliveryOutput(ctx, runtimehelix.DeliveryResult{
		SessionID: "ses_activation", PromptID: "pmt_activation",
	})
	require.NoError(t, err)
	require.Equal(t, "int_activation_retry", output.InteractionID)
	require.Equal(t, string(types.InteractionStateComplete), output.Status)
	require.Equal(t, "retry output", output.Output)
}
