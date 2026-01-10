package types

import (
	"context"
	"testing"
)

func TestSetHelixProjectContext(t *testing.T) {
	ctx := context.Background()
	ctx = SetHelixProjectContext(ctx, "123")
	projectID, ok := GetHelixProjectContext(ctx)
	if !ok {
		t.Errorf("project ID not found")
	}
	if projectID.ProjectID != "123" {
		t.Errorf("project ID is not 123")
	}
}
