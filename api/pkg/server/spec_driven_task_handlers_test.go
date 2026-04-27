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
			name:   "post-implementation: implementation_review → done",
			task:   &types.SpecTask{Status: types.TaskStatusImplementationReview, SandboxState: "running"},
			latest: &types.Interaction{State: types.InteractionStateWaiting},
			want:   types.AgentWorkStateDone,
		},
		{
			name:   "post-implementation: pull_request → done",
			task:   &types.SpecTask{Status: types.TaskStatusPullRequest, SandboxState: "absent"},
			latest: nil,
			want:   types.AgentWorkStateDone,
		},
		{
			name:   "post-implementation: done → done",
			task:   &types.SpecTask{Status: types.TaskStatusDone, SandboxState: "absent"},
			latest: nil,
			want:   types.AgentWorkStateDone,
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
