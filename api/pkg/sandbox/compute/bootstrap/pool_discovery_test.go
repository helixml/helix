package bootstrap

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/sandbox/compute"
)

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
