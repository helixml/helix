package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// ControllerSuite covers the thin delegation methods (Get/List/Update),
// HydraClient routing, ReapExpired iteration, ensureSandboxAPIToken's two
// branches, and resolveImage. The fatter Create / billing / placement paths
// have their own dedicated suites.
type ControllerSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	store      *store.MockStore
	connman    *connman.ConnectionManager
	controller *Controller
	ctx        context.Context
}

func TestControllerSuite(t *testing.T) {
	suite.Run(t, new(ControllerSuite))
}

func (s *ControllerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.connman = connman.New()
	s.ctx = context.Background()

	runtimes, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes:       "headless-ubuntu=ubuntu:22.04|sleep infinity,node22=node:22-bookworm-slim",
		DefaultRuntime: "headless-ubuntu",
	})
	s.Require().NoError(err)
	s.controller = New(s.store, s.connman, runtimes, "https://api.example.com", "")
}

func (s *ControllerSuite) TearDownTest() {
	s.connman.Stop()
	s.ctrl.Finish()
}

func (s *ControllerSuite) TestNewDefaultsWorkspaceDir() {
	c := New(s.store, nil, s.controller.runtimes, "", "")
	s.Require().Equal("/data/sandboxes", c.workspaceDir)
}

func (s *ControllerSuite) TestRuntimesReturnsRegistry() {
	s.Require().Same(s.controller.runtimes, s.controller.Runtimes())
}

func (s *ControllerSuite) TestGetDelegatesToStore() {
	expected := &types.Sandbox{ID: "sbx_1"}
	s.store.EXPECT().GetSandbox(s.ctx, "sbx_1").Return(expected, nil)

	got, err := s.controller.Get(s.ctx, "sbx_1")
	s.Require().NoError(err)
	s.Require().Same(expected, got)
}

func (s *ControllerSuite) TestListPassesOrgAndProjectQuery() {
	expected := []*types.Sandbox{{ID: "sbx_1"}, {ID: "sbx_2"}}
	s.store.EXPECT().ListSandboxes(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, q *store.ListSandboxesQuery) ([]*types.Sandbox, error) {
			s.Require().Equal("org_1", q.OrganizationID)
			s.Require().Equal("prj_1", q.ProjectID)
			return expected, nil
		},
	)

	got, err := s.controller.List(s.ctx, "org_1", "prj_1")
	s.Require().NoError(err)
	s.Require().Equal(expected, got)
}

func (s *ControllerSuite) TestUpdateAppliesNameTagsAndExtendsExpiry() {
	createdAt := time.Now().Add(-30 * time.Minute)
	sb := &types.Sandbox{
		ID:             "sbx_update",
		Name:           "old",
		TimeoutSeconds: 600,
		CreatedAt:      createdAt,
	}
	s.store.EXPECT().GetSandbox(s.ctx, "sbx_update").Return(sb, nil)
	s.store.EXPECT().UpdateSandbox(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, in *types.Sandbox) (*types.Sandbox, error) {
			return in, nil
		},
	)

	newName := "new-name"
	newTTL := 7200
	tags := map[string]string{"team": "platform"}
	out, err := s.controller.Update(s.ctx, "sbx_update", &types.UpdateSandboxRequest{
		Name:           &newName,
		TimeoutSeconds: &newTTL,
		Tags:           &tags,
	})
	s.Require().NoError(err)
	s.Require().Equal("new-name", out.Name)
	s.Require().Equal(7200, out.TimeoutSeconds)
	s.Require().NotNil(out.ExpiresAt)
	s.Require().WithinDuration(createdAt.Add(2*time.Hour), *out.ExpiresAt, time.Second)

	var decoded map[string]string
	s.Require().NoError(json.Unmarshal(out.Tags, &decoded))
	s.Require().Equal("platform", decoded["team"])
}

func (s *ControllerSuite) TestUpdateNilRequestReturnsRowUnchanged() {
	sb := &types.Sandbox{ID: "sbx_noop", Name: "keep"}
	s.store.EXPECT().GetSandbox(s.ctx, "sbx_noop").Return(sb, nil)

	out, err := s.controller.Update(s.ctx, "sbx_noop", nil)
	s.Require().NoError(err)
	s.Require().Same(sb, out)
}

func (s *ControllerSuite) TestUpdatePropagatesGetError() {
	s.store.EXPECT().GetSandbox(s.ctx, "sbx_missing").Return(nil, store.ErrNotFound)
	_, err := s.controller.Update(s.ctx, "sbx_missing", &types.UpdateSandboxRequest{})
	s.Require().ErrorIs(err, store.ErrNotFound)
}

func (s *ControllerSuite) TestHydraClientErrorsWhenNoHostBound() {
	_, err := s.controller.HydraClient(&types.Sandbox{ID: "sbx_no_host", Status: types.SandboxStatusPending})
	s.Require().Error(err)
	s.Require().Contains(err.Error(), "no host assigned")
}

func (s *ControllerSuite) TestHydraClientReturnsNonNilWhenHostBound() {
	c, err := s.controller.HydraClient(&types.Sandbox{ID: "sbx_bound", HostDeviceID: "host-a"})
	s.Require().NoError(err)
	s.Require().NotNil(c)
}

func (s *ControllerSuite) TestReapExpiredDeletesEachExpiredSandbox() {
	expired := []*types.Sandbox{
		{ID: "sbx_a", OrganizationID: "org_1"},
		{ID: "sbx_b", OrganizationID: "org_2"},
	}
	s.store.EXPECT().ListExpiredSandboxes(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, now time.Time) ([]*types.Sandbox, error) {
			s.Require().WithinDuration(time.Now(), now, time.Second)
			return expired, nil
		},
	)

	// Delete() path for each: GetSandbox + billing settings (disabled) +
	// SetSandboxStatus(stopping) + GetAPIKey(not found) + DeleteSandbox.
	// Status != running so billSandboxFinal short-circuits without hitting
	// GetSystemSettings.
	for _, sb := range expired {
		s.store.EXPECT().GetSandbox(s.ctx, sb.ID).Return(sb, nil)
		s.store.EXPECT().SetSandboxStatus(s.ctx, sb.ID, types.SandboxStatusStopping, "").Return(nil)
		s.store.EXPECT().GetAPIKey(s.ctx, gomock.Any()).Return(nil, store.ErrNotFound)
		s.store.EXPECT().DeleteSandbox(s.ctx, sb.ID).Return(nil)
	}

	s.Require().NoError(s.controller.ReapExpired(s.ctx))
}

func (s *ControllerSuite) TestReapExpiredPropagatesListError() {
	s.store.EXPECT().ListExpiredSandboxes(s.ctx, gomock.Any()).Return(nil, errors.New("db down"))
	err := s.controller.ReapExpired(s.ctx)
	s.Require().Error(err)
	s.Require().Contains(err.Error(), "db down")
}

func (s *ControllerSuite) TestReapExpiredContinuesAfterPerSandboxFailure() {
	expired := []*types.Sandbox{
		{ID: "sbx_fail"},
		{ID: "sbx_ok"},
	}
	s.store.EXPECT().ListExpiredSandboxes(s.ctx, gomock.Any()).Return(expired, nil)

	// First sandbox fails on GetSandbox (logged + skipped), second succeeds.
	s.store.EXPECT().GetSandbox(s.ctx, "sbx_fail").Return(nil, errors.New("vanished"))
	s.store.EXPECT().GetSandbox(s.ctx, "sbx_ok").Return(expired[1], nil)
	s.store.EXPECT().SetSandboxStatus(s.ctx, "sbx_ok", types.SandboxStatusStopping, "").Return(nil)
	s.store.EXPECT().GetAPIKey(s.ctx, gomock.Any()).Return(nil, store.ErrNotFound)
	s.store.EXPECT().DeleteSandbox(s.ctx, "sbx_ok").Return(nil)

	s.Require().NoError(s.controller.ReapExpired(s.ctx))
}

func (s *ControllerSuite) TestEnsureSandboxAPITokenReturnsExistingKey() {
	sb := &types.Sandbox{ID: "sbx_token", OrganizationID: "org_1", Owner: "user_1"}
	existing := &types.ApiKey{Key: "preexisting-token"}
	s.store.EXPECT().GetAPIKey(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, q *types.ApiKey) (*types.ApiKey, error) {
			s.Require().Equal("sbx_token", q.SessionID)
			s.Require().Equal("user_1", q.Owner)
			return existing, nil
		},
	)

	tok, err := s.controller.ensureSandboxAPIToken(s.ctx, sb)
	s.Require().NoError(err)
	s.Require().Equal("preexisting-token", tok)
}

func (s *ControllerSuite) TestEnsureSandboxAPITokenMintsNewKeyWhenNoneExists() {
	sb := &types.Sandbox{ID: "sbx_new", OrganizationID: "org_1", Owner: "user_1"}
	s.store.EXPECT().GetAPIKey(s.ctx, gomock.Any()).Return(nil, store.ErrNotFound)
	s.store.EXPECT().CreateAPIKey(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.ApiKey) (*types.ApiKey, error) {
			s.Require().Equal("sbx_new", k.SessionID)
			s.Require().Equal(types.APIkeytypeAPI, k.Type)
			s.Require().Equal(types.OwnerTypeUser, k.OwnerType)
			s.Require().NotEmpty(k.Key)
			return k, nil
		},
	)

	tok, err := s.controller.ensureSandboxAPIToken(s.ctx, sb)
	s.Require().NoError(err)
	s.Require().NotEmpty(tok)
}

func (s *ControllerSuite) TestEnsureSandboxAPITokenPropagatesUnexpectedGetError() {
	sb := &types.Sandbox{ID: "sbx_err", OrganizationID: "org_1", Owner: "user_1"}
	s.store.EXPECT().GetAPIKey(s.ctx, gomock.Any()).Return(nil, errors.New("db boom"))
	_, err := s.controller.ensureSandboxAPIToken(s.ctx, sb)
	s.Require().Error(err)
}

func TestResolveImageReadsHeartbeatBlob(t *testing.T) {
	versions, _ := json.Marshal(map[string]string{"ubuntu": "abc123"})
	host := &types.SandboxInstance{ID: "host-a", DesktopVersions: versions}

	img, err := resolveImage(host, "helix-ubuntu", "ubuntu")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img != "helix-ubuntu:abc123" {
		t.Fatalf("got %q, want helix-ubuntu:abc123", img)
	}
}

func TestResolveImageMissingVersionKey(t *testing.T) {
	versions, _ := json.Marshal(map[string]string{"sway": "abc123"})
	host := &types.SandboxInstance{ID: "host-a", DesktopVersions: versions}

	if _, err := resolveImage(host, "helix-ubuntu", "ubuntu"); err == nil {
		t.Fatal("expected error when host doesn't advertise the requested version")
	}
}

func TestResolveImageInvalidJSON(t *testing.T) {
	host := &types.SandboxInstance{ID: "host-a", DesktopVersions: []byte("{not json")}
	if _, err := resolveImage(host, "helix-ubuntu", "ubuntu"); err == nil {
		t.Fatal("expected parse error on malformed desktop_versions")
	}
}

func TestRuntimeRegistryDefaultRuntimeName(t *testing.T) {
	r, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes:       "headless-ubuntu=ubuntu:22.04",
		DefaultRuntime: "headless-ubuntu",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if got := r.DefaultRuntimeName(); got != "headless-ubuntu" {
		t.Fatalf("got %q, want headless-ubuntu", got)
	}
}

func TestRuntimeRegistryNamesIncludesAllRegistered(t *testing.T) {
	r, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes:       "headless-ubuntu=ubuntu:22.04,node22=node:22-bookworm-slim",
		DefaultRuntime: "headless-ubuntu",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	got := r.Names()
	sort.Strings(got)
	// builtinDesktop + the two CSV entries.
	want := []string{"headless-ubuntu", "node22", string(types.SandboxRuntimeUbuntuDesktop)}
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestRuntimeRegistryFallsBackToFirstHeadlessRuntime(t *testing.T) {
	// No DefaultRuntime set — registry should auto-pick the first headless
	// runtime so the operator doesn't trip over a missing config.
	r, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes: "node22=node:22-bookworm-slim",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if r.DefaultRuntimeName() != "node22" {
		t.Fatalf("got %q, want node22", r.DefaultRuntimeName())
	}
}

func TestRuntimeRegistryRejectsMalformedEntry(t *testing.T) {
	_, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes:       "no-equals-sign",
		DefaultRuntime: "no-equals-sign",
	})
	if err == nil {
		t.Fatal("expected error on malformed runtime entry")
	}
}

func TestRuntimeRegistryRejectsUnknownDefaultRuntime(t *testing.T) {
	_, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes:       "node22=node:22-bookworm-slim",
		DefaultRuntime: "python313",
	})
	if err == nil {
		t.Fatal("expected error when default runtime isn't registered")
	}
}

func TestRuntimeRegistryResolveCustomImageRequiresGate(t *testing.T) {
	r, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes:       "headless-ubuntu=ubuntu:22.04",
		DefaultRuntime: "headless-ubuntu",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := r.Resolve(&types.CreateSandboxRequest{Image: "alpine:3.19"}); err == nil {
		t.Fatal("expected error when AllowCustomImage is false")
	}
}

func TestRuntimeRegistryResolveCustomImageRejectsRuntimeAndImageTogether(t *testing.T) {
	r, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes:         "headless-ubuntu=ubuntu:22.04",
		DefaultRuntime:   "headless-ubuntu",
		AllowCustomImage: true,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err = r.Resolve(&types.CreateSandboxRequest{
		Image:   "alpine:3.19",
		Runtime: types.SandboxRuntimeHeadlessUbuntu,
	})
	if err == nil {
		t.Fatal("expected error when both runtime and image are set")
	}
}

func TestRuntimeRegistryResolveCustomImageBuildsAdHocSpec(t *testing.T) {
	r, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes:         "headless-ubuntu=ubuntu:22.04",
		DefaultRuntime:   "headless-ubuntu",
		AllowCustomImage: true,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	spec, err := r.Resolve(&types.CreateSandboxRequest{Image: "alpine:3.19"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Name != "custom" {
		t.Fatalf("got Name=%q, want custom", spec.Name)
	}
	if spec.Image != "alpine:3.19" {
		t.Fatalf("got Image=%q, want alpine:3.19", spec.Image)
	}
}

func TestRuntimeRegistryResolveUnknownRuntime(t *testing.T) {
	r, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes:       "headless-ubuntu=ubuntu:22.04",
		DefaultRuntime: "headless-ubuntu",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := r.Resolve(&types.CreateSandboxRequest{Runtime: types.SandboxRuntime("ghost")}); err == nil {
		t.Fatal("expected error for unknown runtime")
	}
}

func TestInstanceAdvertisesVersion(t *testing.T) {
	tests := []struct {
		name     string
		instance *types.SandboxInstance
		key      string
		want     bool
	}{
		{
			name:     "empty blob",
			instance: &types.SandboxInstance{},
			key:      "ubuntu",
			want:     false,
		},
		{
			name:     "malformed json",
			instance: &types.SandboxInstance{DesktopVersions: []byte("{")},
			key:      "ubuntu",
			want:     false,
		},
		{
			name: "version present",
			instance: &types.SandboxInstance{
				DesktopVersions: mustMarshal(map[string]string{"ubuntu": "abc"}),
			},
			key:  "ubuntu",
			want: true,
		},
		{
			name: "version is empty string",
			instance: &types.SandboxInstance{
				DesktopVersions: mustMarshal(map[string]string{"ubuntu": ""}),
			},
			key:  "ubuntu",
			want: false,
		},
		{
			name: "different key",
			instance: &types.SandboxInstance{
				DesktopVersions: mustMarshal(map[string]string{"sway": "abc"}),
			},
			key:  "ubuntu",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := instanceAdvertisesVersion(tt.instance, tt.key); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
