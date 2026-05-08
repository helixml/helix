package profile

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// fakeDB is a tiny in-memory implementation of profile.DB for tests. We
// don't pull in the full gomocked store.MockStore — these tests only care
// about the parse-on-save behaviour, not generic CRUD plumbing.
type fakeDB struct {
	mu       sync.Mutex
	byID     map[string]*types.RunnerProfile
	byName   map[string]string // name -> id
}

func newFakeDB() *fakeDB {
	return &fakeDB{
		byID:   map[string]*types.RunnerProfile{},
		byName: map[string]string{},
	}
}

func (f *fakeDB) CreateRunnerProfile(_ context.Context, p *types.RunnerProfile) (*types.RunnerProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, dup := f.byName[p.Name]; dup {
		return nil, errors.New("name already exists")
	}
	cp := *p
	f.byID[p.ID] = &cp
	f.byName[p.Name] = p.ID
	return &cp, nil
}

func (f *fakeDB) GetRunnerProfile(_ context.Context, id string) (*types.RunnerProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (f *fakeDB) GetRunnerProfileByName(_ context.Context, name string) (*types.RunnerProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.byName[name]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *f.byID[id]
	return &cp, nil
}

func (f *fakeDB) UpdateRunnerProfile(_ context.Context, p *types.RunnerProfile) (*types.RunnerProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.byID[p.ID]
	if !ok {
		return nil, store.ErrNotFound
	}
	if existing.Name != p.Name {
		delete(f.byName, existing.Name)
		f.byName[p.Name] = p.ID
	}
	cp := *p
	f.byID[p.ID] = &cp
	return &cp, nil
}

func (f *fakeDB) DeleteRunnerProfile(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.byID[id]
	if !ok {
		return store.ErrNotFound
	}
	delete(f.byID, id)
	delete(f.byName, p.Name)
	return nil
}

func (f *fakeDB) ListRunnerProfiles(_ context.Context) ([]*types.RunnerProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*types.RunnerProfile, 0, len(f.byID))
	for _, p := range f.byID {
		cp := *p
		out = append(out, &cp)
	}
	return out, nil
}

const tinyCompose = `
services:
  vllm-tiny:
    image: vllm/vllm-openai:latest
    container_name: vllm-tiny
    ports:
      - "127.0.0.1:8000:8000"
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
              capabilities: [gpu]
    command:
      - --model
      - Qwen/Qwen2.5-0.5B-Instruct
      - --served-model-name
      - qwen2.5-0.5b
`

func TestService_Create_DerivesModelsAndCount(t *testing.T) {
	svc := New(newFakeDB())
	p, err := svc.Create(context.Background(), SaveInput{
		Name:          "tiny",
		Description:   "a small one",
		ComposeYAML:   tinyCompose,
		Vendor:        types.GPUVendorNVIDIA,
		Architectures: []string{"hopper", "blackwell"},
		ModelMatch:    "^NVIDIA",
		MinVRAMBytes:  4 << 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == "" {
		t.Error("Create should generate an ID")
	}
	if !strings.HasPrefix(p.ID, "rprof_") {
		t.Errorf("ID prefix: got %q, want rprof_*", p.ID)
	}
	if len(p.Models) != 1 || p.Models[0].Name != "qwen2.5-0.5b" {
		t.Errorf("Models: got %+v", p.Models)
	}
	// Operator-declared fields stored verbatim.
	if p.GPURequirement.Vendor != types.GPUVendorNVIDIA {
		t.Errorf("vendor not stored: %+v", p.GPURequirement)
	}
	if len(p.GPURequirement.Architectures) != 2 || p.GPURequirement.Architectures[0] != "hopper" {
		t.Errorf("architectures not stored verbatim: %+v", p.GPURequirement.Architectures)
	}
	if p.GPURequirement.ModelMatch != "^NVIDIA" {
		t.Errorf("ModelMatch: got %q", p.GPURequirement.ModelMatch)
	}
	if p.GPURequirement.MinVRAMBytes != 4<<30 {
		t.Errorf("MinVRAMBytes: got %d", p.GPURequirement.MinVRAMBytes)
	}
	// Derived field — count from compose.
	if p.GPURequirement.Count != 1 {
		t.Errorf("Count: got %d, want 1", p.GPURequirement.Count)
	}
}

func TestService_Create_RejectsBadInput(t *testing.T) {
	svc := New(newFakeDB())
	cases := []SaveInput{
		{},                                                       // empty name + yaml
		{Name: "x"},                                              // missing yaml
		{Name: "x", ComposeYAML: tinyCompose, ID: "rprof_xxx"},   // ID set on Create
		{Name: "x", ComposeYAML: tinyCompose, Vendor: "intel"},   // bad vendor
		{Name: "x", ComposeYAML: "this: is: not: valid: at: all"},// invalid YAML
	}
	for i, in := range cases {
		_, err := svc.Create(context.Background(), in)
		if err == nil {
			t.Errorf("case %d: expected error, got nil", i)
		}
	}
}

func TestService_Update_RederivesAndPreservesID(t *testing.T) {
	db := newFakeDB()
	svc := New(db)
	created, err := svc.Create(context.Background(), SaveInput{Name: "x", ComposeYAML: tinyCompose})
	if err != nil {
		t.Fatal(err)
	}
	originalID := created.ID

	// Update with a new compose that has a different model name and 4 GPUs.
	bigCompose := `
services:
  big:
    image: vllm/vllm-openai:latest
    container_name: vllm-big
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0", "1", "2", "3"]
    command: ["--model", "x/y", "--served-model-name", "big-model", "--tensor-parallel-size", "4"]
`
	updated, err := svc.Update(context.Background(), SaveInput{
		ID:          originalID,
		Name:        "x",
		ComposeYAML: bigCompose,
		Vendor:      types.GPUVendorNVIDIA,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != originalID {
		t.Errorf("ID changed on update: got %q, want %q", updated.ID, originalID)
	}
	if updated.Models[0].Name != "big-model" {
		t.Errorf("Models not re-derived: got %+v", updated.Models)
	}
	if updated.GPURequirement.Count != 4 {
		t.Errorf("Count not re-derived: got %d, want 4", updated.GPURequirement.Count)
	}
}

func TestService_Update_RequiresExisting(t *testing.T) {
	svc := New(newFakeDB())
	_, err := svc.Update(context.Background(), SaveInput{ID: "rprof_nope", Name: "x", ComposeYAML: tinyCompose})
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestService_Update_RequiresID(t *testing.T) {
	svc := New(newFakeDB())
	_, err := svc.Update(context.Background(), SaveInput{Name: "x", ComposeYAML: tinyCompose})
	if err == nil {
		t.Error("expected error when Update called without ID")
	}
}

func TestService_GetListDelete(t *testing.T) {
	svc := New(newFakeDB())
	ctx := context.Background()
	p1, _ := svc.Create(ctx, SaveInput{Name: "alpha", ComposeYAML: tinyCompose})
	p2, _ := svc.Create(ctx, SaveInput{Name: "beta", ComposeYAML: tinyCompose})

	got, err := svc.Get(ctx, p1.ID)
	if err != nil || got.Name != "alpha" {
		t.Errorf("Get: got %+v err=%v", got, err)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("List: got %d, want 2", len(list))
	}

	if err := svc.Delete(ctx, p2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, p2.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("after delete: got %v, want ErrNotFound", err)
	}
}
