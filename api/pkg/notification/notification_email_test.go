package notification

import (
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/require"
)

func Test_getEmailMessage_CronTriggerComplete(t *testing.T) {
	cfg := &config.Notifications{
		AppURL: "https://app.helix.ai",
	}

	notifier := &Email{
		cfg: cfg,
	}

	title, message, err := notifier.getEmailMessage(&Notification{
		Event: types.EventCronTriggerComplete,
		Session: &types.Session{
			ID:   "123",
			Name: "Test Session",
		},
		Message: "Test Message",
	})
	require.NoError(t, err)

	// Check that title is the session name
	require.Equal(t, "Test Session", title)

	// Check that message contains expected content
	require.NotEmpty(t, message)

	// Check that message contains the session URL
	expectedURL := "https://app.helix.ai/session/123"
	require.Contains(t, message, expectedURL, "expected message to contain URL '%s', but it doesn't", expectedURL)

	// Check that message contains the session name
	require.Contains(t, message, "Test Session", "expected message to contain session name 'Test Session', but it doesn't")

	// Check that message contains the test message
	require.Contains(t, message, "Test Message", "expected message to contain 'Test Message', but it doesn't")
}

func Test_getEmailMessage_UnknownEvent(t *testing.T) {
	cfg := &config.Notifications{
		AppURL: "https://app.helix.ai",
	}

	notifier := &Email{
		cfg: cfg,
	}

	// Create a notification with an unknown event type
	notification := &Notification{
		Event: types.Event(999), // Use an unknown event value
		Session: &types.Session{
			ID:   "789",
			Name: "Test Session",
		},
		Message: "Test Message",
	}

	title, message, err := notifier.getEmailMessage(notification)

	// Should return an error for unknown event
	require.Error(t, err)

	// Title and message should be empty when there's an error
	require.Empty(t, title)

	require.Empty(t, message)

	// Check that error message contains the unknown event
	require.Contains(t, err.Error(), "unknown event", "expected error to contain 'unknown event', got '%s'", err.Error())
}

func Test_getEmailMessage_CronTriggerComplete_Markdown(t *testing.T) {
	cfg := &config.Notifications{
		AppURL: "https://app.helix.ai",
	}

	notifier := &Email{
		cfg: cfg,
	}

	const messageMarkdown = `
# Test Message

This is a test message.

- List item1
`

	messageHTML, err := notifier.renderMarkdown(messageMarkdown)
	require.NoError(t, err)

	// Check that message contains the test message
	require.Contains(t, messageHTML, "<p>This is a test message.</p>", "expected message to contain 'This is a test message.', but it doesn't")

	// Check that message contains the list item
	require.Contains(t, messageHTML, "<li>List item1</li>", "expected message to contain 'List item1', but it doesn't")
}

func Test_getEmailMessage_CronTriggerFailed(t *testing.T) {
	cfg := &config.Notifications{
		AppURL: "https://app.helix.ai",
	}

	notifier := &Email{
		cfg: cfg,
	}

	title, message, err := notifier.getEmailMessage(&Notification{
		Event: types.EventCronTriggerFailed,
		Session: &types.Session{
			ID:   "456",
			Name: "Failed Session",
		},
		Message: "Task failed due to error",
	})

	require.NoError(t, err)

	// Check that title is the session name
	require.Equal(t, "Failed Session", title)

	// Check that message contains expected content
	require.NotEmpty(t, message)

	// Check that message contains the session URL
	expectedURL := "https://app.helix.ai/session/456"
	require.Contains(t, message, expectedURL, "expected message to contain URL '%s', but it doesn't", expectedURL)

	// Check that message contains the test message
	require.Contains(t, message, "Task failed due to error", "expected message to contain 'Task failed due to error', but it doesn't")
}

func Test_renderMarkdown_WithDividers(t *testing.T) {
	message := `─────────────────────────────────────────────  
	1. NVDA (NVIDIA Corporation)  
	───────────────────────────────────────────── `

	cfg := &config.Notifications{
		AppURL: "https://app.helix.ai",
	}

	notifier := &Email{
		cfg: cfg,
	}

	converted, err := notifier.renderMarkdown(message)
	require.NoError(t, err)

	require.Contains(t, converted, "<hr>")
	require.Contains(t, converted, "1. NVDA (NVIDIA Corporation)")
}
