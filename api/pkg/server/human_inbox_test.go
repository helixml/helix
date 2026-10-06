package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func TestOrgMemberNotifierCreatesReadOnlyAttentionEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)

	var captured *types.AttentionEvent
	mockStore.EXPECT().CreateAttentionEvent(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, event *types.AttentionEvent) (*types.AttentionEvent, error) {
			captured = event
			return event, nil
		})

	err := (orgMemberNotifier{store: mockStore}).NotifyInfo(
		context.Background(), "org-1", "usr-1", "chief-of-staff",
		"Chief of Staff is starting", "This can take a few minutes.",
	)
	require.NoError(t, err)
	require.Equal(t, "usr-1", captured.UserID)
	require.Equal(t, "org-1", captured.OrganizationID)
	require.Equal(t, types.AttentionEventOrgMessage, captured.EventType)

	var metadata map[string]string
	require.NoError(t, json.Unmarshal(captured.Metadata, &metadata))
	require.Equal(t, "chief-of-staff", metadata["bot_id"])
	require.Equal(t, "true", metadata["no_reply"])
}
