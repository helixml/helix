package controller

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestLoadAssistant_AppliesDirectChatReasoningEffort(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetUserMeta(gomock.Any(), "user-1").Return(&types.UserMeta{}, nil)

	c := &Controller{Options: Options{Store: mockStore}}
	assistant, err := c.loadAssistant(
		context.Background(),
		&types.User{ID: "user-1"},
		&ChatCompletionOptions{ReasoningEffort: "high"},
	)

	require.NoError(t, err)
	require.Equal(t, "high", assistant.ReasoningEffort)
}
