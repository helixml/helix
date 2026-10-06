package attachment

import (
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/aggregate"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

type Attachment struct {
	aggregate.Aggregate
	WorkerID orgchart.NodeID
	Source   eventsource.SourceRef
}

func New(id, orgID string, workerID orgchart.NodeID, source eventsource.SourceRef, createdBy string, createdAt time.Time) (Attachment, error) {
	a, err := aggregate.New(id, orgID, createdBy, createdAt)
	if err != nil {
		return Attachment{}, fmt.Errorf("worker attachment: %w", err)
	}
	if workerID == "" {
		return Attachment{}, errors.New("worker attachment worker id is empty")
	}
	if err := source.Validate(); err != nil {
		return Attachment{}, fmt.Errorf("worker attachment: %w", err)
	}
	return Attachment{Aggregate: a, WorkerID: workerID, Source: source}, nil
}
