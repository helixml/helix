//go:build !linux

package hosthistory

import (
	"context"
	"errors"
	"fmt"
)

func Run(context.Context, string) error {
	return fmt.Errorf("host resource collection requires Linux: %w", errors.ErrUnsupported)
}
