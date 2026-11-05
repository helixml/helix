package configregistry

import (
	"context"
	"fmt"
)

const MaxConcurrentActivationsKey = "worker.max_concurrent_activations"

func (r *Registry) GetMaxConcurrentActivations(ctx context.Context, orgID string) (int, error) {
	value, err := r.GetInt(ctx, orgID, MaxConcurrentActivationsKey)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", MaxConcurrentActivationsKey)
	}
	return int(value), nil
}
