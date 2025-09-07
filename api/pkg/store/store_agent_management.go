package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// CreateHelpRequest creates a new help request
func (s *PostgresStore) CreateHelpRequest(ctx context.Context, request *types.HelpRequest) error {
	return s.gdb.WithContext(ctx).Create(request).Error
}

// GetHelpRequest retrieves a help request by session ID and interaction ID
func (s *PostgresStore) GetHelpRequest(ctx context.Context, sessionID, interactionID string) (*types.HelpRequest, error) {
	var request types.HelpRequest
	err := s.gdb.WithContext(ctx).
		Where("session_id = ? AND interaction_id = ?", sessionID, interactionID).
		First(&request).Error
	if err != nil {
		return nil, err
	}
	return &request, nil
}

// GetHelpRequestByID retrieves a help request by ID
func (s *PostgresStore) GetHelpRequestByID(ctx context.Context, id string) (*types.HelpRequest, error) {
	var request types.HelpRequest
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&request).Error
	if err != nil {
		return nil, err
	}
	return &request, nil
}

// UpdateHelpRequest updates an existing help request
func (s *PostgresStore) UpdateHelpRequest(ctx context.Context, request *types.HelpRequest) error {
	request.UpdatedAt = time.Now()
	return s.gdb.WithContext(ctx).Save(request).Error
}

// ListActiveHelpRequests returns all help requests that are not resolved or cancelled
func (s *PostgresStore) ListActiveHelpRequests(ctx context.Context) ([]*types.HelpRequest, error) {
	var requests []*types.HelpRequest
	err := s.gdb.WithContext(ctx).
		Where("status NOT IN (?)", []string{"resolved", "cancelled"}).
		Order("created_at DESC").
		Find(&requests).Error
	return requests, err
}

// ListHelpRequests returns help requests with pagination and filtering
func (s *PostgresStore) ListHelpRequests(ctx context.Context, query *ListHelpRequestsQuery) (*types.HelpRequestsListResponse, error) {
	var requests []*types.HelpRequest
	var total int64

	db := s.gdb.WithContext(ctx).Model(&types.HelpRequest{})

	// Apply filters
	if query.UserID != "" {
		db = db.Where("user_id = ?", query.UserID)
	}
	if query.SessionID != "" {
		db = db.Where("session_id = ?", query.SessionID)
	}
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	if query.HelpType != "" {
		db = db.Where("help_type = ?", query.HelpType)
	}
	if query.Urgency != "" {
		db = db.Where("urgency = ?", query.Urgency)
	}

	// Count total
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// Apply pagination
	offset := query.Page * query.PageSize
	if err := db.Offset(offset).Limit(query.PageSize).Order("created_at DESC").Find(&requests).Error; err != nil {
		return nil, err
	}

	return &types.HelpRequestsListResponse{
		HelpRequests: requests,
		Total:        int(total),
		Page:         query.Page,
		PageSize:     query.PageSize,
	}, nil
}

// CreateAgentWorkItem creates a new work item in the agent scheduler
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
func (s *PostgresStore) ListAgentWorkItems(ctx context.Context, query *ListAgentWorkItemsQuery) (*types.AgentWorkItemsListResponse, error) {
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
		db = db.Where("session_id IS NOT NULL AND session_id != ''")
	}
	if query.UnassignedOnly {
		db = db.Where("session_id IS NULL OR session_id = ''")
	}

	// Count total
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// Apply pagination and ordering
	offset := query.Page * query.PageSize
	orderBy := "created_at DESC"
	if query.OrderBy != "" {
		orderBy = query.OrderBy
	}

	if err := db.Offset(offset).Limit(query.PageSize).Order(orderBy).Find(&items).Error; err != nil {
		return nil, err
	}

	return &types.AgentWorkItemsListResponse{
		WorkItems: items,
		Total:     int(total),
		Page:      query.Page,
		PageSize:  query.PageSize,
	}, nil
}

// GetNextPendingWorkItem gets the next work item that should be processed
func (s *PostgresStore) GetNextPendingWorkItem(ctx context.Context, agentType string) (*types.AgentWorkItem, error) {
	var item types.AgentWorkItem
	db := s.gdb.WithContext(ctx).Where("status = ? AND (agent_type = ? OR agent_type = '')", "pending", agentType)

	// Order by priority (lower numbers = higher priority), then by creation time
	err := db.Order("priority ASC, created_at ASC").First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// CreateAgentSession creates a new agent session record
func (s *PostgresStore) CreateAgentSession(ctx context.Context, session *types.AgentSession) error {
	return s.gdb.WithContext(ctx).Create(session).Error
}

// GetAgentSession retrieves an agent session by session ID
func (s *PostgresStore) GetAgentSession(ctx context.Context, sessionID string) (*types.AgentSession, error) {
	var session types.AgentSession
	err := s.gdb.WithContext(ctx).Where("session_id = ?", sessionID).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetAgentSessionByID retrieves an agent session by ID
func (s *PostgresStore) GetAgentSessionByID(ctx context.Context, id string) (*types.AgentSession, error) {
	var session types.AgentSession
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// UpdateAgentSession updates an existing agent session
func (s *PostgresStore) UpdateAgentSession(ctx context.Context, session *types.AgentSession) error {
	session.UpdatedAt = time.Now()
	return s.gdb.WithContext(ctx).Save(session).Error
}

// ListAgentSessions returns agent sessions with pagination and filtering
func (s *PostgresStore) ListAgentSessions(ctx context.Context, query *ListAgentSessionsQuery) (*types.AgentSessionsListResponse, error) {
	var sessions []*types.AgentSession
	var total int64

	db := s.gdb.WithContext(ctx).Model(&types.AgentSession{})

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
	if query.HealthStatus != "" {
		db = db.Where("health_status = ?", query.HealthStatus)
	}
	if query.ActiveOnly {
		db = db.Where("status IN (?)", []string{"starting", "active", "waiting_for_help", "paused"})
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

	return &types.AgentSessionsListResponse{
		Sessions: sessions,
		Total:    int(total),
		Page:     query.Page,
		PageSize: query.PageSize,
	}, nil
}

// GetSessionsNeedingHelp returns all sessions that are waiting for human help
func (s *PostgresStore) GetSessionsNeedingHelp(ctx context.Context) ([]*types.AgentSession, error) {
	var sessions []*types.AgentSession
	err := s.gdb.WithContext(ctx).
		Where("status = ?", "waiting_for_help").
		Order("last_activity DESC").
		Find(&sessions).Error
	return sessions, err
}

// MarkSessionAsNeedingHelp marks a session as waiting for human help
func (s *PostgresStore) MarkSessionAsNeedingHelp(ctx context.Context, sessionID string, task string) error {
	return s.gdb.WithContext(ctx).Model(&types.AgentSession{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":       "waiting_for_help",
			"current_task": task,
			"updated_at":   time.Now(),
		}).Error
}

// MarkSessionAsActive marks a session as active
func (s *PostgresStore) MarkSessionAsActive(ctx context.Context, sessionID string, task string) error {
	return s.gdb.WithContext(ctx).Model(&types.AgentSession{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":        "active",
			"current_task":  task,
			"last_activity": time.Now(),
			"updated_at":    time.Now(),
		}).Error
}

// CleanupExpiredSessions removes sessions that have been inactive for too long
func (s *PostgresStore) CleanupExpiredSessions(ctx context.Context, timeout time.Duration) error {
	cutoff := time.Now().Add(-timeout)
	return s.gdb.WithContext(ctx).
		Where("last_activity < ? AND status IN (?)", cutoff, []string{"starting", "active", "paused"}).
		Update("status", "failed").
		Error
}

// Query types for the store methods
type ListHelpRequestsQuery struct {
	Page      int
	PageSize  int
	UserID    string
	SessionID string
	Status    string
	HelpType  string
	Urgency   string
}

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

// CreateJobCompletion creates a new job completion record
func (s *PostgresStore) CreateJobCompletion(ctx context.Context, completion *types.JobCompletion) error {
	return s.gdb.WithContext(ctx).Create(completion).Error
}

// GetJobCompletion retrieves a job completion by session ID and interaction ID
func (s *PostgresStore) GetJobCompletion(ctx context.Context, sessionID, interactionID string) (*types.JobCompletion, error) {
	var completion types.JobCompletion
	err := s.gdb.WithContext(ctx).
		Where("session_id = ? AND interaction_id = ?", sessionID, interactionID).
		First(&completion).Error
	if err != nil {
		return nil, err
	}
	return &completion, nil
}

// GetJobCompletionByID retrieves a job completion by ID
func (s *PostgresStore) GetJobCompletionByID(ctx context.Context, id string) (*types.JobCompletion, error) {
	var completion types.JobCompletion
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&completion).Error
	if err != nil {
		return nil, err
	}
	return &completion, nil
}

// UpdateJobCompletion updates an existing job completion
func (s *PostgresStore) UpdateJobCompletion(ctx context.Context, completion *types.JobCompletion) error {
	completion.UpdatedAt = time.Now()
	return s.gdb.WithContext(ctx).Save(completion).Error
}

// ListJobCompletions returns job completions with pagination and filtering
func (s *PostgresStore) ListJobCompletions(ctx context.Context, query *ListJobCompletionsQuery) (*types.JobCompletionsListResponse, error) {
	var completions []*types.JobCompletion
	var total int64

	db := s.gdb.WithContext(ctx).Model(&types.JobCompletion{})

	// Apply filters
	if query.UserID != "" {
		db = db.Where("user_id = ?", query.UserID)
	}
	if query.SessionID != "" {
		db = db.Where("session_id = ?", query.SessionID)
	}
	if query.CompletionStatus != "" {
		db = db.Where("completion_status = ?", query.CompletionStatus)
	}
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	if query.ReviewNeeded != nil {
		db = db.Where("review_needed = ?", *query.ReviewNeeded)
	}

	// Count total
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// Apply pagination
	offset := query.Page * query.PageSize
	if err := db.Offset(offset).Limit(query.PageSize).Order("created_at DESC").Find(&completions).Error; err != nil {
		return nil, err
	}

	return &types.JobCompletionsListResponse{
		JobCompletions: completions,
		Total:          int(total),
		Page:           query.Page,
		PageSize:       query.PageSize,
	}, nil
}

// GetRecentCompletions returns recent job completions
func (s *PostgresStore) GetRecentCompletions(ctx context.Context, limit int) ([]*types.JobCompletion, error) {
	var completions []*types.JobCompletion
	err := s.gdb.WithContext(ctx).
		Order("created_at DESC").
		Limit(limit).
		Find(&completions).Error
	return completions, err
}

// GetPendingReviews returns job completions that need review
func (s *PostgresStore) GetPendingReviews(ctx context.Context) ([]*types.JobCompletion, error) {
	var completions []*types.JobCompletion
	err := s.gdb.WithContext(ctx).
		Where("status = ?", "pending_review").
		Order("created_at DESC").
		Find(&completions).Error
	return completions, err
}

// MarkSessionAsCompleted marks a session as completed
func (s *PostgresStore) MarkSessionAsCompleted(ctx context.Context, sessionID string, completionType string) error {
	status := "completed"
	if completionType == "pending_review" {
		status = "pending_review"
	}

	return s.gdb.WithContext(ctx).Model(&types.AgentSession{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":       status,
			"completed_at": time.Now(),
			"updated_at":   time.Now(),
		}).Error
}

type ListJobCompletionsQuery struct {
	Page             int
	PageSize         int
	UserID           string
	SessionID        string
	CompletionStatus string
	ReviewNeeded     *bool
	Status           string
}

// GetAgentSessionStatus retrieves an agent session status by session ID
func (s *PostgresStore) GetAgentSessionStatus(ctx context.Context, sessionID string) (*types.AgentSessionStatus, error) {
	var status types.AgentSessionStatus
	err := s.gdb.WithContext(ctx).Where("session_id = ?", sessionID).First(&status).Error
	if err != nil {
		return nil, err
	}
	return &status, nil
}

// CreateAgentSessionStatus creates a new agent session status record
func (s *PostgresStore) CreateAgentSessionStatus(ctx context.Context, status *types.AgentSessionStatus) error {
	return s.gdb.WithContext(ctx).Create(status).Error
}

// UpdateAgentSessionStatus updates an existing agent session status
func (s *PostgresStore) UpdateAgentSessionStatus(ctx context.Context, status *types.AgentSessionStatus) error {
	status.UpdatedAt = time.Now()
	return s.gdb.WithContext(ctx).Save(status).Error
}

// ListAgentSessionStatus returns agent session statuses with pagination and filtering
func (s *PostgresStore) ListAgentSessionStatus(ctx context.Context, query *ListAgentSessionsQuery) (*types.AgentSessionsResponse, error) {
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
	if query.HealthStatus != "" {
		db = db.Where("health_status = ?", query.HealthStatus)
	}
	if query.ActiveOnly {
		db = db.Where("status IN (?)", []string{"starting", "active", "waiting_for_help", "paused"})
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
	if err := s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{}).
		Select("status, COUNT(*) as count").
		Group("status").
		Find(&statusCounts).Error; err != nil {
		return nil, err
	}

	for _, sc := range statusCounts {
		switch sc.Status {
		case "pending":
			stats.TotalPending = sc.Count
		case "in_progress":
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
	if err := s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{}).
		Select("agent_type, COUNT(*) as count").
		Where("status = ?", "pending").
		Group("agent_type").
		Find(&agentTypeCounts).Error; err != nil {
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
	if err := s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{}).
		Select("source, COUNT(*) as count").
		Where("status = ?", "pending").
		Group("source").
		Find(&sourceCounts).Error; err != nil {
		return nil, err
	}

	for _, sc := range sourceCounts {
		stats.BySource[sc.Source] = sc.Count
	}

	// Count by priority
	var priorityCounts []struct {
		Priority int
		Count    int
	}
	if err := s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{}).
		Select("priority, COUNT(*) as count").
		Where("status = ?", "pending").
		Group("priority").
		Find(&priorityCounts).Error; err != nil {
		return nil, err
	}

	for _, pc := range priorityCounts {
		priorityStr := fmt.Sprintf("priority_%d", pc.Priority)
		stats.ByPriority[priorityStr] = pc.Count
	}

	// Count active sessions
	var activeSessionCount int64
	if err := s.gdb.WithContext(ctx).Model(&types.AgentSessionStatus{}).
		Where("status IN (?)", []string{"starting", "active", "waiting_for_help", "paused"}).
		Count(&activeSessionCount).Error; err != nil {
		return nil, err
	}
	stats.ActiveSessions = int(activeSessionCount)

	// Calculate average wait time for pending items
	var pendingCount int64
	if err := s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{}).
		Where("status = ?", "pending").
		Count(&pendingCount).Error; err != nil {
		return nil, err
	}

	if pendingCount > 0 {
		var avgWaitMinutes float64
		if err := s.gdb.WithContext(ctx).Model(&types.AgentWorkItem{}).
			Select("AVG(EXTRACT(EPOCH FROM (NOW() - created_at))/60)").
			Where("status = ?", "pending").
			Scan(&avgWaitMinutes).Error; err != nil {
			return nil, err
		}
		stats.AverageWaitTime = avgWaitMinutes
	}

	// Get oldest pending item
	if pendingCount > 0 {
		var oldestItem types.AgentWorkItem
		if err := s.gdb.WithContext(ctx).
			Where("status = ?", "pending").
			Order("created_at ASC").
			First(&oldestItem).Error; err != nil {
			return nil, err
		}
		stats.OldestPending = &oldestItem.CreatedAt
	}

	return stats, nil
}
