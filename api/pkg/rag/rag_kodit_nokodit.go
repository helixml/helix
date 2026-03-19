//go:build nokodit

package rag

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// KoditRAG is a stub that returns an error when built without kodit support.
type KoditRAG struct{}

func NewKoditRAG(_ services.KoditServicer, _ store.Store, _ config.FileStore) *KoditRAG {
	return &KoditRAG{}
}

func (k *KoditRAG) Index(_ context.Context, _ ...*types.SessionRAGIndexChunk) error {
	return fmt.Errorf("kodit support not compiled in (build tag: nokodit)")
}

func (k *KoditRAG) RegisterDirectory(_ context.Context, _, _, _, _ string) error {
	return fmt.Errorf("kodit support not compiled in (build tag: nokodit)")
}

func (k *KoditRAG) Query(_ context.Context, _ *types.SessionRAGQuery) ([]*types.SessionRAGResult, error) {
	return nil, fmt.Errorf("kodit support not compiled in (build tag: nokodit)")
}

func (k *KoditRAG) Delete(_ context.Context, _ *types.DeleteIndexRequest) error {
	return fmt.Errorf("kodit support not compiled in (build tag: nokodit)")
}
