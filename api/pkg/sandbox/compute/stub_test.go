package compute

import (
	"context"
	"strings"
	"testing"
)

func TestStubProviderName(t *testing.T) {
	if got := NewStubProvider("").Name(); got != "stub" {
		t.Errorf("Name() default = %q, want %q", got, "stub")
	}
	if got := NewStubProvider("yellowdog-test").Name(); got != "yellowdog-test" {
		t.Errorf("Name() custom = %q, want %q", got, "yellowdog-test")
	}
}

func TestStubProvisionReturnsProvisioningHandle(t *testing.T) {
	s := NewStubProvider("test")
	h, err := s.Provision(context.Background(), Spec{GPUVendor: "nvidia"})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if h == nil {
		t.Fatal("Provision returned nil handle")
	}
	if h.State != StateProvisioning {
		t.Errorf("State = %q, want %q", h.State, StateProvisioning)
	}
	if h.ProviderName != "test" {
		t.Errorf("ProviderName = %q, want %q", h.ProviderName, "test")
	}
	if h.ProviderID == "" {
		t.Error("ProviderID is empty")
	}
	if h.SandboxID != "" {
		t.Error("SandboxID should be empty until the host registers")
	}
	if h.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set at Provision time")
	}
}

func TestStubProvisionAssignsUniqueIDs(t *testing.T) {
	s := NewStubProvider("test")
	h1, _ := s.Provision(context.Background(), Spec{})
	h2, _ := s.Provision(context.Background(), Spec{})
	if h1.ProviderID == h2.ProviderID {
		t.Errorf("ProviderIDs should differ, both got %q", h1.ProviderID)
	}
}

func TestStubProvisionHookCanOverrideState(t *testing.T) {
	s := NewStubProvider("test")
	s.SetProvisionHook(func(_ Spec, h *Handle) {
		h.State = StateReady
		h.SandboxID = "ses_synthetic_001"
	})
	h, _ := s.Provision(context.Background(), Spec{})
	if h.State != StateReady {
		t.Errorf("hook did not apply: State = %q, want %q", h.State, StateReady)
	}
	if h.SandboxID != "ses_synthetic_001" {
		t.Errorf("SandboxID = %q, want %q", h.SandboxID, "ses_synthetic_001")
	}
}

func TestStubDeprovisionMarksTerminated(t *testing.T) {
	s := NewStubProvider("test")
	h, _ := s.Provision(context.Background(), Spec{})
	if err := s.Deprovision(context.Background(), h, DeprovisionOpts{Reason: "test"}); err != nil {
		t.Fatalf("Deprovision: %v", err)
	}
	if h.State != StateTerminated {
		t.Errorf("State after Deprovision = %q, want %q", h.State, StateTerminated)
	}
}

func TestStubDeprovisionIsIdempotent(t *testing.T) {
	s := NewStubProvider("test")
	h, _ := s.Provision(context.Background(), Spec{})
	_ = s.Deprovision(context.Background(), h, DeprovisionOpts{})
	if err := s.Deprovision(context.Background(), h, DeprovisionOpts{}); err != nil {
		t.Errorf("second Deprovision should be idempotent; got %v", err)
	}
}

func TestStubDeprovisionRejectsNilHandle(t *testing.T) {
	s := NewStubProvider("test")
	if err := s.Deprovision(context.Background(), nil, DeprovisionOpts{}); err == nil {
		t.Error("Deprovision(nil) should error")
	}
}

func TestStubList(t *testing.T) {
	s := NewStubProvider("test")
	for i := 0; i < 3; i++ {
		if _, err := s.Provision(context.Background(), Spec{}); err != nil {
			t.Fatalf("Provision %d: %v", i, err)
		}
	}
	handles, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(handles) != 3 {
		t.Errorf("List len = %d, want 3", len(handles))
	}
}

func TestStubListReturnsCopies(t *testing.T) {
	// Mutating a returned handle must not change the stub's internal
	// state. Defensive contract for downstream consumers.
	s := NewStubProvider("test")
	_, _ = s.Provision(context.Background(), Spec{})
	handles, _ := s.List(context.Background())
	handles[0].State = StateFailed

	again, _ := s.List(context.Background())
	if again[0].State == StateFailed {
		t.Error("List should return defensive copies; internal state was mutated by caller")
	}
}

func TestStubHealthCheckOK(t *testing.T) {
	s := NewStubProvider("test")
	h, _ := s.Provision(context.Background(), Spec{})
	if err := s.HealthCheck(context.Background(), h); err != nil {
		t.Errorf("HealthCheck on provisioning handle = %v, want nil", err)
	}
}

func TestStubHealthCheckUnknownHandle(t *testing.T) {
	s := NewStubProvider("test")
	err := s.HealthCheck(context.Background(), &Handle{ProviderID: "does-not-exist"})
	if err == nil {
		t.Error("HealthCheck on unknown handle should error")
	}
}

func TestStubHealthCheckRejectsNilHandle(t *testing.T) {
	s := NewStubProvider("test")
	if err := s.HealthCheck(context.Background(), nil); err == nil {
		t.Error("HealthCheck(nil) should error")
	}
}

func TestStubHealthCheckAfterDeprovisionFails(t *testing.T) {
	s := NewStubProvider("test")
	h, _ := s.Provision(context.Background(), Spec{})
	_ = s.Deprovision(context.Background(), h, DeprovisionOpts{})

	err := s.HealthCheck(context.Background(), h)
	if err == nil {
		t.Error("HealthCheck on terminated handle should error")
	}
	if !strings.Contains(err.Error(), "Terminated") && !strings.Contains(err.Error(), "terminated") {
		t.Errorf("error should mention terminated state; got %q", err.Error())
	}
}

func TestStubLabelsCopiedIntoMetadata(t *testing.T) {
	s := NewStubProvider("test")
	h, _ := s.Provision(context.Background(), Spec{
		Labels: map[string]string{"region": "eu-west-2", "owner": "team-foo"},
	})
	if got := h.Metadata["region"]; got != "eu-west-2" {
		t.Errorf("Metadata[region] = %q, want %q", got, "eu-west-2")
	}
	if got := h.Metadata["owner"]; got != "team-foo" {
		t.Errorf("Metadata[owner] = %q, want %q", got, "team-foo")
	}
}
