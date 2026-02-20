package notification

import (
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSlackSender records calls to SendMessage
type mockSlackSender struct {
	calls []slackCall
	err   error // if set, SendMessage returns this error
}

type slackCall struct {
	userEmail string
	message   string
}

func (m *mockSlackSender) SendMessage(userEmail string, message string) error {
	m.calls = append(m.calls, slackCall{userEmail: userEmail, message: message})
	return m.err
}

func TestSendWaitlistSignupAlert_WithEmailAndName(t *testing.T) {
	slack := &mockSlackSender{}
	alerter := &AdminAlerter{slack: slack}

	user := &types.User{
		Email:    "test@example.com",
		FullName: "Test User",
	}

	alerter.sendWaitlistSignupAlert(user)

	require.Len(t, slack.calls, 1)
	assert.Equal(t, "", slack.calls[0].userEmail)
	assert.Contains(t, slack.calls[0].message, "test@example.com")
	assert.Contains(t, slack.calls[0].message, "Test User")
	assert.Contains(t, slack.calls[0].message, "waiting for approval")
}

func TestSendWaitlistSignupAlert_WithEmailOnly(t *testing.T) {
	slack := &mockSlackSender{}
	alerter := &AdminAlerter{slack: slack}

	user := &types.User{
		Email: "noname@example.com",
	}

	alerter.sendWaitlistSignupAlert(user)

	require.Len(t, slack.calls, 1)
	assert.Contains(t, slack.calls[0].message, "noname@example.com")
	assert.NotContains(t, slack.calls[0].message, "()")
	assert.Contains(t, slack.calls[0].message, "waiting for approval")
}

func TestSendWaitlistSignupAlert_NilSlack(t *testing.T) {
	alerter := &AdminAlerter{slack: nil}

	user := &types.User{
		Email: "test@example.com",
	}

	// Should not panic
	alerter.sendWaitlistSignupAlert(user)
}

func TestSendWaitlistSignupAlert_SlackError(t *testing.T) {
	slack := &mockSlackSender{err: errors.New("webhook failed")}
	alerter := &AdminAlerter{slack: slack}

	user := &types.User{
		Email:    "test@example.com",
		FullName: "Test User",
	}

	// Should not panic — errors are logged, not returned
	alerter.sendWaitlistSignupAlert(user)

	require.Len(t, slack.calls, 1)
}
