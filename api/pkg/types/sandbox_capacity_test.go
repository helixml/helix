package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSandboxMaxSandboxes(t *testing.T) {
	tests := []struct {
		value string
		want  int
		ok    bool
	}{
		{value: "", want: 0, ok: true},
		{value: " 1 ", want: 1, ok: true},
		{value: "20", want: 20, ok: true},
		{value: "0", want: 0, ok: true},
		{value: "-1", want: -1, ok: true},
		{value: "-2", want: 0, ok: false},
		{value: "invalid", want: 0, ok: false},
	}

	for _, test := range tests {
		got, ok := ParseSandboxMaxSandboxes(test.value)
		assert.Equal(t, test.want, got, test.value)
		assert.Equal(t, test.ok, ok, test.value)
	}
}
