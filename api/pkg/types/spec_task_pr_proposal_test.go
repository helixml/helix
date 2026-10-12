package types

import "testing"

func TestInitAutoApprovePullRequests(t *testing.T) {
	on, off := true, false
	projectOn := &Project{AutoApprovePullRequests: true}
	projectOff := &Project{}
	cases := []struct {
		name      string
		requested *bool
		project   *Project
		want      bool
	}{
		{"project default on", nil, projectOn, true},
		{"project default off", nil, projectOff, false},
		{"no project", nil, nil, false},
		{"explicit off overrides project on", &off, projectOn, false},
		{"explicit on overrides project off", &on, projectOff, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := &SpecTask{}
			task.InitAutoApprovePullRequests(tc.requested, tc.project, "usr_creator")
			if task.AutoApprovePullRequests != tc.want {
				t.Fatalf("enabled = %v, want %v", task.AutoApprovePullRequests, tc.want)
			}
			wantBy := ""
			if tc.want {
				wantBy = "usr_creator"
			}
			if task.AutoApprovePullRequestsBy != wantBy {
				t.Fatalf("by = %q, want %q", task.AutoApprovePullRequestsBy, wantBy)
			}
		})
	}
}
