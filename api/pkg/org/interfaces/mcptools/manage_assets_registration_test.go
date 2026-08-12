package mcptools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func TestAssetManagementToolsRegisteredAndGrantedToOwner(t *testing.T) {
	t.Parallel()
	st := orgmemory.New()
	cfg := DefaultDeps(st)
	injectTestPublishing(&cfg)
	reg := NewRegistry()
	require.NoError(t, RegisterBuiltins(reg, cfg.Build()))

	owner := make(map[tool.Name]bool, len(OwnerBotTools()))
	for _, name := range OwnerBotTools() {
		owner[name] = true
	}
	base := make(map[tool.Name]bool, len(BaseReadTools))
	for _, name := range BaseReadTools {
		base[name] = true
	}
	for _, name := range AssetManagementTools {
		_, err := reg.Get(name)
		assert.NoError(t, err, "tool %q must be registered", name)
		assert.True(t, owner[name], "OwnerBotTools missing %q", name)
		assert.False(t, base[name], "asset management tool %q must not be universal", name)
	}
}
