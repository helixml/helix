package crisp

import (
	"testing"

	"github.com/crisp-im/go-crisp-api/crisp/v3"
	"github.com/helixml/helix/api/pkg/ptr"
	"github.com/stretchr/testify/assert"
)

func Test_isInstructedToStop(t *testing.T) {
	t.Run("Simple stop", func(t *testing.T) {
		messages := []crisp.ConversationMessage{
			{
				From:    ptr.To("operator"),
				Type:    ptr.To("text"),
				Content: ptr.To(interface{}("Helix stop")),
			},
		}

		stop := isInstructedToStop("Helix", messages)
		assert.True(t, stop)
	})

	t.Run("Stop, then continue", func(t *testing.T) {
		messages := []crisp.ConversationMessage{
			{
				From:    ptr.To("operator"),
				Type:    ptr.To("text"),
				Content: ptr.To(interface{}("Helix stop")),
			},
			{
				From:    ptr.To("operator"),
				Type:    ptr.To("text"),
				Content: ptr.To(interface{}("Helix continue")),
			},
		}

		stop := isInstructedToStop("Helix", messages)
		assert.False(t, stop)
	})
}

func Test_isLastOperatorMessageHuman(t *testing.T) {
	t.Run("Last operator message is from bot", func(t *testing.T) {
		messages := []crisp.ConversationMessage{
			{
				From: ptr.To("operator"),
				Type: ptr.To("text"),
				User: &crisp.ConversationMessageUser{
					Nickname: ptr.To("Helix"),
				},
				Content: ptr.To(interface{}("Hello, how can I help?")),
			},
		}

		isHuman := isLastOperatorMessageHuman("Helix", messages)
		assert.False(t, isHuman)
	})

	t.Run("Last operator message is from human operator", func(t *testing.T) {
		messages := []crisp.ConversationMessage{
			{
				From: ptr.To("operator"),
				Type: ptr.To("text"),
				User: &crisp.ConversationMessageUser{
					Nickname: ptr.To("John"),
				},
				Content: ptr.To(interface{}("I'll take over from here")),
			},
		}

		isHuman := isLastOperatorMessageHuman("Helix", messages)
		assert.True(t, isHuman)
	})

	t.Run("Bot message followed by human operator message", func(t *testing.T) {
		messages := []crisp.ConversationMessage{
			{
				From: ptr.To("operator"),
				Type: ptr.To("text"),
				User: &crisp.ConversationMessageUser{
					Nickname: ptr.To("Helix"),
				},
				Content: ptr.To(interface{}("Hello, how can I help?")),
			},
			{
				From: ptr.To("operator"),
				Type: ptr.To("text"),
				User: &crisp.ConversationMessageUser{
					Nickname: ptr.To("Sarah"),
				},
				Content: ptr.To(interface{}("Let me handle this")),
			},
		}

		isHuman := isLastOperatorMessageHuman("Helix", messages)
		assert.True(t, isHuman)
	})

	t.Run("Human operator message followed by bot message", func(t *testing.T) {
		messages := []crisp.ConversationMessage{
			{
				From: ptr.To("operator"),
				Type: ptr.To("text"),
				User: &crisp.ConversationMessageUser{
					Nickname: ptr.To("Sarah"),
				},
				Content: ptr.To(interface{}("Helix continue")),
			},
			{
				From: ptr.To("operator"),
				Type: ptr.To("text"),
				User: &crisp.ConversationMessageUser{
					Nickname: ptr.To("Helix"),
				},
				Content: ptr.To(interface{}("Sure, I can help with that")),
			},
		}

		isHuman := isLastOperatorMessageHuman("Helix", messages)
		assert.False(t, isHuman)
	})

	t.Run("User messages with operator messages mixed", func(t *testing.T) {
		messages := []crisp.ConversationMessage{
			{
				From: ptr.To("operator"),
				Type: ptr.To("text"),
				User: &crisp.ConversationMessageUser{
					Nickname: ptr.To("Mike"),
				},
				Content: ptr.To(interface{}("I'm helping now")),
			},
			{
				From: ptr.To("user"),
				Type: ptr.To("text"),
				User: &crisp.ConversationMessageUser{
					Nickname: ptr.To("Customer"),
				},
				Content: ptr.To(interface{}("Thank you!")),
			},
		}

		isHuman := isLastOperatorMessageHuman("Helix", messages)
		assert.True(t, isHuman)
	})

	t.Run("No operator messages", func(t *testing.T) {
		messages := []crisp.ConversationMessage{
			{
				From: ptr.To("user"),
				Type: ptr.To("text"),
				User: &crisp.ConversationMessageUser{
					Nickname: ptr.To("Customer"),
				},
				Content: ptr.To(interface{}("Hello?")),
			},
		}

		isHuman := isLastOperatorMessageHuman("Helix", messages)
		assert.False(t, isHuman)
	})

	t.Run("Empty messages list", func(t *testing.T) {
		messages := []crisp.ConversationMessage{}

		isHuman := isLastOperatorMessageHuman("Helix", messages)
		assert.False(t, isHuman)
	})

	t.Run("Operator message with nil User", func(t *testing.T) {
		messages := []crisp.ConversationMessage{
			{
				From:    ptr.To("operator"),
				Type:    ptr.To("text"),
				User:    nil,
				Content: ptr.To(interface{}("Message without user")),
			},
		}

		isHuman := isLastOperatorMessageHuman("Helix", messages)
		assert.False(t, isHuman)
	})

	t.Run("Operator message with nil Nickname", func(t *testing.T) {
		messages := []crisp.ConversationMessage{
			{
				From: ptr.To("operator"),
				Type: ptr.To("text"),
				User: &crisp.ConversationMessageUser{
					Nickname: nil,
				},
				Content: ptr.To(interface{}("Message without nickname")),
			},
		}

		isHuman := isLastOperatorMessageHuman("Helix", messages)
		assert.False(t, isHuman)
	})
}

func Test_isMessageDirectedToBot(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		nickName string
		message  string
		want     bool
	}{
		{
			name:     "Message is directed to bot",
			nickName: "Helix",
			message:  "Hey Helix",
			want:     true,
		},
		{
			name:     "Message is not directed to bot",
			nickName: "Helix",
			message:  "Hey John",
			want:     false,
		},
		{
			name:     "Message is directed to bot with different case",
			nickName: "Helix",
			message:  "Hey helix",
			want:     true,
		},
		{
			name:     "Message is directed to bot with different case",
			nickName: "Helix",
			message:  "Hey HELIX",
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMessageDirectedToBot(tt.nickName, tt.message)
			if got != tt.want {
				t.Errorf("isMessageDirectedToBot() = %v, want %v", got, tt.want)
			}
		})
	}
}
