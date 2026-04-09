package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
)

type StoreEventOperation string

const (
	StoreEventOperationCreated StoreEventOperation = "created"
	StoreEventOperationUpdated StoreEventOperation = "updated"
	StoreEventOperationDeleted StoreEventOperation = "deleted"
)

type StoreEventResourceType string

const (
	StoreEventResourceTypeSpecTask StoreEventResourceType = "spec_task"
	StoreEventResourceTypeSession  StoreEventResourceType = "session"
)

type StoreEvent struct {
	Operation      StoreEventOperation    `json:"operation"`
	ResourceType   StoreEventResourceType `json:"resource_type"`
	ResourceID     string                 `json:"resource_id,omitempty"`
	OrganizationID string                 `json:"organization_id,omitempty"`
	ProjectID      string                 `json:"project_id,omitempty"`
	OccurredAt     time.Time              `json:"occurred_at"`
	Resource       json.RawMessage        `json:"resource"`
}

func (e *StoreEvent) UnmarshalResource(dst any) error {
	if len(e.Resource) == 0 {
		return fmt.Errorf("event resource is empty")
	}

	if err := json.Unmarshal(e.Resource, dst); err != nil {
		return fmt.Errorf("failed to unmarshal event resource: %w", err)
	}

	return nil
}

type StoreEventSubscriptionFilter struct {
	ResourceType   StoreEventResourceType
	ResourceID     string
	OrganizationID string
	ProjectID      string
}

func (f *StoreEventSubscriptionFilter) Matches(event *StoreEvent) bool {
	if f == nil {
		return true
	}

	if f.ResourceType != "" && event.ResourceType != f.ResourceType {
		return false
	}

	if f.ResourceID != "" && event.ResourceID != f.ResourceID {
		return false
	}

	if f.OrganizationID != "" && event.OrganizationID != f.OrganizationID {
		return false
	}

	if f.ProjectID != "" && event.ProjectID != f.ProjectID {
		return false
	}

	return true
}

func (s *PostgresStore) publishStoreEvent(ctx context.Context, operation StoreEventOperation, resource any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	event, err := buildStoreEvent(operation, resource)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal store event: %w", err)
	}

	return s.pubsub.Publish(ctx, newStoreEventSubject(event.ResourceType, event.OrganizationID, event.ProjectID), payload)
}

func (s *PostgresStore) subscribeStoreEvents(ctx context.Context, filter *StoreEventSubscriptionFilter, handler func(event *StoreEvent) error) (pubsub.Subscription, error) {
	subject := newStoreEventSubscriptionSubject(filter)

	return s.pubsub.Subscribe(ctx, subject, func(payload []byte) error {
		var event StoreEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return fmt.Errorf("failed to unmarshal store event: %w", err)
		}

		if !filter.Matches(&event) {
			return nil
		}

		return handler(&event)
	})
}

func buildStoreEvent(operation StoreEventOperation, resource any) (*StoreEvent, error) {
	resourceType, resourceID, organizationID, projectID, err := extractStoreEventMetadata(resource)
	if err != nil {
		return nil, err
	}

	resourcePayload, err := json.Marshal(resource)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resource payload: %w", err)
	}

	return &StoreEvent{
		Operation:      operation,
		ResourceType:   resourceType,
		ResourceID:     resourceID,
		OrganizationID: organizationID,
		ProjectID:      projectID,
		OccurredAt:     time.Now().UTC(),
		Resource:       resourcePayload,
	}, nil
}

func extractStoreEventMetadata(resource any) (resourceType StoreEventResourceType, resourceID, organizationID, projectID string, err error) {
	switch r := resource.(type) {
	case *types.SpecTask:
		if r == nil {
			return "", "", "", "", fmt.Errorf("resource is nil spec task")
		}
		return StoreEventResourceTypeSpecTask, r.ID, r.OrganizationID, r.ProjectID, nil
	case types.SpecTask:
		return StoreEventResourceTypeSpecTask, r.ID, r.OrganizationID, r.ProjectID, nil
	case *types.Session:
		if r == nil {
			return "", "", "", "", fmt.Errorf("resource is nil session")
		}
		return StoreEventResourceTypeSession, r.ID, r.OrganizationID, r.ProjectID, nil
	case types.Session:
		return StoreEventResourceTypeSession, r.ID, r.OrganizationID, r.ProjectID, nil
	default:
		return "", "", "", "", fmt.Errorf("unsupported resource type for store event: %T", resource)
	}
}

func newStoreEventSubscriptionSubject(filter *StoreEventSubscriptionFilter) string {
	resourceToken := "*"
	if filter != nil && filter.ResourceType != "" {
		resourceToken = string(filter.ResourceType)
	}

	orgToken := "*"
	if filter != nil && filter.OrganizationID != "" {
		orgToken = subjectToken(filter.OrganizationID)
	}

	projectToken := "*"
	if filter != nil && filter.ProjectID != "" {
		projectToken = subjectToken(filter.ProjectID)
	}

	return fmt.Sprintf("store.events.%s.%s.%s", resourceToken, orgToken, projectToken)
}

func newStoreEventSubject(resourceType StoreEventResourceType, organizationID, projectID string) string {
	return fmt.Sprintf(
		"store.events.%s.%s.%s",
		resourceType,
		subjectToken(organizationID),
		subjectToken(projectID),
	)
}

func subjectToken(value string) string {
	if value == "" {
		return "_"
	}

	return strings.ReplaceAll(value, ".", "_")
}
