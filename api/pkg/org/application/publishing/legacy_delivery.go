package publishing

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

var (
	ErrLegacyDeliveryNotApplicable  = errors.New("legacy delivery not applicable")
	ErrLegacyDeliveryWithoutReceipt = errors.New("legacy delivery has no synchronous receipt")
)

// LegacyDelivery is the single deletion-marked adapter that preserves
// external delivery for the Topic REST/MCP compatibility surface through PR 4.
// It is not part of the target Trigger/action model.
type LegacyDelivery struct {
	deliverers map[transport.Kind]LegacyDeliverer
}

func NewLegacyDelivery(deliverers map[transport.Kind]LegacyDeliverer) *LegacyDelivery {
	if deliverers == nil {
		deliverers = make(map[transport.Kind]LegacyDeliverer)
	}
	return &LegacyDelivery{deliverers: deliverers}
}

func (d *LegacyDelivery) Register(kind transport.Kind, deliverer LegacyDeliverer) {
	d.deliverers[kind] = deliverer
}

func (d *LegacyDelivery) Deliver(ctx context.Context, topic streaming.Topic, event streaming.Event, msg streaming.Message) (DeliveryReceipt, bool, error) {
	deliverer := d.deliverers[topic.Transport.Kind]
	if deliverer == nil {
		return DeliveryReceipt{}, false, nil
	}
	receipt, err := deliverer.Deliver(ctx, topic, event, msg)
	if errors.Is(err, ErrLegacyDeliveryNotApplicable) {
		return DeliveryReceipt{}, false, nil
	}
	if errors.Is(err, ErrLegacyDeliveryWithoutReceipt) {
		return DeliveryReceipt{}, false, nil
	}
	if err != nil {
		receipt.Status = "failed"
		if receipt.Provider == "" {
			receipt.Provider = string(topic.Transport.Kind)
		}
		receipt.Error = "do not retry publish: " + err.Error()
		return receipt, true, err
	}
	return receipt, true, nil
}
