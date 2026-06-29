package bootstrap

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/sandbox/compute"
	"github.com/helixml/helix/api/pkg/sandbox/compute/yellowdog"
)

func TestToDiscoveredPools(t *testing.T) {
	in := []yellowdog.NodePool{
		{WorkerTag: "worker-gpu", InstanceType: "g5.xlarge", NodeCount: 2},
		{WorkerTag: "worker-inf2", InstanceType: "inf2.8xlarge", NodeCount: 1},
	}
	got := toDiscoveredPools(in)
	want := []compute.DiscoveredPool{
		{Key: "worker-gpu|g5.xlarge", WorkerTag: "worker-gpu", InstanceType: "g5.xlarge", NodeCount: 2},
		{Key: "worker-inf2|inf2.8xlarge", WorkerTag: "worker-inf2", InstanceType: "inf2.8xlarge", NodeCount: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("len=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pool[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestPoolDeploymentTagInjective(t *testing.T) {
	// Two pool Keys that sanitise to the SAME readable string ("." and "_"
	// both -> "-") must still produce DISTINCT deployment tags - otherwise
	// the two Managers would share row ownership (provider.Name() collision)
	// and double-count each other's floor.
	a := poolDeploymentTag("helix-ns", "team.a|inf2.8xlarge")
	b := poolDeploymentTag("helix-ns", "team_a|inf2.8xlarge")
	if a == b {
		t.Fatalf("distinct pool keys collided on deployment tag: %q", a)
	}
	// Same key is stable (deterministic).
	if poolDeploymentTag("helix-ns", "team.a|inf2.8xlarge") != a {
		t.Fatal("poolDeploymentTag is not deterministic for the same key")
	}
	// Tag carries the base prefix.
	if got := poolDeploymentTag("base", "worker-gpu|g5.xlarge"); got[:5] != "base-" {
		t.Fatalf("tag %q does not start with the base prefix", got)
	}
}

func testFactory() *ydManagerFactory {
	return &ydManagerFactory{
		cfg: config.Compute{
			Provider:           "yellowdog",
			Floor:              1,
			ReconcileInterval:  30 * time.Second,
			HealthCheckTimeout: 10 * time.Second,
			Yellowdog: config.Yellowdog{
				APIKeyID: "k", APISecret: "s", Namespace: "ns",
			},
		},
		deploymentTagBase: "helix-ns",
		serverURL:         "https://helix.example.com",
		runnerToken:       "tok",
		sandboxImage:      "ghcr.io/helixml/helix-sandbox:test",
		maxSandboxes:      5,
		store:             nullStore{},
	}
}

func TestYDManagerFactoryClassifies(t *testing.T) {
	f := testFactory()
	// An nvidia pool builds a Manager.
	m, err := f.NewPoolManager(compute.DiscoveredPool{Key: "worker-gpu|g5.xlarge", WorkerTag: "worker-gpu", InstanceType: "g5.xlarge"})
	if err != nil {
		t.Fatalf("NewPoolManager(g5): %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil manager for g5")
	}
	// A neuron pool builds a Manager too.
	if _, err := f.NewPoolManager(compute.DiscoveredPool{Key: "worker-inf2|inf2.8xlarge", WorkerTag: "worker-inf2", InstanceType: "inf2.8xlarge"}); err != nil {
		t.Fatalf("NewPoolManager(inf2): %v", err)
	}
	// An unclassifiable instance type errors (the supervisor then skips it).
	if _, err := f.NewPoolManager(compute.DiscoveredPool{Key: "worker-x|t3.small", WorkerTag: "worker-x", InstanceType: "t3.small"}); err == nil {
		t.Fatal("expected error for unclassifiable instance type")
	}
}

func TestSanitizeTagSegment(t *testing.T) {
	cases := map[string]string{
		"worker-psamuel|inf2.8xlarge": "worker-psamuel-inf2-8xlarge",
		"worker-gpu|g5.xlarge":        "worker-gpu-g5-xlarge",
		"a_b c":                       "a-b-c",
	}
	for in, want := range cases {
		if got := sanitizeTagSegment(in); got != want {
			t.Errorf("sanitizeTagSegment(%q)=%q want %q", in, got, want)
		}
	}
}
