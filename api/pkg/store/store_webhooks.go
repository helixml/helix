package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WebhookDeliveryUpdate struct {
	ID             string
	Status         types.WebhookDeliveryStatus
	AttemptCount   int
	NextAttemptAt  time.Time
	LastAttemptAt  time.Time
	DeliveredAt    *time.Time
	LastStatusCode int
	LastError      string
}

func (s *PostgresStore) CreateWebhookEndpoint(ctx context.Context, endpoint *types.WebhookEndpoint) error {
	if endpoint == nil || endpoint.ID == "" || endpoint.OrganizationID == "" {
		return errors.New("webhook endpoint ID and organization ID are required")
	}
	if err := s.gdb.WithContext(ctx).Create(endpoint).Error; err != nil {
		return fmt.Errorf("create webhook endpoint: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetWebhookEndpoint(ctx context.Context, organizationID, endpointID string) (*types.WebhookEndpoint, error) {
	var endpoint types.WebhookEndpoint
	if err := s.gdb.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, endpointID).First(&endpoint).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get webhook endpoint: %w", err)
	}
	return &endpoint, nil
}

func (s *PostgresStore) ListWebhookEndpoints(ctx context.Context, organizationID string) ([]*types.WebhookEndpoint, error) {
	var endpoints []*types.WebhookEndpoint
	if err := s.gdb.WithContext(ctx).Where("organization_id = ?", organizationID).Order("created_at DESC").Find(&endpoints).Error; err != nil {
		return nil, fmt.Errorf("list webhook endpoints: %w", err)
	}
	return endpoints, nil
}

func (s *PostgresStore) UpdateWebhookEndpointConfig(ctx context.Context, endpoint *types.WebhookEndpoint) error {
	if endpoint == nil || endpoint.ID == "" || endpoint.OrganizationID == "" {
		return errors.New("webhook endpoint ID and organization ID are required")
	}
	endpoint.UpdatedAt = time.Now().UTC()
	result := s.gdb.WithContext(ctx).Model(&types.WebhookEndpoint{}).
		Where("id = ? AND organization_id = ?", endpoint.ID, endpoint.OrganizationID).
		Select("project_id", "url", "description", "events", "status", "disabled_reason", "updated_by", "updated_at").
		Updates(endpoint)
	if result.Error != nil {
		return fmt.Errorf("update webhook endpoint: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) RotateWebhookEndpointSecret(ctx context.Context, endpoint *types.WebhookEndpoint) error {
	if endpoint == nil || endpoint.ID == "" || endpoint.OrganizationID == "" {
		return errors.New("webhook endpoint ID and organization ID are required")
	}
	now := time.Now().UTC()
	result := s.gdb.WithContext(ctx).Model(&types.WebhookEndpoint{}).
		Where("id = ? AND organization_id = ?", endpoint.ID, endpoint.OrganizationID).
		Updates(map[string]any{
			"secret_encrypted":           endpoint.SecretEncrypted,
			"secret_preview":             endpoint.SecretPreview,
			"previous_secret_encrypted":  endpoint.PreviousSecretEncrypted,
			"previous_secret_expires_at": endpoint.PreviousSecretExpiresAt,
			"updated_by":                 endpoint.UpdatedBy,
			"updated_at":                 now,
		})
	if result.Error != nil {
		return fmt.Errorf("rotate webhook endpoint secret: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	endpoint.UpdatedAt = now
	return nil
}

func (s *PostgresStore) DisableWebhookEndpoint(ctx context.Context, organizationID, endpointID, reason, updatedBy string) error {
	result := s.gdb.WithContext(ctx).Model(&types.WebhookEndpoint{}).
		Where("id = ? AND organization_id = ?", endpointID, organizationID).
		Updates(map[string]any{
			"status":          types.WebhookEndpointStatusDisabled,
			"disabled_reason": reason,
			"updated_by":      updatedBy,
			"updated_at":      time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("disable webhook endpoint: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) ListWebhookDeliveries(ctx context.Context, endpointID string, limit int) ([]*types.WebhookDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var deliveries []*types.WebhookDelivery
	if err := s.gdb.WithContext(ctx).Where("endpoint_id = ?", endpointID).Order("created_at DESC").Limit(limit).Find(&deliveries).Error; err != nil {
		return nil, fmt.Errorf("list webhook deliveries: %w", err)
	}
	return deliveries, nil
}

func (s *PostgresStore) GetWebhookDelivery(ctx context.Context, endpointID, deliveryID string) (*types.WebhookDelivery, error) {
	var delivery types.WebhookDelivery
	if err := s.gdb.WithContext(ctx).Where("endpoint_id = ? AND id = ?", endpointID, deliveryID).First(&delivery).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get webhook delivery: %w", err)
	}
	return &delivery, nil
}

func (s *PostgresStore) GetWebhookEvent(ctx context.Context, eventID string) (*types.WebhookEvent, error) {
	var event types.WebhookEvent
	if err := s.gdb.WithContext(ctx).Where("id = ?", eventID).First(&event).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get webhook event: %w", err)
	}
	return &event, nil
}

func (s *PostgresStore) ClaimWebhookDeliveries(ctx context.Context, now, lockedUntil time.Time, limit int) ([]*types.WebhookDelivery, error) {
	if limit <= 0 {
		limit = 25
	}
	var deliveries []*types.WebhookDelivery
	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("((status IN ?) OR (status = ? AND locked_until <= ?)) AND next_attempt_at <= ?",
				[]types.WebhookDeliveryStatus{types.WebhookDeliveryStatusPending, types.WebhookDeliveryStatusRetrying},
				types.WebhookDeliveryStatusProcessing, now, now).
			Order("next_attempt_at ASC").Limit(limit).Find(&deliveries).Error; err != nil {
			return err
		}
		if len(deliveries) == 0 {
			return nil
		}
		ids := make([]string, 0, len(deliveries))
		for _, delivery := range deliveries {
			ids = append(ids, delivery.ID)
			delivery.Status = types.WebhookDeliveryStatusProcessing
			delivery.LockedUntil = &lockedUntil
		}
		return tx.Model(&types.WebhookDelivery{}).Where("id IN ?", ids).Updates(map[string]any{
			"status":       types.WebhookDeliveryStatusProcessing,
			"locked_until": lockedUntil,
			"updated_at":   now,
		}).Error
	})
	if err != nil {
		return nil, fmt.Errorf("claim webhook deliveries: %w", err)
	}
	return deliveries, nil
}

func (s *PostgresStore) CompleteWebhookDelivery(ctx context.Context, update *WebhookDeliveryUpdate) error {
	if update == nil || update.ID == "" {
		return errors.New("webhook delivery update ID is required")
	}
	fields := map[string]any{
		"status":           update.Status,
		"attempt_count":    update.AttemptCount,
		"next_attempt_at":  update.NextAttemptAt,
		"last_attempt_at":  update.LastAttemptAt,
		"last_status_code": update.LastStatusCode,
		"last_error":       update.LastError,
		"locked_until":     nil,
		"updated_at":       time.Now().UTC(),
	}
	if update.DeliveredAt != nil {
		fields["delivered_at"] = *update.DeliveredAt
	}
	result := s.gdb.WithContext(ctx).Model(&types.WebhookDelivery{}).Where("id = ?", update.ID).Updates(fields)
	if result.Error != nil {
		return fmt.Errorf("complete webhook delivery: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) ReplayWebhookDelivery(ctx context.Context, endpointID, deliveryID string, now time.Time) error {
	result := s.gdb.WithContext(ctx).Model(&types.WebhookDelivery{}).
		Where("id = ? AND endpoint_id = ? AND status <> ?", deliveryID, endpointID, types.WebhookDeliveryStatusProcessing).
		Updates(map[string]any{
			"status":           types.WebhookDeliveryStatusPending,
			"attempt_count":    0,
			"next_attempt_at":  now,
			"locked_until":     nil,
			"last_attempt_at":  nil,
			"delivered_at":     nil,
			"last_status_code": 0,
			"last_error":       "",
			"updated_at":       now,
		})
	if result.Error != nil {
		return fmt.Errorf("replay webhook delivery: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) EnqueueWebhookEvent(ctx context.Context, eventType, organizationID, projectID string, data any) error {
	return s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return enqueueWebhookEventTx(tx, eventType, organizationID, projectID, data)
	})
}

func enqueueWebhookEventTx(tx *gorm.DB, eventType, organizationID, projectID string, data any) error {
	if organizationID == "" {
		return nil
	}
	var endpoints []*types.WebhookEndpoint
	if err := tx.Where("organization_id = ? AND status = ? AND (project_id = '' OR project_id = ?)",
		organizationID, types.WebhookEndpointStatusActive, projectID).Find(&endpoints).Error; err != nil {
		return fmt.Errorf("find matching webhook endpoints: %w", err)
	}
	matched := make([]*types.WebhookEndpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if webhookEndpointAccepts(endpoint, eventType) {
			matched = append(matched, endpoint)
		}
	}
	if len(matched) == 0 {
		return nil
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal webhook event data: %w", err)
	}
	now := time.Now().UTC()
	event := &types.WebhookEvent{
		ID:             "whevt_" + uuid.NewString(),
		APIVersion:     "v1",
		Type:           eventType,
		OrganizationID: organizationID,
		ProjectID:      projectID,
		Data:           payload,
		CreatedAt:      now,
	}
	if err := tx.Create(event).Error; err != nil {
		return fmt.Errorf("create webhook event: %w", err)
	}
	for _, endpoint := range matched {
		delivery := &types.WebhookDelivery{
			ID:            "whd_" + uuid.NewString(),
			EventID:       event.ID,
			EndpointID:    endpoint.ID,
			Status:        types.WebhookDeliveryStatusPending,
			NextAttemptAt: now,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := tx.Create(delivery).Error; err != nil {
			return fmt.Errorf("create webhook delivery: %w", err)
		}
	}
	return nil
}

func webhookEndpointAccepts(endpoint *types.WebhookEndpoint, eventType string) bool {
	for _, subscribed := range endpoint.Events {
		if subscribed == eventType || subscribed == "*" {
			return true
		}
	}
	return false
}
