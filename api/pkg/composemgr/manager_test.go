package composemgr

import (
	"strings"
	"testing"
)

func TestRewriteRegistry_DefaultRegistry(t *testing.T) {
	yaml := `
services:
  s:
    image: vllm/vllm-openai:latest
`
	got := rewriteRegistry(yaml, "mirror.local")
	if !strings.Contains(got, "image: mirror.local/vllm/vllm-openai:latest") {
		t.Errorf("expected rewrite to mirror.local; got:\n%s", got)
	}
}

func TestRewriteRegistry_ExplicitRegistry(t *testing.T) {
	yaml := `
services:
  s:
    image: ghcr.io/helixml/foo:v1
`
	got := rewriteRegistry(yaml, "mirror.local")
	if !strings.Contains(got, "image: mirror.local/helixml/foo:v1") {
		t.Errorf("expected leading ghcr.io stripped; got:\n%s", got)
	}
}

func TestRewriteRegistry_AlreadyPointedAtMirror(t *testing.T) {
	yaml := `
services:
  s:
    image: mirror.local/vllm/vllm-openai:latest
`
	got := rewriteRegistry(yaml, "mirror.local")
	// Should be unchanged.
	if !strings.Contains(got, "image: mirror.local/vllm/vllm-openai:latest") {
		t.Errorf("idempotent rewrite broke; got:\n%s", got)
	}
	if strings.Contains(got, "mirror.local/mirror.local/") {
		t.Errorf("double-mirrored image; got:\n%s", got)
	}
}

func TestRewriteRegistry_RegistryWithPort(t *testing.T) {
	yaml := `
services:
  s:
    image: registry.local:5000/foo:v1
`
	got := rewriteRegistry(yaml, "mirror.local")
	if !strings.Contains(got, "image: mirror.local/foo:v1") {
		t.Errorf("expected port-bearing registry stripped; got:\n%s", got)
	}
}

func TestRewriteRegistry_LocalhostRegistry(t *testing.T) {
	yaml := `
services:
  s:
    image: localhost/foo:v1
`
	got := rewriteRegistry(yaml, "mirror.local")
	if !strings.Contains(got, "image: mirror.local/foo:v1") {
		t.Errorf("expected localhost stripped; got:\n%s", got)
	}
}

func TestRewriteRegistry_PreservesIndentation(t *testing.T) {
	yaml := "services:\n  s:\n    image: vllm/vllm-openai:latest\n    container_name: vllm\n"
	got := rewriteRegistry(yaml, "mirror.local")
	if !strings.Contains(got, "    image: mirror.local/vllm/vllm-openai:latest\n    container_name: vllm") {
		t.Errorf("indentation lost; got:\n%s", got)
	}
}

func TestRewriteRegistry_MultipleServices(t *testing.T) {
	yaml := `
services:
  a:
    image: vllm/vllm-openai:latest
  b:
    image: ghcr.io/helixml/foo:v1
`
	got := rewriteRegistry(yaml, "mirror.local")
	if !strings.Contains(got, "mirror.local/vllm/vllm-openai:latest") || !strings.Contains(got, "mirror.local/helixml/foo:v1") {
		t.Errorf("expected both services rewritten; got:\n%s", got)
	}
}

func TestNew_AppliesDefaults(t *testing.T) {
	m := New(Options{})
	if m.opts.ConfigDir == "" || m.opts.DockerComposeBinary == "" {
		t.Errorf("New should fill defaults: %+v", m.opts)
	}
	if m.opts.ReadinessPollInterval == 0 || m.opts.ReadinessTimeout == 0 {
		t.Errorf("New should fill timeouts: %+v", m.opts)
	}
}

func TestSnapshot_DeepCopiesServiceHealth(t *testing.T) {
	m := New(Options{})
	m.mu.Lock()
	m.state.ServiceHealth = map[string]string{"a": "healthy"}
	m.mu.Unlock()

	snap := m.Snapshot()
	snap.ServiceHealth["a"] = "MUTATED"

	again := m.Snapshot()
	if again.ServiceHealth["a"] != "healthy" {
		t.Errorf("Snapshot should deep-copy ServiceHealth; got %s", again.ServiceHealth["a"])
	}
}
