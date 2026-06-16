package types

import "testing"

func TestSecretScopeValid(t *testing.T) {
	valid := []SecretScope{SecretScopeDev, SecretScopeProd, SecretScopeBoth}
	for _, s := range valid {
		if !s.Valid() {
			t.Errorf("expected scope %q to be valid", s)
		}
	}
	for _, s := range []SecretScope{"", "staging", "DEV"} {
		if s.Valid() {
			t.Errorf("expected scope %q to be invalid", s)
		}
	}
}

func TestSecretScopeAppliesTo(t *testing.T) {
	cases := []struct {
		scope  SecretScope
		target SecretScope
		want   bool
	}{
		// dev-scoped secret
		{SecretScopeDev, SecretScopeDev, true},
		{SecretScopeDev, SecretScopeProd, false},
		// prod-scoped secret
		{SecretScopeProd, SecretScopeProd, true},
		{SecretScopeProd, SecretScopeDev, false},
		// both-scoped secret applies everywhere
		{SecretScopeBoth, SecretScopeDev, true},
		{SecretScopeBoth, SecretScopeProd, true},
	}
	for _, c := range cases {
		if got := c.scope.AppliesTo(c.target); got != c.want {
			t.Errorf("SecretScope(%q).AppliesTo(%q) = %v, want %v", c.scope, c.target, got, c.want)
		}
	}
}
