package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// CreateAgentWorkItem creates a new work item in the agent queue
func (s *PostgresStore) CreateAgentWorkItem(ctx context.Context, item *types.AgentWorkItem) error {
	return s.gdb.WithContext(ctx).Create(item).Error
}

// GetAgentWorkItem retrieves a work item by ID
func (s *PostgresStore) GetAgentWorkItem(ctx context.Context, id string) (*types.AgentWorkItem, error) {
	var item types.AgentWorkItem
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// UpdateAgentWorkItem updates an existing work item
func (s *PostgresStore) UpdateAgentWorkItem(ctx context.Context, item *types.AgentWorkItem) error {
	item.UpdatedAt = time.Now()
	return s.gdb.WithContext(ctx).Save(item).Error
}

// ListAgentWorkItems returns work items with pagination and filtering
func (s *PostgresStore) ListAgentWorkItems(ctx context.Context, query *ListAgentWorkItemsQuery) (*types.AgentWorkItemsResponse, error) {
	var items []*types.AgentWorkItem
	var total int64

	db := s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{})

	// Apply filters
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	if query.Source != "" {
		db = db.Where("source = ?", query.Source)
	}
	if query.AgentType != "" {
		db = db.Where("agent_type = ?", query.AgentType)
	}
	if query.UserID != "" {
		db = db.Where("user_id = ?", query.UserID)
	}
	if query.AppID != "" {
		db = db.Where("app_id = ?", query.AppID)
	}
	if query.OrganizationID != "" {
		db = db.Where("organization_id = ?", query.OrganizationID)
	}
	if query.TriggerConfigID != "" {
		db = db.Where("trigger_config_id = ?", query.TriggerConfigID)
	}
	if query.AssignedOnly {
		db = db.Where("assigned_session_id IS NOT NULL AND assigned_session_id != ''")
	}
	if query.UnassignedOnly {
		db = db.Where("assigned_session_id IS NULL OR assigned_session_id = ''")
	}

	// Count total
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// Apply pagination and ordering
	offset := query.Page * query.PageSize
	orderBy := "priority ASC, created_at ASC"
	if query.OrderBy != "" {
		orderBy = query.OrderBy
	}

	if err := db.Offset(offset).Limit(query.PageSize).Order(orderBy).Find(&items).Error; err != nil {
		return nil, err
	}

	return &types.AgentWorkItemsResponse{
		Items:    items,
		Total:    int(total),
		Page:     query.Page,
		PageSize: query.PageSize,
	}, nil
}

// GetNextPendingWorkItem gets the next work item that should be processed
func (s *PostgresStore) GetNextPendingWorkItem(ctx context.Context, agentType string) (*types.AgentWorkItem, error) {
	var item types.AgentWorkItem
	db := s.gdb.WithContext(ctx).Where("status = ?", "pending")

	// Filter by agent type if specified
	if agentType != "" {
		db = db.Where("agent_type = ? OR agent_type = ''", agentType)
	}

	// Only get items that are scheduled for now or earlier
	db = db.Where("scheduled_for IS NULL OR scheduled_for <= ?", time.Now())

	// Order by priority (lower numbers = higher priority), then by creation time
	err := db.Order("priority ASC, created_at ASC").First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// AssignWorkItemToSession assigns a work item to an agent session
func (s *PostgresStore) AssignWorkItemToSession(ctx context.Context, workItemID, sessionID string) error {
	return s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{}).
		Where("id = ?", workItemID).
		Updates(map[string]interface{}{
			"status":              "assigned",
			"assigned_session_id": sessionID,
			"started_at":          time.Now(),
			"updated_at":          time.Now(),
		}).Error
}

// UnassignWorkItem removes assignment from a work item
func (s *PostgresStore) UnassignWorkItem(ctx context.Context, workItemID string) error {
	return s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{}).
		Where("id = ?", workItemID).
		Updates(map[string]interface{}{
			"status":              "pending",
			"assigned_session_id": nil,
			"started_at":          nil,
			"updated_at":          time.Now(),
		}).Error
}

// MarkWorkItemInProgress marks a work item as in progress
func (s *PostgresStore) MarkWorkItemInProgress(ctx context.Context, workItemID string) error {
	return s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{}).
		Where("id = ?", workItemID).
		Updates(map[string]interface{}{
			"status":     "in_progress",
			"updated_at": time.Now(),
		}).Error
}

// CompleteWorkItem marks a work item as completed
func (s *PostgresStore) CompleteWorkItem(ctx context.Context, workItemID string, success bool, result string) error {
	status := "completed"
	if !success {
		status = "failed"
	}

	updates := map[string]interface{}{
		"status":       status,
		"completed_at": time.Now(),
		"updated_at":   time.Now(),
	}

	if !success {
		updates["last_error"] = result
	}

	return s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{}).
		Where("id = ?", workItemID).
		Updates(updates).Error
}

// CreateAgentSessionStatus creates a new agent session status record
func (s *PostgresStore) CreateAgentSessionStatus(ctx context.Context, session *types.AgentSessionStatus) error {
	return s.gdb.WithContext(ctx).Create(session).Error
}

// GetAgentSessionStatus retrieves an agent session status by session ID
func (s *PostgresStore) GetAgentSessionStatus(ctx context.Context, sessionID string) (*types.AgentSessionStatus, error) {
	var session types.AgentSessionStatus
	err := s.gdb.WithContext(ctx).Where("session_id = ?", sessionID).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// UpdateAgentSessionStatus updates an existing agent session status
func (s *PostgresStore) UpdateAgentSessionStatus(ctx context.Context, session *types.AgentSessionStatus) error {
	session.UpdatedAt = time.Now()
	return s.gdb.WithContext(ctx).Save(session).Error
}

// ListAgentSessions returns agent sessions with pagination and filtering
func (s *PostgresStore) ListAgentSessions(ctx context.Context, query *ListAgentSessionsQuery) (*types.AgentSessionsResponse, error) {
	var sessions []*types.AgentSessionStatus
	var total int64

	db := s.gdb.WithContext(ctx).Model(&types.AgentSessionStatus{})

	// Apply filters
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	if query.AgentType != "" {
		db = db.Where("agent_type = ?", query.AgentType)
	}
	if query.UserID != "" {
		db = db.Where("user_id = ?", query.UserID)
	}
	if query.AppID != "" {
		db = db.Where("app_id = ?", query.AppID)
	}
	if query.OrganizationID != "" {
		db = db.Where("organization_id = ?", query.OrganizationID)
	}
	if query.ActiveOnly {
		db = db.Where("status IN (?)", []string{"starting", "active", "waiting_for_help", "paused"})
	}
	if query.HealthStatus != "" {
		db = db.Where("health_status = ?", query.HealthStatus)
	}

	// Count total
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// Apply pagination
	offset := query.Page * query.PageSize
	orderBy := "last_activity DESC"
	if query.OrderBy != "" {
		orderBy = query.OrderBy
	}

	if err := db.Offset(offset).Limit(query.PageSize).Order(orderBy).Find(&sessions).Error; err != nil {
		return nil, err
	}

	return &types.AgentSessionsResponse{
		Sessions: sessions,
		Total:    int(total),
		Page:     query.Page,
		PageSize: query.PageSize,
	}, nil
}

// GetSessionsNeedingHelp returns all sessions that are waiting for human help
func (s *PostgresStore) GetSessionsNeedingHelp(ctx context.Context) ([]*types.AgentSessionStatus, error) {
	var sessions []*types.AgentSessionStatus
	err := s.gdb.WithContext(ctx).
		Where("status = ?", "waiting_for_help").
		Order("last_activity DESC").
		Find(&sessions).Error
	return sessions, err
}

// MarkSessionAsNeedingHelp marks a session as waiting for human help
func (s *PostgresStore) MarkSessionAsNeedingHelp(ctx context.Context, sessionID string, task string) error {
	return s.gdb.WithContext(ctx).Model(&types.AgentSessionStatus{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":       "waiting_for_help",
			"current_task": task,
			"updated_at":   time.Now(),
		}).Error
}

// MarkSessionAsActive marks a session as active
func (s *PostgresStore) MarkSessionAsActive(ctx context.Context, sessionID string, task string) error {
	return s.gdb.WithContext(ctx).Model(&types.AgentSessionStatus{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":        "active",
			"current_task":  task,
			"last_activity": time.Now(),
			"updated_at":    time.Now(),
		}).Error
}

// MarkSessionAsCompleted marks a session as completed
func (s *PostgresStore) MarkSessionAsCompleted(ctx context.Context, sessionID string, completionType string) error {
	status := "completed"
	if completionType == "pending_review" {
		status = "pending_review"
	}

	return s.gdb.WithContext(ctx).Model(&types.AgentSessionStatus{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":       status,
			"completed_at": time.Now(),
			"updated_at":   time.Now(),
		}).Error
}

// UpdateSessionHealth updates the health status of a session
func (s *PostgresStore) UpdateSessionHealth(ctx context.Context, sessionID, healthStatus string) error {
	return s.gdb.WithContext(ctx).Model(&types.AgentSessionStatus{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"health_status":     healthStatus,
			"health_checked_at": time.Now(),
			"last_activity":     time.Now(),
			"updated_at":        time.Now(),
		}).Error
}

// CreateAgentWorkExecution creates a new work execution record
func (s *PostgresStore) CreateAgentWorkExecution(ctx context.Context, execution *types.AgentWorkExecution) error {
	return s.gdb.WithContext(ctx).Create(execution).Error
}

// UpdateAgentWorkExecution updates a work execution record
func (s *PostgresStore) UpdateAgentWorkExecution(ctx context.Context, execution *types.AgentWorkExecution) error {
	execution.UpdatedAt = time.Now()
	return s.gdb.WithContext(ctx).Save(execution).Error
}

// ListAgentWorkExecutions returns work executions with pagination and filtering
func (s *PostgresStore) ListAgentWorkExecutions(ctx context.Context, query *ListAgentWorkExecutionsQuery) (*types.AgentWorkExecutionsResponse, error) {
	var executions []*types.AgentWorkExecution
	var total int64

	db := s.gdb.WithContext(ctx).Model(&types.AgentWorkExecution{})

	// Apply filters
	if query.TriggerConfigID != "" {
		db = db.Where("trigger_config_id = ?", query.TriggerConfigID)
	}
	if query.WorkItemID != "" {
		db = db.Where("work_item_id = ?", query.WorkItemID)
	}
	if query.SessionID != "" {
		db = db.Where("session_id = ?", query.SessionID)
	}
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	if query.AgentType != "" {
		db = db.Where("agent_type = ?", query.AgentType)
	}

	// Count total
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// Apply pagination
	offset := query.Page * query.PageSize
	orderBy := "created_at DESC"
	if query.OrderBy != "" {
		orderBy = query.OrderBy
	}

	if err := db.Offset(offset).Limit(query.PageSize).Order(orderBy).Find(&executions).Error; err != nil {
		return nil, err
	}

	return &types.AgentWorkExecutionsResponse{
		Executions: executions,
		Total:      int(total),
		Page:       query.Page,
		PageSize:   query.PageSize,
	}, nil
}

// GetAgentWorkQueueStats returns statistics about the agent work queue
func (s *PostgresStore) GetAgentWorkQueueStats(ctx context.Context) (*types.AgentWorkQueueStats, error) {
	stats := &types.AgentWorkQueueStats{
		ByAgentType: make(map[string]int),
		BySource:    make(map[string]int),
		ByPriority:  make(map[string]int),
	}

	// Count by status
	var statusCounts []struct {
		Status string
		Count  int
	}
	err := s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{}).
		Select("status, count(*) as count").
		Group("status").
		Find(&statusCounts).Error
	if err != nil {
		return nil, err
	}

	for _, sc := range statusCounts {
		switch sc.Status {
		case "pending":
			stats.TotalPending = sc.Count
		case "in_progress", "assigned":
			stats.TotalRunning = sc.Count
		case "completed":
			stats.TotalCompleted = sc.Count
		case "failed":
			stats.TotalFailed = sc.Count
		}
	}

	// Count by agent type
	var agentTypeCounts []struct {
		AgentType string
		Count     int
	}
	err = s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{}).
		Select("agent_type, count(*) as count").
		Where("status IN (?)", []string{"pending", "in_progress", "assigned"}).
		Group("agent_type").
		Find(&agentTypeCounts).Error
	if err != nil {
		return nil, err
	}

	for _, atc := range agentTypeCounts {
		stats.ByAgentType[atc.AgentType] = atc.Count
	}

	// Count by source
	var sourceCounts []struct {
		Source string
		Count  int
	}
	err = s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{}).
		Select("source, count(*) as count").
		Where("status IN (?)", []string{"pending", "in_progress", "assigned"}).
		Group("source").
		Find(&sourceCounts).Error
	if err != nil {
		return nil, err
	}

	for _, sc := range sourceCounts {
		stats.BySource[sc.Source] = sc.Count
	}

	// Count active sessions
	err = s.gdb.WithContext(ctx).Model(&types.AgentSessionStatus{}).
		Where("status IN (?)", []string{"starting", "active", "waiting_for_help", "paused"}).
		Count(&stats.ActiveSessions).Error
	if err != nil {
		return nil, err
	}

	// Get oldest pending work item
	var oldestPending time.Time
	err = s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{}).
		Select("created_at").
		Where("status = ?", "pending").
		Order("created_at ASC").
		Limit(1).
		Pluck("created_at", &oldestPending).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if err != gorm.ErrRecordNotFound {
		stats.OldestPending = &oldestPending
		stats.AverageWaitTime = time.Since(oldestPending).Minutes()
	}

	return stats, nil
}

// CleanupExpiredSessions removes sessions that have been inactive for too long
func (s *PostgresStore) CleanupExpiredSessions(ctx context.Context, timeout time.Duration) error {
	cutoff := time.Now().Add(-timeout)
	return s.gdb.WithContext(ctx).Model(&types.AgentSessionStatus{}).
		Where("last_activity < ? AND status IN (?)", cutoff, []string{"starting", "active", "paused"}).
		Update("status", "failed").
		Error
}

// RetryFailedWorkItem retries a failed work item
func (s *PostgresStore) RetryFailedWorkItem(ctx context.Context, workItemID string) error {
	return s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{}).
		Where("id = ? AND status = ?", workItemID, "failed").
		Where("retry_count < max_retries").
		Updates(map[string]interface{}{
			"status":              "pending",
			"retry_count":         gorm.Expr("retry_count + 1"),
			"assigned_session_id": nil,
			"started_at":          nil,
			"completed_at":        nil,
			"updated_at":          time.Now(),
		}).Error
}

// Query types for the store methods
type ListAgentWorkItemsQuery struct {
	Page            int
	PageSize        int
	Status          string
	Source          string
	AgentType       string
	UserID          string
	AppID           string
	OrganizationID  string
	TriggerConfigID string
	AssignedOnly    bool
	UnassignedOnly  bool
	OrderBy         string
}

type ListAgentSessionsQuery struct {
	Page           int
	PageSize       int
	Status         string
	AgentType      string
	UserID         string
	AppID          string
	OrganizationID string
	ActiveOnly     bool
	HealthStatus   string
	OrderBy        string
}

type ListAgentWorkExecutionsQuery struct {
	Page            int
	PageSize        int
	TriggerConfigID string
	WorkItemID      string
	SessionID       string
	Status          string
	AgentType       string
	OrderBy         string
}
