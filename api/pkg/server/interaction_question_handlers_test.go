package server

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestValidateQuestionAnswers(t *testing.T) {
	question := &types.PendingQuestion{Questions: []types.UserQuestion{
		{ID: "framework", Options: []types.UserQuestionOption{{Label: "React"}, {Label: "Vue"}}},
		{ID: "features", MultiSelect: true, Options: []types.UserQuestionOption{{Label: "Auth"}, {Label: "Billing"}}},
	}}

	require.NoError(t, validateQuestionAnswers(question, map[string]string{
		"framework": "React",
		"features":  "Auth\nBilling",
	}))
	require.Error(t, validateQuestionAnswers(question, map[string]string{
		"framework": "Svelte",
		"features":  "Auth",
	}))
	require.Error(t, validateQuestionAnswers(question, map[string]string{
		"framework": "React",
	}))
}
