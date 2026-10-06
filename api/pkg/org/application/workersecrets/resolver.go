package workersecrets

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
)

type ResolveFunc func(context.Context, workersecret.Binding) (workersecret.Resolved, error)
type ValidateFunc func(context.Context, workersecret.Binding) error

type Resolver struct {
	HelixSecret              ResolveFunc
	ValidateHelixSecret      ValidateFunc
	ConnectedAccount         ResolveFunc
	ValidateConnectedAccount ValidateFunc
}

func (r Resolver) Validate(ctx context.Context, b workersecret.Binding) error {
	switch b.SourceKind {
	case workersecret.SourceHelixSecret:
		if r.ValidateHelixSecret == nil {
			return fmt.Errorf("Helix Secret resolution is unavailable")
		}
		return r.ValidateHelixSecret(ctx, b)
	case workersecret.SourceConnectedAccount:
		if r.ValidateConnectedAccount == nil {
			return fmt.Errorf("Connected Account resolution is unavailable")
		}
		return r.ValidateConnectedAccount(ctx, b)
	default:
		return fmt.Errorf("unsupported source kind %q", b.SourceKind)
	}
}
func (r Resolver) Resolve(ctx context.Context, b workersecret.Binding) (workersecret.Resolved, error) {
	if err := r.Validate(ctx, b); err != nil {
		return workersecret.Resolved{}, err
	}
	if b.SourceKind == workersecret.SourceHelixSecret {
		if r.HelixSecret == nil {
			return workersecret.Resolved{}, fmt.Errorf("Helix Secret resolution is unavailable")
		}
		return r.HelixSecret(ctx, b)
	}
	if r.ConnectedAccount == nil {
		return workersecret.Resolved{}, fmt.Errorf("Connected Account resolution is unavailable")
	}
	return r.ConnectedAccount(ctx, b)
}
