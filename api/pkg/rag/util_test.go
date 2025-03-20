package rag

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestParseFilterActions(t *testing.T) {
	for _, test := range []struct {
		name     string
		prompt   string
		expected []string
	}{
		{
			name:     "document id with square brackets",
			prompt:   "This is a test prompt @filter([DOC_ID:doc1])",
			expected: []string{"[DOC_ID:doc1]"},
		},
		{
			name:     "random normal text",
			prompt:   "This is a test prompt @filter(doc1)",
			expected: []string{"doc1"},
		},
		{
			name:     "multiple filter actions",
			prompt:   "This is a test prompt @filter([DOC_ID:doc1]) @filter([DOC_ID:doc2])",
			expected: []string{"[DOC_ID:doc1]", "[DOC_ID:doc2]"},
		},
		{
			name:     "fail when there is a bracket in the document id",
			prompt:   "This is a test prompt @filter([DOC()])",
			expected: []string{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			filterActions := ParseFilterActions(test.prompt)
			assert.DeepEqual(t, test.expected, filterActions)
		})
	}
}
