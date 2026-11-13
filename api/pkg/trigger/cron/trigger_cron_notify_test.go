package cron

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// Failure notifications for external agent runs identify the app's owner and
// organization, which recipient filtering needs to deliver them.
func TestExternalAgentFailureNotificationCarriesOwner(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	notifier := notification.NewMockNotifier(ctrl)

	st.EXPECT().CreateTriggerExecutionUnlessRunning(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, e *types.TriggerExecution) (*types.TriggerExecution, bool, error) {
			return e, true, nil
		})
	st.EXPECT().UpdateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, e *types.TriggerExecution) (*types.TriggerExecution, error) { return e, nil })

	var got *types.Notification
	notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, n *types.Notification) error { got = n; return nil })

	app := &types.App{ID: "app-1", Owner: "usr_owner", OrganizationID: "org_1"}
	trigger := &types.CronTrigger{Emails: []string{"owner@example.com"}}
	_, err := executeExternalAgentCronTask(context.Background(), st, nil, notifier, app, "usr_owner", "trg-1", trigger, "nightly", "do it", testCronActor)
	require.Error(t, err)

	require.NotNil(t, got)
	require.Equal(t, types.EventCronTriggerFailed, got.Event)
	require.NotNil(t, got.Session)
	require.Equal(t, "usr_owner", got.Session.Owner)
	require.Equal(t, "org_1", got.Session.OrganizationID)
	require.Equal(t, []string{"owner@example.com"}, got.Emails)
}
