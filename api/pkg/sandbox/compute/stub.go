package compute

import (
	"context"
	"errors"
	"sync"
	"time"
)

// StubProvider is an in-memory Provider implementation used by tests
// that need a Provider without standing up a real cloud account. It
// records calls, returns canned Handles, and never makes network
// requests.
//
// Useful for:
//   - Verifying the dispatcher / reconciler logic in isolation from
//     any real cloud provider.
//   - Driving end-to-end Helix tests where a "host came online" is
//     simulated rather than actually provisioned.
//   - Confirming at compile time that the Provider interface is
//     implementable (the `var _ Provider = (*StubProvider)(nil)` line
//     below is a static compliance check).
//
// The stub is goroutine-safe; the mutex covers handles + counters
// against concurrent calls.
type StubProvider struct {
	mu          sync.Mutex
	name        string
	nextID      int
	handles     map[string]*Handle
	provisionFn func(spec Spec, h *Handle)
}

// Compile-time check that StubProvider satisfies the Provider
// interface. If the interface drifts and StubProvider stops matching,
// this line forces a build failure rather than a runtime surprise.
var _ Provider = (*StubProvider)(nil)

// NewStubProvider returns an empty stub. name is what Name() will
// return; pass "" for the default "stub".
func NewStubProvider(name string) *StubProvider {
	if name == "" {
		name = "stub"
	}
	return &StubProvider{
		name:    name,
		handles: make(map[string]*Handle),
	}
}

// SetProvisionHook lets a test inject custom behaviour during
// Provision (e.g. immediately mark the handle as Ready to short-circuit
// the "wait for register" phase). The hook is called BEFORE the handle
// is stored.
func (s *StubProvider) SetProvisionHook(fn func(spec Spec, h *Handle)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.provisionFn = fn
}

// Name returns the configured provider name.
func (s *StubProvider) Name() string {
	return s.name
}

// Provision records the request and returns a Handle in
// StateProvisioning with a generated synthetic ProviderID.
func (s *StubProvider) Provision(_ context.Context, spec Spec) (*Handle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	h := &Handle{
		ProviderName: s.name,
		ProviderID:   stubID(s.name, s.nextID),
		State:        StateProvisioning,
		CreatedAt:    time.Now(),
		Metadata:     copyLabels(spec.Labels),
	}
	if s.provisionFn != nil {
		s.provisionFn(spec, h)
	}
	s.handles[h.ProviderID] = h
	return h, nil
}

// Deprovision marks the handle StateTerminated. Returns nil if the
// handle is already terminated or not known (idempotent per the
// interface contract).
func (s *StubProvider) Deprovision(_ context.Context, handle *Handle, _ DeprovisionOpts) error {
	if handle == nil {
		return errors.New("sandbox/compute/stub: Deprovision called with nil handle")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if h, ok := s.handles[handle.ProviderID]; ok {
		h.State = StateTerminated
	}
	handle.State = StateTerminated
	return nil
}

// List returns a snapshot of currently-tracked handles.
func (s *StubProvider) List(_ context.Context) ([]*Handle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Handle, 0, len(s.handles))
	for _, h := range s.handles {
		// Return shallow copies so callers can't mutate our state.
		cp := *h
		out = append(out, &cp)
	}
	return out, nil
}

// HealthCheck looks up the handle by ProviderID and copies the
// current State onto the caller's handle pointer. Returns an error
// if the handle is unknown or in StateFailed / StateTerminated.
func (s *StubProvider) HealthCheck(_ context.Context, handle *Handle) error {
	if handle == nil {
		return errors.New("sandbox/compute/stub: HealthCheck called with nil handle")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.handles[handle.ProviderID]
	if !ok {
		return errors.New("sandbox/compute/stub: handle not found")
	}
	handle.State = stored.State
	if stored.State == StateFailed {
		return errors.New("sandbox/compute/stub: handle in StateFailed")
	}
	if stored.State == StateTerminated {
		return errors.New("sandbox/compute/stub: handle in StateTerminated")
	}
	return nil
}

func stubID(name string, n int) string {
	return name + "-handle-" + itoa(n)
}

func itoa(n int) string {
	// Avoid pulling strconv into the package surface for just this.
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func copyLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
