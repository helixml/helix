package services

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGitReceivePackRefs(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []string
	}{
		{
			name:     "empty input",
			input:    []byte{},
			expected: nil,
		},
		{
			name:     "flush packet only",
			input:    []byte("0000"),
			expected: nil,
		},
		{
			name: "single branch push",
			// Format: <4-byte hex length><old-sha> <new-sha> refs/heads/<branch>\0<capabilities>\n
			input: buildPktLine(
				"0000000000000000000000000000000000000000 1234567890123456789012345678901234567890 refs/heads/feature/0004-my-branch",
				"report-status side-band-64k",
			),
			expected: []string{"feature/0004-my-branch"},
		},
		{
			name: "multiple branch push",
			input: append(
				buildPktLine(
					"0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 refs/heads/feature/0001-first",
					"report-status",
				),
				append(
					buildPktLine(
						"2222222222222222222222222222222222222222 3333333333333333333333333333333333333333 refs/heads/feature/0002-second",
						"",
					),
					[]byte("0000")..., // flush packet
				)...,
			),
			expected: []string{"feature/0001-first", "feature/0002-second"},
		},
		{
			name: "branch push with helix-specs",
			input: append(
				buildPktLine(
					"0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 refs/heads/helix-specs",
					"report-status",
				),
				append(
					buildPktLine(
						"2222222222222222222222222222222222222222 3333333333333333333333333333333333333333 refs/heads/feature/0004-fix-bug",
						"",
					),
					[]byte("0000")...,
				)...,
			),
			expected: []string{"helix-specs", "feature/0004-fix-bug"},
		},
		{
			name: "tag push (should be ignored)",
			input: append(
				buildPktLine(
					"0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 refs/tags/v1.0.0",
					"report-status",
				),
				[]byte("0000")...,
			),
			expected: nil,
		},
		{
			name: "mixed refs and tags",
			input: append(
				buildPktLine(
					"0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 refs/heads/main",
					"report-status",
				),
				append(
					buildPktLine(
						"2222222222222222222222222222222222222222 3333333333333333333333333333333333333333 refs/tags/v1.0.0",
						"",
					),
					append(
						buildPktLine(
							"4444444444444444444444444444444444444444 5555555555555555555555555555555555555555 refs/heads/feature/test",
							"",
						),
						[]byte("0000")...,
					)...,
				)...,
			),
			expected: []string{"main", "feature/test"},
		},
		{
			name: "real-world example with branch containing slashes",
			input: append(
				buildPktLine(
					"abcdef1234567890abcdef1234567890abcdef12 fedcba0987654321fedcba0987654321fedcba09 refs/heads/feature/0003-based-on-guidelines",
					"report-status delete-refs side-band-64k quiet object-format=sha1 agent=git/2.43.0",
				),
				[]byte("0000PACK...")..., // flush packet followed by packfile
			),
			expected: []string{"feature/0003-based-on-guidelines"},
		},
		{
			name:     "malformed input - truncated length",
			input:    []byte("00"),
			expected: nil,
		},
		{
			name:     "malformed input - invalid hex",
			input:    []byte("ZZZZ"),
			expected: nil,
		},
		{
			name: "branch name with special chars",
			input: append(
				buildPktLine(
					"0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 refs/heads/user/john.doe/fix-bug-123",
					"",
				),
				[]byte("0000")...,
			),
			expected: []string{"user/john.doe/fix-bug-123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseGitReceivePackRefs(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// buildPktLine creates a git pkt-line with the given content and optional capabilities
// Format: <4-byte hex length><content>[\0<capabilities>]\n
// The length includes the 4-byte prefix itself
func buildPktLine(content, capabilities string) []byte {
	var line string
	if capabilities != "" {
		line = content + "\x00" + capabilities + "\n"
	} else {
		line = content + "\n"
	}

	// Length includes the 4-byte length prefix itself
	length := len(line) + 4

	// Format as 4-character hex string
	return []byte(fmt.Sprintf("%04x%s", length, line))
}

// TestBuildPktLine verifies our test helper creates valid pkt-lines
func TestBuildPktLine(t *testing.T) {
	// Test a simple case
	line := buildPktLine("hello", "")
	// "hello\n" = 6 bytes + 4 byte prefix = 10 = 0x000a
	assert.Equal(t, "000ahello\n", string(line))

	// Test with capabilities
	line = buildPktLine("test", "cap1 cap2")
	// "test\0cap1 cap2\n" = 15 bytes + 4 = 19 = 0x0013
	assert.Equal(t, "0013test\x00cap1 cap2\n", string(line))
}
