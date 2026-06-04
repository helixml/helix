// Tests for the generic per-activation secret-injection hook. Pins:
//
//   - SpawnSecretInjectorFunc adapter wires a plain function into the
//     interface without each transport having to declare a custom
//     type.
//   - The spawner iterates every registered SecretInjector on every
//     activation (not just the first), so re-issued tokens propagate
//     without re-hiring the Worker.
//   - Returning an empty / nil map is a soft skip — the spawner must
//     NOT shadow a previously-valid secret with "".
//   - An injector that returns an error must NOT take the whole
//     activation down — other injectors keep running, and the error
//     is logged for the operator.
//   - Multiple injectors compose: each transport (github, postmark,
//     custom) can plug its own secrets in without coordinating with
//     the others.
package helix

import (
	"context"
	"errors"
	"testing"
)

func TestSpawnSecretInjectorFunc_DelegatesToFn(t *testing.T) {
	t.Parallel()
	var gotOrgID string
	inj := SpawnSecretInjectorFunc{
		Label: "test",
		Fn: func(_ context.Context, orgID string) (map[string]string, error) {
			gotOrgID = orgID
			return map[string]string{"FOO": "bar"}, nil
		},
	}
	if inj.Name() != "test" {
		t.Errorf("Name() = %q, want test", inj.Name())
	}
	got, err := inj.InjectSecrets(context.Background(), "org-test")
	if err != nil {
		t.Fatalf("InjectSecrets: %v", err)
	}
	if gotOrgID != "org-test" {
		t.Errorf("Fn called with orgID = %q, want org-test", gotOrgID)
	}
	if got["FOO"] != "bar" {
		t.Errorf("got = %v, want FOO=bar", got)
	}
}

// TestSpawnSecretInjectorFunc_EmptyLabelStillUsable pins the
// degraded case: callers can omit Label and the adapter still works
// (Name returns ""). Real wiring should always set a label so
// logs are useful, but the runtime doesn't enforce it — keeps the
// type ergonomic in tests.
func TestSpawnSecretInjectorFunc_EmptyLabelStillUsable(t *testing.T) {
	t.Parallel()
	inj := SpawnSecretInjectorFunc{
		Fn: func(_ context.Context, _ string) (map[string]string, error) {
			return nil, nil
		},
	}
	if inj.Name() != "" {
		t.Errorf("Name() with empty Label = %q, want \"\"", inj.Name())
	}
	if _, err := inj.InjectSecrets(context.Background(), "org-test"); err != nil {
		t.Fatalf("InjectSecrets: %v", err)
	}
}

// TestSpawnSecretInjectorFunc_NilFnIsSafe pins the "operator didn't
// wire one of these" case — returns (nil, nil) without panicking.
// The spawner skips empty maps so an unwired injector contributes
// nothing.
func TestSpawnSecretInjectorFunc_NilFnIsSafe(t *testing.T) {
	t.Parallel()
	inj := SpawnSecretInjectorFunc{Label: "noop"}
	got, err := inj.InjectSecrets(context.Background(), "org-test")
	if err != nil {
		t.Fatalf("InjectSecrets: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %v, want empty", got)
	}
}

// stubInjector is a tiny test double for the SpawnSecretInjector
// interface — drives both happy and error paths in the iteration
// loop tests.
type stubInjector struct {
	name    string
	secrets map[string]string
	err     error
	calls   int
}

func (s *stubInjector) Name() string { return s.name }

func (s *stubInjector) InjectSecrets(_ context.Context, _ string) (map[string]string, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.secrets, nil
}

// TestRunSecretInjectors_AppliesEveryNonEmptyResult pins the
// happy-path composition: multiple injectors, each returning their
// own secret keys, all end up upserted via PutProjectSecret.
func TestRunSecretInjectors_AppliesEveryNonEmptyResult(t *testing.T) {
	t.Parallel()
	gh := &stubInjector{name: "github", secrets: map[string]string{"GH_TOKEN": "gho_abc"}}
	pm := &stubInjector{name: "postmark", secrets: map[string]string{"POSTMARK_TOKEN": "pm_xyz"}}

	puts := map[string]string{}
	cfg := SpawnerConfig{
		SecretInjectors: []SpawnSecretInjector{gh, pm},
	}
	cfg.runSecretInjectors(context.Background(), "org-test", "prj-1", func(_ context.Context, _ string, name, value string) error {
		puts[name] = value
		return nil
	})

	if gh.calls != 1 || pm.calls != 1 {
		t.Errorf("injector calls = (gh:%d, pm:%d), want both 1", gh.calls, pm.calls)
	}
	if puts["GH_TOKEN"] != "gho_abc" {
		t.Errorf("GH_TOKEN = %q, want gho_abc", puts["GH_TOKEN"])
	}
	if puts["POSTMARK_TOKEN"] != "pm_xyz" {
		t.Errorf("POSTMARK_TOKEN = %q, want pm_xyz", puts["POSTMARK_TOKEN"])
	}
}

// TestRunSecretInjectors_EmptyMapSkipsUpsert pins the soft-skip
// contract: an injector returning an empty map (e.g. no OAuth
// connection yet) must NOT cause the spawner to write an empty
// value, which would shadow a previously-valid secret in the
// sandbox container.
func TestRunSecretInjectors_EmptyMapSkipsUpsert(t *testing.T) {
	t.Parallel()
	noop := &stubInjector{name: "noop", secrets: nil}
	puts := map[string]string{}
	cfg := SpawnerConfig{SecretInjectors: []SpawnSecretInjector{noop}}
	cfg.runSecretInjectors(context.Background(), "org-test", "prj-1", func(_ context.Context, _ string, name, value string) error {
		puts[name] = value
		return nil
	})
	if len(puts) != 0 {
		t.Errorf("PutProjectSecret called %d times; want 0 for empty injector result", len(puts))
	}
}

// TestRunSecretInjectors_ErrorFromOneDoesNotBlockOthers pins the
// best-effort contract: one transport's resolver failing (e.g. the
// GitHub API is down) must NOT prevent other transports' secrets
// from being injected.
func TestRunSecretInjectors_ErrorFromOneDoesNotBlockOthers(t *testing.T) {
	t.Parallel()
	broken := &stubInjector{name: "github", err: errors.New("github API timeout")}
	working := &stubInjector{name: "postmark", secrets: map[string]string{"POSTMARK_TOKEN": "pm_xyz"}}

	puts := map[string]string{}
	cfg := SpawnerConfig{SecretInjectors: []SpawnSecretInjector{broken, working}}
	cfg.runSecretInjectors(context.Background(), "org-test", "prj-1", func(_ context.Context, _ string, name, value string) error {
		puts[name] = value
		return nil
	})

	if broken.calls != 1 || working.calls != 1 {
		t.Errorf("both injectors should be called; got broken=%d working=%d", broken.calls, working.calls)
	}
	if puts["POSTMARK_TOKEN"] != "pm_xyz" {
		t.Errorf("POSTMARK_TOKEN = %q, want pm_xyz (broken injector should not block working ones)", puts["POSTMARK_TOKEN"])
	}
}

// TestRunSecretInjectors_NilSliceIsNoop pins the zero-value path —
// SpawnerConfig built without SecretInjectors must not crash and
// must not call put.
func TestRunSecretInjectors_NilSliceIsNoop(t *testing.T) {
	t.Parallel()
	calls := 0
	cfg := SpawnerConfig{}
	cfg.runSecretInjectors(context.Background(), "org-test", "prj-1", func(_ context.Context, _ string, _, _ string) error {
		calls++
		return nil
	})
	if calls != 0 {
		t.Errorf("nil SecretInjectors should produce 0 put calls, got %d", calls)
	}
}
