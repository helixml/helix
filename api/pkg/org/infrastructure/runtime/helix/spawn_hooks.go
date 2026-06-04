package helix

import "context"

// SpawnSecretInjector is the contract each transport (or any other
// per-activation extension) implements to push secrets into the
// Worker's per-Worker project at activation time.
//
// The spawner calls every registered injector after ensureProject
// and before ensureSession on every activation, so refreshed tokens
// / rotated keys propagate on the next desktop boot without
// re-hiring the Worker. Returned secrets are upserted via
// ProjectService.PutProjectSecret; helix's existing
// GetProjectSecretsAsEnvVars surfaces them as env vars on container
// start.
//
// The spawner is intentionally Kind-agnostic — it does NOT know
// about GitHub, Postmark, or any future transport. Transports
// expose constructors (e.g. github.NewSecretInjector(resolver))
// that build a concrete SpawnSecretInjector; the host wires a
// []SpawnSecretInjector slice onto SpawnerConfig.
//
// Contract:
//   - Returning an empty / nil map is a SOFT SKIP — the spawner
//     does not call PutProjectSecret. Important for the "operator
//     hasn't connected this transport's auth yet" path so we don't
//     shadow a previously-valid secret with "".
//   - An injector that returns an error must NOT take the
//     activation down. The spawner logs the error and proceeds —
//     other injectors and the rest of the activation continue.
//   - Resolved on every activation (not at hire) so re-issued
//     tokens propagate without re-hiring the Worker.
type SpawnSecretInjector interface {
	// Name returns a stable identifier used by the spawner's
	// structured logs ("injector=github", "injector=postmark", …).
	// Returning "" is allowed but loses log clarity — production
	// transports should always set a label.
	Name() string

	// InjectSecrets returns the map of {secret_name: value} the
	// spawner should upsert on the Worker's project this
	// activation. Empty map = skip.
	InjectSecrets(ctx context.Context, orgID string) (map[string]string, error)
}

// SpawnSecretInjectorFunc adapts a plain function into the
// SpawnSecretInjector interface so transports (and tests) don't
// have to declare a custom type per injector. Used by the host
// when bridging a transport's existing resolver / loader into the
// spawner's hook surface.
//
//	helix.SpawnSecretInjectorFunc{
//	    Label: "github",
//	    Fn: func(ctx context.Context, orgID string) (map[string]string, error) {
//	        token, err := ghResolver(ctx, orgID)
//	        if err != nil { return nil, err }
//	        if token == "" { return nil, nil }
//	        return map[string]string{"GH_TOKEN": token}, nil
//	    },
//	}
type SpawnSecretInjectorFunc struct {
	Label string
	Fn    func(ctx context.Context, orgID string) (map[string]string, error)
}

func (f SpawnSecretInjectorFunc) Name() string { return f.Label }

func (f SpawnSecretInjectorFunc) InjectSecrets(ctx context.Context, orgID string) (map[string]string, error) {
	if f.Fn == nil {
		return nil, nil
	}
	return f.Fn(ctx, orgID)
}

// putSecretFn is the narrow callback runSecretInjectors uses to
// upsert a single project secret. Pulled out so the loop can be
// unit-tested without standing up a full ProjectService — the
// production wiring just adapts ProjectService.PutProjectSecret
// into this shape.
type putSecretFn func(ctx context.Context, projectID, name, value string) error

// runSecretInjectors invokes every registered injector and upserts
// the returned secrets via `put`. Empty maps soft-skip; errors are
// best-effort (logged via cfg.Logger, never propagated up the
// activation stack). Logging is intentional here — the activation
// must keep going even if a transport's auth is temporarily down.
//
// Method receiver, not function, so the spawner can call
// `cfg.runSecretInjectors(...)` without re-passing the logger and
// (in production) the project ID lookup.
func (c SpawnerConfig) runSecretInjectors(ctx context.Context, orgID, projectID string, put putSecretFn) {
	for _, inj := range c.SecretInjectors {
		if inj == nil {
			continue
		}
		secrets, err := inj.InjectSecrets(ctx, orgID)
		if err != nil {
			if c.Logger != nil {
				c.Logger.Warn("helix spawner: inject secrets failed",
					"injector", inj.Name(), "org", orgID, "err", err)
			}
			continue
		}
		if len(secrets) == 0 {
			continue
		}
		for name, value := range secrets {
			if err := put(ctx, projectID, name, value); err != nil {
				if c.Logger != nil {
					c.Logger.Warn("helix spawner: put project secret failed",
						"injector", inj.Name(), "secret", name, "project", projectID, "err", err)
				}
				continue
			}
		}
	}
}
