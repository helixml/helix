package store

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *PostgresStore) CreateTool(ctx context.Context, tool *types.Tool) (*types.Tool, error) {
	if tool.ID == "" {
		tool.ID = system.GenerateToolID()
	}

	if tool.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	err := s.gdb.WithContext(ctx).Create(&tool).Error
	if err != nil {
		return nil, err
	}
	return s.GetTool(ctx, tool.ID)
}

func (s *PostgresStore) GetTool(ctx context.Context, id string) (*types.Tool, error) {

}
