package helix

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/types"
)

func TestEffectiveLaunchConfigResolvesBotThenOrgThenGlobal(t *testing.T) {
	t.Parallel()
	base, _ := orgchart.NewNode("w-x", "# Role", nil, time.Now().UTC(), "org-test")

	got := EffectiveLaunchConfig(base, "", nil)
	if got.SandboxRuntime != types.SandboxRuntimeUbuntuDesktop {
		t.Fatalf("global default runtime = %q, want ubuntu-desktop", got.SandboxRuntime)
	}
	if std := types.EffectiveSpecTaskSandboxResources(nil); got.SandboxResources != std {
		t.Fatalf("global default resources = %+v, want %+v", got.SandboxResources, std)
	}

	got = EffectiveLaunchConfig(base, types.SandboxRuntimeHeadlessUbuntu, &types.SandboxResourceOverrides{VCPUs: 8, MemoryMB: 16384})
	if got.SandboxRuntime != types.SandboxRuntimeHeadlessUbuntu || got.SandboxResources.VCPUs != 8 {
		t.Fatalf("org default not applied: %+v", got)
	}

	bot := base.WithSandboxRuntime(string(types.SandboxRuntimeUbuntuDesktop)).WithSandboxResources(4, 8192)
	got = EffectiveLaunchConfig(bot, types.SandboxRuntimeHeadlessUbuntu, &types.SandboxResourceOverrides{VCPUs: 8, MemoryMB: 16384})
	if got.SandboxRuntime != types.SandboxRuntimeUbuntuDesktop || got.SandboxResources.VCPUs != 4 || got.SandboxResources.MemoryMB != 8192 {
		t.Fatalf("bot config must win over org default: %+v", got)
	}
}

func TestSpawnerStampsBotLaunchConfigOnFreshSession(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	bot, err := s.Nodes.Get(context.Background(), "org-test", wid)
	if err != nil {
		t.Fatalf("get bot: %v", err)
	}
	bot = bot.WithSandboxRuntime(string(types.SandboxRuntimeHeadlessUbuntu)).WithSandboxResources(4, 8192)
	if err := s.Nodes.Update(context.Background(), bot); err != nil {
		t.Fatalf("update bot: %v", err)
	}
	fc := &fakeHelixClient{
		startSessionID: "ses_new",
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	sp := Spawner(newHelixCfg(t, fc, s))
	if err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerHire}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	fc.mu.Lock()
	launch := fc.lastStartParams.Launch
	fc.mu.Unlock()
	if launch.SandboxRuntime != types.SandboxRuntimeHeadlessUbuntu {
		t.Fatalf("fresh session runtime = %q, want headless-ubuntu", launch.SandboxRuntime)
	}
	if launch.SandboxResources.VCPUs != 4 || launch.SandboxResources.MemoryMB != 8192 {
		t.Fatalf("fresh session resources = %+v, want 4/8192", launch.SandboxResources)
	}
}

func TestSpawnerSyncsChangedLaunchConfigOntoExistingSession(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_new",
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	cfg := newHelixCfg(t, fc, s)
	// Org default is headless at 8 cores; the bot has no config of its own.
	cfg.SandboxRuntime = types.SandboxRuntimeHeadlessUbuntu
	cfg.SandboxResources = &types.SandboxResourceOverrides{VCPUs: 8, MemoryMB: 16384}
	sp := Spawner(cfg)
	if err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerHire}}); err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	fc.mu.Lock()
	first := fc.lastStartParams.Launch
	fc.mu.Unlock()
	if first.SandboxRuntime != types.SandboxRuntimeHeadlessUbuntu || first.SandboxResources.VCPUs != 8 {
		t.Fatalf("org default not inherited on first start: %+v", first)
	}

	// Operator switches this bot to a 12-core desktop. The next activation
	// must push that onto the existing session before anything restarts it.
	bot, err := s.Nodes.Get(context.Background(), "org-test", wid)
	if err != nil {
		t.Fatalf("get bot: %v", err)
	}
	bot = bot.WithSandboxRuntime(string(types.SandboxRuntimeUbuntuDesktop)).WithSandboxResources(12, 24576)
	if err := s.Nodes.Update(context.Background(), bot); err != nil {
		t.Fatalf("update bot: %v", err)
	}
	if err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerEvent, EventID: "e-1"}}); err != nil {
		t.Fatalf("spawn 2: %v", err)
	}
	fc.mu.Lock()
	synced := fc.lastSyncLaunch
	fc.mu.Unlock()
	if synced.SandboxRuntime != types.SandboxRuntimeUbuntuDesktop || synced.SandboxResources.VCPUs != 12 || synced.SandboxResources.MemoryMB != 24576 {
		t.Fatalf("existing session not synced with new bot config: %+v", synced)
	}
}

func TestSpawnerHeadlessBotIgnoresDesktopQuota(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	bot, err := s.Nodes.Get(context.Background(), "org-test", wid)
	if err != nil {
		t.Fatalf("get bot: %v", err)
	}
	if err := s.Nodes.Update(context.Background(), bot.WithSandboxRuntime(string(types.SandboxRuntimeHeadlessUbuntu))); err != nil {
		t.Fatalf("update bot: %v", err)
	}
	fc := &quotaFullFakeClient{
		fakeHelixClient: fakeHelixClient{
			startSessionID: "ses_headless",
			outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
		},
	}
	cfg := newHelixCfg(t, &fc.fakeHelixClient, s)
	cfg.Client = fc
	sp := Spawner(cfg)
	if err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerHire}}); err != nil {
		t.Fatalf("headless bot must not be gated by the desktop quota: %v", err)
	}
	if got := atomic.LoadInt32(&fc.startCalls); got != 1 {
		t.Fatalf("StartSession calls = %d, want 1", got)
	}
}
