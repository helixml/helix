package eventsource

import (
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

type Kind string

const (
	KindTrigger         Kind = "trigger"
	KindProcessorOutput Kind = "processor_output"
)

type SourceRef struct {
	Kind        Kind
	TriggerID   string
	ProcessorID processor.ProcessorID
	OutputID    string
}

func Trigger(id string) SourceRef { return SourceRef{Kind: KindTrigger, TriggerID: id} }
func ProcessorOutput(processorID processor.ProcessorID, outputID string) SourceRef {
	return SourceRef{Kind: KindProcessorOutput, ProcessorID: processorID, OutputID: outputID}
}

func (s SourceRef) Validate() error {
	switch s.Kind {
	case KindTrigger:
		if s.TriggerID == "" {
			return errors.New("trigger source id is empty")
		}
		if s.ProcessorID != "" || s.OutputID != "" {
			return errors.New("trigger source contains processor fields")
		}
	case KindProcessorOutput:
		if s.ProcessorID == "" {
			return errors.New("processor source processor id is empty")
		}
		if s.OutputID == "" {
			return errors.New("processor source output id is empty")
		}
		if s.TriggerID != "" {
			return errors.New("processor source contains trigger id")
		}
	default:
		return fmt.Errorf("unknown source kind %q", s.Kind)
	}
	return nil
}

type Event struct {
	ID                  string
	OrganizationID      string
	Source              SourceRef
	Message             streaming.Message
	OriginatingWorkerID string
	CreatedAt           time.Time
}

func NewEvent(id, orgID string, source SourceRef, message streaming.Message, originatingWorkerID string, createdAt time.Time) (Event, error) {
	if id == "" {
		return Event{}, errors.New("source event id is empty")
	}
	if orgID == "" {
		return Event{}, errors.New("source event organization id is empty")
	}
	if err := source.Validate(); err != nil {
		return Event{}, fmt.Errorf("source event: %w", err)
	}
	encoded, err := message.Encode()
	if err != nil {
		return Event{}, fmt.Errorf("source event message: %w", err)
	}
	if encoded == "{}" {
		return Event{}, errors.New("source event message is empty")
	}
	if createdAt.IsZero() {
		return Event{}, errors.New("source event created at is zero")
	}
	return Event{ID: id, OrganizationID: orgID, Source: source, Message: message, OriginatingWorkerID: originatingWorkerID, CreatedAt: createdAt.UTC()}, nil
}
