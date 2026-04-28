package server

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestDeriveAgentWorkState(t *testing.T) {
	cases := []struct {
		name   string
		task   *types.SpecTask
		latest *types.Interaction
		want   types.AgentWorkState
	}{
		{
			name:   "sandbox absent → empty (UI shows sandbox hint)",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "absent"},
			latest: &types.Interaction{State: types.InteractionStateWaiting},
			want:   "",
		},
		{
			name:   "sandbox starting → empty",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "starting"},
			latest: nil,
			want:   "",
		},
		{
			name:   "running + waiting interaction → working",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "running"},
			latest: &types.Interaction{State: types.InteractionStateWaiting},
			want:   types.AgentWorkStateWorking,
		},
		{
			name:   "running + complete interaction → idle",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "running"},
			latest: &types.Interaction{State: types.InteractionStateComplete},
			want:   types.AgentWorkStateIdle,
		},
		{
			name:   "running + error interaction → idle (agent isn't actively working)",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "running"},
			latest: &types.Interaction{State: types.InteractionStateError},
			want:   types.AgentWorkStateIdle,
		},
		{
			name:   "running + no interaction at all → idle",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "running"},
			latest: nil,
			want:   types.AgentWorkStateIdle,
		},
		{
			name:   "running + editing interaction → idle (only Waiting counts as working)",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "running"},
			latest: &types.Interaction{State: types.InteractionStateEditing},
			want:   types.AgentWorkStateIdle,
		},
		{
			name:   "running + none-state interaction → idle (transient pre-Waiting state)",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "running"},
			latest: &types.Interaction{State: types.InteractionStateNone},
			want:   types.AgentWorkStateIdle,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveAgentWorkState(tc.task, tc.latest)
			if got != tc.want {
				t.Errorf("deriveAgentWorkState(%+v, %+v) = %q; want %q", tc.task, tc.latest, got, tc.want)
			}
		})
	}
}
