package eventsource_test

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/stretchr/testify/require"
)

func TestSourceRefValidation(t *testing.T) {
	valid := []eventsource.SourceRef{eventsource.Trigger("tr-1"), eventsource.ProcessorOutput("p-1", "po-1")}
	for _, src := range valid {
		require.NoError(t, src.Validate())
	}
	invalid := []eventsource.SourceRef{{}, {Kind: eventsource.KindTrigger}, {Kind: eventsource.KindTrigger, TriggerID: "tr", OutputID: "po"}, {Kind: eventsource.KindProcessorOutput, ProcessorID: "p"}, {Kind: eventsource.KindProcessorOutput, ProcessorID: "p", OutputID: "po", TriggerID: "tr"}}
	for _, src := range invalid {
		require.Error(t, src.Validate())
	}
}
func TestEventValidation(t *testing.T) {
	now := time.Now()
	src := eventsource.Trigger("tr-1")
	msg := streaming.Message{Body: "hello"}
	_, err := eventsource.NewEvent("e-1", "org-1", src, msg, "", now)
	require.NoError(t, err)
	tests := []struct {
		name, id, org string
		src           eventsource.SourceRef
		msg           streaming.Message
		at            time.Time
	}{{"id", "", "org-1", src, msg, now}, {"org", "e-1", "", src, msg, now}, {"source", "e-1", "org-1", eventsource.SourceRef{}, msg, now}, {"message", "e-1", "org-1", src, streaming.Message{}, now}, {"time", "e-1", "org-1", src, msg, time.Time{}}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := eventsource.NewEvent(tt.id, tt.org, tt.src, tt.msg, "", tt.at)
			require.Error(t, err)
		})
	}
}
