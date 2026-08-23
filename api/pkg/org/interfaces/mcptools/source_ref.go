package mcptools

import (
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
)

// ParseProcessorOutput turns the tool-facing "<processorId>:<outputId>"
// shorthand into a terminal source reference. The shorthand exists
// because a chat model reliably produces one string per branch but
// routinely garbles a nested object; list_processors prints branches in
// exactly this form so the value can be copied straight through.
func ParseProcessorOutput(raw string) (eventsource.SourceRef, error) {
	processorID, outputID, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok || strings.TrimSpace(processorID) == "" || strings.TrimSpace(outputID) == "" {
		return eventsource.SourceRef{}, fmt.Errorf("processor output %q must be \"<processorId>:<outputId>\"", raw)
	}
	return eventsource.ProcessorOutput(strings.TrimSpace(processorID), strings.TrimSpace(outputID)), nil
}
