package mcptools

import (
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// TestQuotedListRendersDomainEnums covers the helper used by every
// "valid: …" error. WorkerKind is gone (a Bot has no kind), so the only
// remaining string-enum domain surfaced through QuotedList is the
// transport kind — pin it so a future enum addition that breaks the
// generic constraint is caught here, not in a Slack-channel report.
func TestQuotedListRendersDomainEnums(t *testing.T) {
	t.Parallel()
	if got := orgchart.QuotedList(transport.KindValues()); !strings.Contains(got, `"local"`) || !strings.Contains(got, `"github"`) {
		t.Errorf("TransportKind QuotedList = %q (missing local or github)", got)
	}
}
