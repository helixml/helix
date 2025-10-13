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
