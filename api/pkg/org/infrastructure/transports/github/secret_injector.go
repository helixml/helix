package github

import (
	"context"

	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
)

// NewSecretInjector returns the GitHub transport's
// SpawnSecretInjector: a per-activation hook that resolves the
// org's GitHub OAuth access token via the provided resolver and
// returns it as the `GH_TOKEN` secret. The host wires the returned
// injector onto SpawnerConfig.SecretInjectors; the spawner upserts
// the result as a project secret on every activation so the
// worker's `gh` CLI starts authenticated.
//
// Reuses the existing TokenResolver — same resolver the github
// stream transport's outbound path uses for its `Token()` lookup,
// so there's one OAuth pipeline driving every helix-org → GitHub
// touchpoint.
//
// Soft-skips by design:
//   - resolver == nil  → empty map (no injection wired)
//   - resolver returns "" without error → empty map (operator
//     hasn't connected GitHub yet — leaving any previously-valid
//     secret alone)
//   - resolver returns an error → propagated; the spawner logs +
//     keeps going so a GitHub API outage can't take an activation
//     down.
func NewSecretInjector(resolver TokenResolver) runtimehelix.SpawnSecretInjector {
	return runtimehelix.SpawnSecretInjectorFunc{
		Label: "github",
		Fn: func(ctx context.Context, orgID string) (map[string]string, error) {
			if resolver == nil {
				return nil, nil
			}
			token, err := resolver(ctx, orgID)
			if err != nil {
				return nil, err
			}
			if token == "" {
				return nil, nil
			}
			return map[string]string{"GH_TOKEN": token}, nil
		},
	}
}
