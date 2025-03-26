package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *PostgresStore) CreateUsageMetric(ctx context.Context, metric *types.UsageMetric) (*types.UsageMetric, error) {
	if metric.ID == "" {
		metric.ID = system.GenerateUsageMetricID()
	}

	call.Created = time.Now()

	err := s.gdb.WithContext(ctx).Create(call).Error
	if err != nil {
		return nil, err
	}
	return call, nil
}
