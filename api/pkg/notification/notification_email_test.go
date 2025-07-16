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
		Event: EventCronTriggerComplete,
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
	require.True(t, contains(message, expectedURL), "expected message to contain URL '%s', but it doesn't", expectedURL)

	// Check that message contains the session name
	require.True(t, contains(message, "Test Session"), "expected message to contain session name 'Test Session', but it doesn't")

	// Check that message contains the test message
	require.True(t, contains(message, "Test Message"), "expected message to contain 'Test Message', but it doesn't")
}

func Test_getEmailMessage_CronTriggerFailed(t *testing.T) {
	cfg := &config.Notifications{
		AppURL: "https://app.helix.ai",
	}

	notifier := &Email{
		cfg: cfg,
	}

	title, message, err := notifier.getEmailMessage(&Notification{
		Event: EventCronTriggerFailed,
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
	require.True(t, contains(message, expectedURL), "expected message to contain URL '%s', but it doesn't", expectedURL)

	// Check that message contains the session name
	require.True(t, contains(message, "Failed Session"), "expected message to contain session name 'Failed Session', but it doesn't")

	// Check that message contains the test message
	require.True(t, contains(message, "Task failed due to error"), "expected message to contain 'Task failed due to error', but it doesn't")
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
		Event: Event(999), // Use an unknown event value
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
	require.True(t, contains(err.Error(), "unknown event"), "expected error to contain 'unknown event', got '%s'", err.Error())
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			func() bool {
				for i := 1; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())))
}
