package data

import (
	"reflect"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestGetLastInteractions(t *testing.T) {
	type args struct {
		session *types.Session
		limit   int
	}
	tests := []struct {
		name string
		args args
		want []*types.Interaction
	}{
		{
			name: "none",
			args: args{
				session: &types.Session{
					Interactions: []*types.Interaction{},
				},
				limit: 6,
			},
			want: []*types.Interaction{},
		},
		{
			name: "exact",
			args: args{
				session: &types.Session{
					Interactions: []*types.Interaction{
						{
							ID: "1",
						},
						{
							ID: "2",
						},
						{
							ID: "3",
						},
						{
							ID: "4",
						},
					},
				},
				limit: 4,
			},
			want: []*types.Interaction{
				{
					ID: "1",
				},
				{
					ID: "2",
				},
				{
					ID: "3",
				},
				{
					ID: "4",
				},
			},
		},
		{
			name: "limited",
			args: args{
				session: &types.Session{
					Interactions: []*types.Interaction{
						{
							ID: "1",
						},
						{
							ID: "2",
						},
						{
							ID: "3",
						},
						{
							ID: "4",
						},
					},
				},
				limit: 2,
			},
			want: []*types.Interaction{
				{
					ID: "3",
				},
				{
					ID: "4",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetLastInteractions(tt.args.session, tt.args.limit)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetLastInteractions() = %v, want %v", got, tt.want)
			}
		})
	}
}
