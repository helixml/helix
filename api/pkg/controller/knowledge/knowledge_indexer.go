package knowledge

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

func (r *Reconciler) index(ctx context.Context) error {
	data, err := r.store.ListKnowledge(ctx, &store.ListKnowledgeQuery{
		State: types.KnowledgeStatePending,
	})
	if err != nil {
		return fmt.Errorf("failed to get knowledge entries, error: %w", err)
	}

	for _, k := range data {
		r.wg.Add(1)

		go func(knowledge *types.Knowledge) {
			log.
				Info().
				Str("knowledge_id", knowledge.ID).
				Msg("indexing knowledge")

			err := r.indexKnowledge(ctx, knowledge)
			if err != nil {
				log.
					Warn().
					Err(err).
					Str("knowledge_id", knowledge.ID).
					Msg("failed to index knowledge")
			}
		}(k)
	}

	return nil
}

func (r *Reconciler) indexKnowledge(ctx context.Context, k *types.Knowledge) error {
	// If source is plain text, nothing to do
	if k.Source.Content != nil {
		k.State = types.KnowledgeStateReady
		_, err := r.store.UpdateKnowledge(ctx, k)
		if err != nil {
			return fmt.Errorf("failed to update knowledge")
		}
		return nil
	}

	// TODO
	return nil
}
