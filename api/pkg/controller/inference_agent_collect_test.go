package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/agent"
)

// responder replays a fixed sequence of agent responses, then blocks the way a
// real session would if the drain loop asked for more than the agent sent.
func responder(t *testing.T, responses ...agent.Response) func() agent.Response {
	t.Helper()

	i := 0
	return func() agent.Response {
		if i >= len(responses) {
			t.Fatalf("drain loop read past the end of the agent's output (%d responses)", len(responses))
		}
		r := responses[i]
		i++
		return r
	}
}

func TestCollectAgentResponse_AccumulatesText(t *testing.T) {
	got, err := collectAgentResponse(responder(t,
		agent.Response{Type: agent.ResponseTypePartialText, Content: "The Porsche "},
		agent.Response{Type: agent.ResponseTypePartialText, Content: "is black"},
		agent.Response{Type: agent.ResponseTypeEnd},
	))

	require.NoError(t, err)
	assert.Equal(t, "The Porsche is black", got)
}

// The regression: an agent error was dropped on the floor, so a failed
// inference returned an empty string and no error. Callers turned that into
// HTTP 200 with an empty assistant message, which is how a provider 429 reached
// CI as `"" does not contain "black"` instead of the actual cause.
func TestCollectAgentResponse_SurfacesAgentError(t *testing.T) {
	_, err := collectAgentResponse(responder(t,
		agent.Response{Type: agent.ResponseTypeError, Content: "429 Too Many Requests: You have no credits remaining"},
		agent.Response{Type: agent.ResponseTypeEnd},
	))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no credits remaining")
}

// Partial output before a failure must not be passed off as a complete answer.
func TestCollectAgentResponse_DiscardsPartialTextOnError(t *testing.T) {
	got, err := collectAgentResponse(responder(t,
		agent.Response{Type: agent.ResponseTypePartialText, Content: "The Porsche is "},
		agent.Response{Type: agent.ResponseTypeError, Content: "upstream exploded"},
		agent.Response{Type: agent.ResponseTypeEnd},
	))

	require.Error(t, err)
	assert.Empty(t, got, "a truncated answer must not be returned as if it were complete")
}

// The first error is the useful one; later ones are usually fallout.
func TestCollectAgentResponse_ReportsFirstError(t *testing.T) {
	_, err := collectAgentResponse(responder(t,
		agent.Response{Type: agent.ResponseTypeError, Content: "first failure"},
		agent.Response{Type: agent.ResponseTypeError, Content: "second failure"},
		agent.Response{Type: agent.ResponseTypeEnd},
	))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "first failure")
	assert.NotContains(t, err.Error(), "second failure")
}

// The loop must keep reading to End even after an error, or the agent goroutine
// is left blocked on a send nobody receives.
func TestCollectAgentResponse_DrainsToEndAfterError(t *testing.T) {
	drained := 0
	next := func() agent.Response {
		drained++
		switch drained {
		case 1:
			return agent.Response{Type: agent.ResponseTypeError, Content: "boom"}
		case 2:
			return agent.Response{Type: agent.ResponseTypePartialText, Content: "trailing chunk"}
		default:
			return agent.Response{Type: agent.ResponseTypeEnd}
		}
	}

	_, err := collectAgentResponse(next)

	require.Error(t, err)
	assert.Equal(t, 3, drained, "must consume everything up to and including End")
}

// Response types the loop does not care about (thinking, step info) must not
// end the drain or leak into the answer.
func TestCollectAgentResponse_IgnoresOtherResponseTypes(t *testing.T) {
	got, err := collectAgentResponse(responder(t,
		agent.Response{Type: agent.ResponseTypeThinking, Content: "considering"},
		agent.Response{Type: agent.ResponseTypePartialText, Content: "answer"},
		agent.Response{Type: agent.ResponseTypeEnd},
	))

	require.NoError(t, err)
	assert.Equal(t, "answer", got)
}
