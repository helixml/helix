package server

import (
	"context"
	"encoding/json"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// TestHumanInboxNotifyExpectsReply verifies expectsReply controls the no_reply
// metadata flag the frontend keys off to show/hide the Respond affordance.
func TestHumanInboxNotifyExpectsReply(t *testing.T) {
	cases := []struct {
		name         string
		expectsReply bool
		wantNoReply  bool
	}{
		{"reply expected -> no no_reply flag", true, false},
		{"fyi -> no_reply flag set", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockStore := store.NewMockStore(ctrl)

			var captured *types.AttentionEvent
			mockStore.EXPECT().
				CreateAttentionEvent(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, ev *types.AttentionEvent) (*types.AttentionEvent, error) {
					captured = ev
					return ev, nil
				})

			h := humanInbox{store: mockStore}
			if err := h.Notify(context.Background(), "org_1", "usr_1", "chief-of-staff", "Chief of Staff", "Priya", "hi", tc.expectsReply); err != nil {
				t.Fatalf("Notify: %v", err)
			}

			var meta map[string]string
			if err := json.Unmarshal(captured.Metadata, &meta); err != nil {
				t.Fatalf("unmarshal metadata: %v", err)
			}
			_, hasNoReply := meta["no_reply"]
			if hasNoReply != tc.wantNoReply {
				t.Fatalf("no_reply present = %v, want %v (metadata=%s)", hasNoReply, tc.wantNoReply, captured.Metadata)
			}
			if meta["bot_id"] != "chief-of-staff" {
				t.Fatalf("bot_id = %q, want chief-of-staff", meta["bot_id"])
			}
		})
	}
}
