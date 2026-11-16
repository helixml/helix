package external_agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func trustedStartupEntries(env []string) []string {
	var out []string
	for _, e := range env {
		if strings.HasPrefix(e, TrustedStartupSHAEnv+"=") {
			out = append(out, e)
		}
	}
	return out
}

// The startup commit comes from the repository row alone: a value an agent
// put in agent.Env or a project secret never reaches the container.
func TestPrimaryRepoEnvTrustedStartupSHAComesFromRepository(t *testing.T) {
	const human = "1111111111111111111111111111111111111111"
	const forged = TrustedStartupSHAEnv + "=2222222222222222222222222222222222222222"

	for _, tc := range []struct {
		name string
		repo *types.GitRepository
		err  error
		want string
	}{
		{"human commit", &types.GitRepository{ID: "repo_1", Name: "kit", TrustedStartupSHA: human}, nil, human},
		{"no trusted commit", &types.GitRepository{ID: "repo_1", Name: "kit"}, nil, ""},
		{"repository lookup fails", nil, errors.New("db down"), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			st := store.NewMockStore(ctrl)
			st.EXPECT().GetGitRepository(gomock.Any(), "repo_1").Return(tc.repo, tc.err)
			h := &HydraExecutor{store: st}

			env := []string{"A=1", forged, "B=2", forged}
			env = h.primaryRepoEnv(context.Background(), &types.DesktopAgent{PrimaryRepositoryID: "repo_1"}, env)

			got := trustedStartupEntries(env)
			if len(got) != 1 || got[0] != TrustedStartupSHAEnv+"="+tc.want {
				t.Fatalf("trusted startup env = %v, want exactly %s=%s", got, TrustedStartupSHAEnv, tc.want)
			}
		})
	}
}

func TestPrimaryRepoEnvClearsForgedSHAWithoutPrimaryRepo(t *testing.T) {
	h := &HydraExecutor{}
	env := h.primaryRepoEnv(context.Background(), &types.DesktopAgent{}, []string{TrustedStartupSHAEnv + "=2222222222222222222222222222222222222222"})
	if got := trustedStartupEntries(env); len(got) != 1 || got[0] != TrustedStartupSHAEnv+"=" {
		t.Fatalf("trusted startup env = %v, want it cleared", got)
	}
}
