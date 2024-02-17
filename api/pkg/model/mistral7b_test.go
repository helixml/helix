package model

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/assert"
)

func Test_formatPrompt(t *testing.T) {
	type args struct {
		session *types.Session
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "one message",
			args: args{
				session: &types.Session{
					Interactions: []*types.Interaction{
						{
							Creator: "user",
							Message: "hello",
						},
					},
				},
			},
			want: "[INST]hello[/INST]\n",
		},
		{
			name: "skip empty message",
			args: args{
				session: &types.Session{
					Interactions: []*types.Interaction{
						{
							Creator: "user",
							Message: "hello",
						},
						{
							Creator: "user",
							Message: "",
						},
					},
				},
			},
			want: "[INST]hello[/INST]\n",
		},
		{
			name: "one message, system prompt",
			args: args{
				session: &types.Session{
					Metadata: types.SessionMetadata{
						SystemPrompt: "system prompt",
					},
					Interactions: []*types.Interaction{
						{
							Creator: "user",
							Message: "hello",
						},
					},
				},
			},
			want: "[INST]system prompt[/INST]\n[INST]hello[/INST]\n",
		},
		{
			name: "limited messages",
			args: args{
				session: &types.Session{
					Interactions: []*types.Interaction{
						{Creator: "user", Message: "be nice"},
						{Creator: "assistant", Message: "ok"},
						{Creator: "user", Message: "q1"},
						{Creator: "assistant", Message: "a1"},
						{Creator: "user", Message: "q2"},
						{Creator: "assistant", Message: "a2"},
						{Creator: "user", Message: "q3"},
						{Creator: "assistant", Message: "a3"},
						{Creator: "user", Message: "q4"},
						{Creator: "assistant", Message: "a4"},
						{Creator: "user", Message: "q5"},
						{Creator: "assistant", Message: "a5"},
						{Creator: "user", Message: "q6"},
						{Creator: "assistant", Message: "a6"},
						{Creator: "user", Message: "q7"},
						{Creator: "assistant", Message: "a7"},
						{Creator: "user", Message: "q8"},
						{Creator: "assistant", Message: "a8"},
					},
				},
			},
			want: `[INST]be nice[/INST]
[INST]q5[/INST]
a5
[INST]q6[/INST]
a6
[INST]q7[/INST]
a7
[INST]q8[/INST]
a8
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPrompt(tt.args.session); got != tt.want {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
