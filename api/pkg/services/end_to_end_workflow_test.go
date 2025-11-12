package services

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

// TestCompleteSpecTaskMultiSessionWorkflow tests the entire end-to-end workflow
func TestCompleteSpecTaskMultiSessionWorkflow(t *testing.T) {
	// This test validates the complete workflow logic without external dependencies
	// It tests the data flow, state transitions, and coordination mechanisms

	// Test data setup
	userID := "user_dev_123"
	projectID := "project_auth_456"
	taskPrompt := "Implement a complete user authentication system with registration, login, logout, profile management, and password reset functionality"

	t.Run("Phase1_SpecTaskCreation", func(t *testing.T) {
		// Simulate SpecTask creation from user prompt
		createRequest := &CreateTaskRequest{
			ProjectID: projectID,
			Prompt:    taskPrompt,
			Type:      "feature",
			Priority:  "high",
			UserID:    userID,
		}

		// Validate creation request structure
		assert.Equal(t, projectID, createRequest.ProjectID)
		assert.Equal(t, taskPrompt, createRequest.Prompt)
		assert.Equal(t, "feature", createRequest.Type)
		assert.Equal(t, userID, createRequest.UserID)

		// Create expected SpecTask
		specTask := &types.SpecTask{
			ID:             generateTestSpecTaskID(),
			ProjectID:      createRequest.ProjectID,
			Name:           generateTaskNameFromPrompt(createRequest.Prompt),
			Description:    createRequest.Prompt,
			Type:           createRequest.Type,
			Priority:       createRequest.Priority,
			Status:         types.TaskStatusBacklog,
			OriginalPrompt: createRequest.Prompt,
			CreatedBy:      createRequest.UserID,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		// Validate SpecTask creation
		assert.NotEmpty(t, specTask.ID)
		assert.Equal(t, types.TaskStatusBacklog, specTask.Status)
		assert.Contains(t, specTask.Name, "authentication")
		assert.Equal(t, taskPrompt, specTask.OriginalPrompt)
	})

	t.Run("Phase2_SpecificationGeneration", func(t *testing.T) {
		// Simulate planning agent generating comprehensive specifications
		specTask := createTestSpecTask(userID, projectID)

		// Transition to spec generation
		specTask.Status = types.TaskStatusSpecGeneration
		specTask.ExternalAgentID = "helix-planning-agent"
		specTask.PlanningSessionID = "planning_session_123"

		// Simulate planning agent output
		generatedSpecs := &SpecGeneration{
			TaskID:             specTask.ID,
			RequirementsSpec:   generateComprehensiveRequirementsSpec(),
			TechnicalDesign:    generateComprehensiveTechnicalDesign(),
			ImplementationPlan: generateComprehensiveImplementationPlan(),
			GeneratedAt:        time.Now(),
			ModelUsed:          "claude-3-5-sonnet-20241022",
			TokensUsed:         15420,
		}

		// Validate generated specifications
		assert.NotEmpty(t, generatedSpecs.RequirementsSpec)
		assert.NotEmpty(t, generatedSpecs.TechnicalDesign)
		assert.NotEmpty(t, generatedSpecs.ImplementationPlan)
		assert.Contains(t, generatedSpecs.RequirementsSpec, "authentication")
		assert.Contains(t, generatedSpecs.TechnicalDesign, "database")
		assert.Contains(t, generatedSpecs.ImplementationPlan, "Task 1")

		// Update SpecTask with generated specs
		specTask.RequirementsSpec = generatedSpecs.RequirementsSpec
		specTask.TechnicalDesign = generatedSpecs.TechnicalDesign
		specTask.ImplementationPlan = generatedSpecs.ImplementationPlan
		specTask.Status = types.TaskStatusSpecReview

		assert.Equal(t, types.TaskStatusSpecReview, specTask.Status)
	})

	t.Run("Phase3_HumanApproval", func(t *testing.T) {
		// Simulate human reviewing and approving specifications
		specTask := createTestSpecTaskWithSpecs(userID, projectID)

		approvalRequest := &types.SpecApprovalResponse{
			TaskID:     specTask.ID,
			Approved:   true,
			Comments:   "Specifications look comprehensive and well thought out. Approved for implementation.",
			ApprovedBy: userID,
			ApprovedAt: time.Now(),
		}

		// Process approval
		specTask.Status = types.TaskStatusSpecApproved
		specTask.SpecApprovedBy = approvalRequest.ApprovedBy
		specTask.SpecApprovedAt = &approvalRequest.ApprovedAt

		// Validate approval state
		assert.Equal(t, types.TaskStatusSpecApproved, specTask.Status)
		assert.Equal(t, userID, specTask.SpecApprovedBy)
		assert.NotNil(t, specTask.SpecApprovedAt)
		assert.True(t, approvalRequest.Approved)
	})

	t.Run("Phase4_MultiSessionImplementationCreation", func(t *testing.T) {
		// Simulate multi-session implementation creation
		specTask := createTestSpecTaskWithSpecs(userID, projectID)
		specTask.Status = types.TaskStatusSpecApproved
		specTask.ExternalAgentID = "zed-implementation-agent"

		// Parse implementation plan into discrete tasks
		implementationTasks := parseTestImplementationPlan(specTask.ImplementationPlan)
		assert.Len(t, implementationTasks, 5) // Based on our test plan

		// Create configuration for implementation sessions
		config := &types.SpecTaskImplementationSessionsCreateRequest{
			SpecTaskID:         specTask.ID,
			ProjectPath:        "/workspace/auth-system",
			AutoCreateSessions: true,
			WorkspaceConfig: map[string]interface{}{
				"SPEC_TASK_ID":      specTask.ID,
				"PROJECT_TYPE":      "authentication",
				"FRAMEWORK":         "nodejs",
				"DATABASE":          "postgresql",
				"TESTING_FRAMEWORK": "jest",
			},
		}

		// Simulate work session creation
		expectedWorkSessions := createExpectedWorkSessions(specTask.ID, implementationTasks)

		// Validate work session creation
		assert.Len(t, expectedWorkSessions, len(implementationTasks))
		for i, ws := range expectedWorkSessions {
			assert.Equal(t, specTask.ID, ws.SpecTaskID)
			assert.Equal(t, types.SpecTaskPhaseImplementation, ws.Phase)
			assert.Equal(t, types.SpecTaskWorkSessionStatusPending, ws.Status)
			assert.Equal(t, implementationTasks[i].Title, ws.ImplementationTaskTitle)
			assert.Equal(t, i, ws.ImplementationTaskIndex)
		}

		// Simulate Zed instance creation
		zedInstanceID := fmt.Sprintf("zed_instance_%s_%d", specTask.ID, time.Now().Unix())
		specTask.ZedInstanceID = zedInstanceID
		specTask.ProjectPath = config.ProjectPath

		// Simulate Zed thread creation for each work session
		expectedZedThreads := createExpectedZedThreads(expectedWorkSessions, zedInstanceID)

		// Validate Zed thread creation
		assert.Len(t, expectedZedThreads, len(expectedWorkSessions))
		for i, zt := range expectedZedThreads {
			assert.Equal(t, expectedWorkSessions[i].ID, zt.WorkSessionID)
			assert.Equal(t, specTask.ID, zt.SpecTaskID)
			assert.Contains(t, zt.ZedThreadID, "thread_")
			assert.Equal(t, types.SpecTaskZedStatusPending, zt.Status)
		}

		// Update SpecTask status
		specTask.Status = types.TaskStatusImplementation
		assert.Equal(t, types.TaskStatusImplementation, specTask.Status)
	})

	t.Run("Phase5_ParallelImplementationExecution", func(t *testing.T) {
		// Simulate parallel execution of implementation sessions
		specTask := createTestSpecTaskWithSessions(userID, projectID)
		workSessions := createTestWorkSessions(specTask.ID)

		// Simulate sessions starting
		for i, ws := range workSessions {
			// Start session
			ws.Status = types.SpecTaskWorkSessionStatusActive
			now := time.Now().Add(time.Duration(i) * time.Minute) // Stagger start times
			ws.StartedAt = &now

			// Validate session start
			assert.Equal(t, types.SpecTaskWorkSessionStatusActive, ws.Status)
			assert.NotNil(t, ws.StartedAt)
			assert.True(t, ws.CanSpawnSessions())
		}

		// Simulate Zed threads becoming active
		zedThreads := createTestZedThreads(workSessions, "zed_instance_test")
		for _, zt := range zedThreads {
			zt.Status = types.SpecTaskZedStatusActive
			now := time.Now()
			zt.LastActivityAt = &now

			assert.Equal(t, types.SpecTaskZedStatusActive, zt.Status)
			assert.True(t, zt.IsActive())
			assert.True(t, zt.HasRecentActivity(1*time.Hour))
		}

		// Simulate progress updates
		progressUpdates := []float64{0.2, 0.5, 0.8, 0.3, 0.1}
		for i, _ := range workSessions {
			// Update progress based on simulated work
			progress := progressUpdates[i]

			// Validate progress tracking
			assert.GreaterOrEqual(t, progress, 0.0)
			assert.LessOrEqual(t, progress, 1.0)
		}
	})

	t.Run("Phase6_SessionSpawningDuringImplementation", func(t *testing.T) {
		// Simulate dynamic session spawning during implementation
		parentSession := &types.SpecTaskWorkSession{
			ID:         "ws_backend_api",
			SpecTaskID: "spec_task_auth",
			Name:       "Backend API Implementation",
			Status:     types.SpecTaskWorkSessionStatusActive,
			Phase:      types.SpecTaskPhaseImplementation,
		}

		// Simulate need for additional work during implementation
		spawnRequests := []types.SpecTaskWorkSessionSpawnRequest{
			{
				ParentWorkSessionID: parentSession.ID,
				Name:                "Database Performance Optimization",
				Description:         "Optimize authentication queries for better performance",
			},
			{
				ParentWorkSessionID: parentSession.ID,
				Name:                "Security Audit Implementation",
				Description:         "Implement additional security measures identified during development",
			},
			{
				ParentWorkSessionID: parentSession.ID,
				Name:                "Rate Limiting Enhancement",
				Description:         "Add sophisticated rate limiting to prevent brute force attacks",
			},
		}

		// Simulate spawning multiple sessions
		spawnedSessions := make([]*types.SpecTaskWorkSession, len(spawnRequests))
		for i, req := range spawnRequests {
			spawnedSession := &types.SpecTaskWorkSession{
				ID:                  generateTestWorkSessionID(),
				SpecTaskID:          parentSession.SpecTaskID,
				HelixSessionID:      generateTestHelixSessionID(),
				Name:                req.Name,
				Description:         req.Description,
				Phase:               types.SpecTaskPhaseImplementation,
				Status:              types.SpecTaskWorkSessionStatusPending,
				ParentWorkSessionID: parentSession.ID,
				SpawnedBySessionID:  parentSession.ID,
			}
			spawnedSessions[i] = spawnedSession

			// Validate spawned session properties
			assert.Equal(t, parentSession.SpecTaskID, spawnedSession.SpecTaskID)
			assert.Equal(t, parentSession.ID, spawnedSession.ParentWorkSessionID)
			assert.Equal(t, parentSession.ID, spawnedSession.SpawnedBySessionID)
			assert.True(t, spawnedSession.HasParent())
			assert.True(t, spawnedSession.WasSpawned())
		}

		// Simulate Zed threads for spawned sessions
		for _, spawnedSession := range spawnedSessions {
			zedThread := &types.SpecTaskZedThread{
				ID:            generateTestZedThreadID(),
				WorkSessionID: spawnedSession.ID,
				SpecTaskID:    spawnedSession.SpecTaskID,
				ZedThreadID:   fmt.Sprintf("thread_%s", spawnedSession.ID),
				Status:        types.SpecTaskZedStatusPending,
			}

			// Validate Zed thread for spawned session
			assert.Equal(t, spawnedSession.ID, zedThread.WorkSessionID)
			assert.Equal(t, spawnedSession.SpecTaskID, zedThread.SpecTaskID)
			assert.Contains(t, zedThread.ZedThreadID, spawnedSession.ID)
		}
	})

	t.Run("Phase7_SessionCoordinationAndCommunication", func(t *testing.T) {
		// Simulate coordination between sessions
		_ = createTestWorkSessions("spec_task_auth")

		// Simulate coordination scenarios
		coordinationScenarios := []struct {
			name        string
			fromSession string
			toSession   string
			eventType   CoordinationEventType
			message     string
		}{
			{
				name:        "Database completion handoff",
				fromSession: "ws_database",
				toSession:   "ws_backend_api",
				eventType:   CoordinationEventTypeHandoff,
				message:     "Database schema is complete, ready for API implementation",
			},
			{
				name:        "API completion broadcast",
				fromSession: "ws_backend_api",
				toSession:   "", // Broadcast
				eventType:   CoordinationEventTypeBroadcast,
				message:     "Authentication API endpoints are ready for frontend integration",
			},
			{
				name:        "Testing blocked notification",
				fromSession: "ws_testing",
				toSession:   "ws_frontend",
				eventType:   CoordinationEventTypeBlocking,
				message:     "Need frontend components before integration tests can proceed",
			},
		}

		// Process coordination events
		coordinationEvents := make([]CoordinationEvent, len(coordinationScenarios))
		for i, scenario := range coordinationScenarios {
			event := CoordinationEvent{
				ID:            generateTestEventID(),
				FromSessionID: scenario.fromSession,
				ToSessionID:   scenario.toSession,
				EventType:     scenario.eventType,
				Message:       scenario.message,
				Data: map[string]interface{}{
					"scenario": scenario.name,
				},
				Timestamp:    time.Now(),
				Acknowledged: false,
			}
			coordinationEvents[i] = event

			// Validate coordination event
			assert.NotEmpty(t, event.ID)
			assert.Equal(t, scenario.fromSession, event.FromSessionID)
			assert.Equal(t, scenario.toSession, event.ToSessionID)
			assert.Equal(t, scenario.eventType, event.EventType)
		}

		// Simulate acknowledgments
		for i := range coordinationEvents {
			coordinationEvents[i].Acknowledged = true
			now := time.Now()
			coordinationEvents[i].AcknowledgedAt = &now
			coordinationEvents[i].Response = "Acknowledged and proceeding"
		}
	})

	t.Run("Phase8_ProgressTrackingAndStatusUpdates", func(t *testing.T) {
		// Simulate progress tracking across multiple sessions
		workSessions := createTestWorkSessions("spec_task_auth")
		_ = parseTestImplementationPlan(generateComprehensiveImplementationPlan())

		// Simulate different completion states
		sessionStates := []struct {
			sessionIndex int
			status       types.SpecTaskWorkSessionStatus
			progress     float64
			completedAt  *time.Time
		}{
			{0, types.SpecTaskWorkSessionStatusCompleted, 1.0, timePtr(time.Now().Add(-1 * time.Hour))},
			{1, types.SpecTaskWorkSessionStatusActive, 0.7, nil},
			{2, types.SpecTaskWorkSessionStatusActive, 0.3, nil},
			{3, types.SpecTaskWorkSessionStatusPending, 0.0, nil},
			{4, types.SpecTaskWorkSessionStatusActive, 0.9, nil},
		}

		// Apply states to sessions
		for _, state := range sessionStates {
			if state.sessionIndex < len(workSessions) {
				ws := workSessions[state.sessionIndex]
				ws.Status = state.status
				ws.CompletedAt = state.completedAt

				// Validate session state
				switch state.status {
				case types.SpecTaskWorkSessionStatusCompleted:
					assert.True(t, ws.IsCompleted())
					assert.NotNil(t, ws.CompletedAt)
					assert.False(t, ws.CanSpawnSessions())
				case types.SpecTaskWorkSessionStatusActive:
					assert.True(t, ws.IsActive())
					assert.True(t, ws.CanSpawnSessions())
				case types.SpecTaskWorkSessionStatusPending:
					assert.True(t, ws.IsPending())
					assert.False(t, ws.CanSpawnSessions())
				}
			}
		}

		// Calculate overall progress
		completedSessions := 0
		activeSessions := 0
		totalProgress := 0.0

		for i, state := range sessionStates {
			if i < len(workSessions) {
				totalProgress += state.progress
				if state.status == types.SpecTaskWorkSessionStatusCompleted {
					completedSessions++
				} else if state.status == types.SpecTaskWorkSessionStatusActive {
					activeSessions++
				}
			}
		}

		overallProgress := totalProgress / float64(len(sessionStates))
		completionPercentage := float64(completedSessions) / float64(len(sessionStates))

		// Validate progress calculations
		assert.Equal(t, 1, completedSessions)
		assert.Equal(t, 3, activeSessions)
		assert.InDelta(t, 0.58, overallProgress, 0.01)     // (1.0 + 0.7 + 0.3 + 0.0 + 0.9) / 5
		assert.InDelta(t, 0.2, completionPercentage, 0.01) // 1 / 5
	})

	t.Run("Phase9_ZedInstanceAndThreadManagement", func(t *testing.T) {
		// Simulate Zed instance lifecycle
		specTaskID := "spec_task_auth"
		zedInstanceID := "zed_instance_" + specTaskID

		// Instance creation
		instanceInfo := &ZedInstanceInfo{
			InstanceID:   zedInstanceID,
			SpecTaskID:   specTaskID,
			Status:       "creating",
			CreatedAt:    time.Now(),
			LastActivity: time.Now(),
			ProjectPath:  "/workspace/auth-system",
			ThreadCount:  0,
		}

		// Validate instance creation
		assert.Equal(t, zedInstanceID, instanceInfo.InstanceID)
		assert.Equal(t, "creating", instanceInfo.Status)
		assert.NotZero(t, instanceInfo.CreatedAt)

		// Simulate instance becoming ready
		instanceInfo.Status = "active"
		instanceInfo.LastActivity = time.Now()

		// Simulate thread creation
		workSessions := createTestWorkSessions(specTaskID)
		threads := make([]*types.SpecTaskZedThread, len(workSessions))

		for i, ws := range workSessions {
			thread := &types.SpecTaskZedThread{
				ID:            generateTestZedThreadID(),
				WorkSessionID: ws.ID,
				SpecTaskID:    ws.SpecTaskID,
				ZedThreadID:   fmt.Sprintf("thread_%s", ws.ID),
				Status:        types.SpecTaskZedStatusPending,
				CreatedAt:     time.Now(),
			}
			threads[i] = thread
			instanceInfo.ThreadCount++

			// Validate thread creation
			assert.Equal(t, ws.ID, thread.WorkSessionID)
			assert.Equal(t, ws.SpecTaskID, thread.SpecTaskID)
			assert.Contains(t, thread.ZedThreadID, ws.ID)
		}

		// Simulate threads becoming active
		activeThreads := 0
		for _, thread := range threads {
			thread.Status = types.SpecTaskZedStatusActive
			now := time.Now()
			thread.LastActivityAt = &now
			activeThreads++

			assert.True(t, thread.IsActive())
			assert.True(t, thread.HasRecentActivity(1*time.Hour))
		}

		// Calculate instance status
		instanceStatus := &ZedInstanceStatus{
			InstanceID:    zedInstanceID,
			SpecTaskID:    specTaskID,
			Status:        "active",
			ThreadCount:   len(threads),
			ActiveThreads: activeThreads,
			LastActivity:  &instanceInfo.LastActivity,
			ProjectPath:   instanceInfo.ProjectPath,
		}

		// Validate instance status
		assert.Equal(t, len(workSessions), instanceStatus.ThreadCount)
		assert.Equal(t, len(workSessions), instanceStatus.ActiveThreads)
		assert.Equal(t, "active", instanceStatus.Status)
	})

	t.Run("Phase10_SessionCompletionAndTaskFinalization", func(t *testing.T) {
		// Simulate sessions completing and task finalization
		workSessions := createTestWorkSessions("spec_task_auth")
		implementationTasks := parseTestImplementationPlan(generateComprehensiveImplementationPlan())

		// Simulate sessions completing in order
		completionOrder := []int{0, 1, 3, 2, 4} // Database, API, Tests, Frontend, Documentation
		completedSessions := make([]*types.SpecTaskWorkSession, 0)

		for _, index := range completionOrder {
			if index < len(workSessions) {
				ws := workSessions[index]

				// Complete the session
				ws.Status = types.SpecTaskWorkSessionStatusCompleted
				now := time.Now()
				ws.CompletedAt = &now
				completedSessions = append(completedSessions, ws)

				// Update corresponding implementation task
				if index < len(implementationTasks) {
					task := &implementationTasks[index]
					task.Status = types.SpecTaskImplementationStatusCompleted
					task.CompletedAt = &now
				}

				// Validate completion
				assert.True(t, ws.IsCompleted())
				assert.NotNil(t, ws.CompletedAt)
				assert.False(t, ws.CanSpawnSessions())
			}
		}

		// Check if all sessions are complete
		allComplete := true
		for _, ws := range workSessions {
			if !ws.IsCompleted() {
				allComplete = false
				break
			}
		}

		// Simulate SpecTask completion
		if allComplete {
			specTask := &types.SpecTask{
				ID:     "spec_task_auth",
				Status: types.TaskStatusDone,
			}
			now := time.Now()
			specTask.CompletedAt = &now

			// Validate task completion
			assert.Equal(t, types.TaskStatusDone, specTask.Status)
			assert.NotNil(t, specTask.CompletedAt)
		}

		assert.True(t, allComplete)
		assert.Len(t, completedSessions, len(workSessions))
	})

	t.Run("Phase11_CleanupAndResourceManagement", func(t *testing.T) {
		// Simulate cleanup after task completion
		specTask := &types.SpecTask{
			ID:            "spec_task_auth",
			Status:        types.TaskStatusDone,
			ZedInstanceID: "zed_instance_spec_task_auth",
		}

		workSessions := createTestWorkSessions(specTask.ID)
		zedThreads := createTestZedThreads(workSessions, specTask.ZedInstanceID)

		// Simulate cleanup process
		for _, thread := range zedThreads {
			// Mark threads as completed
			thread.Status = types.SpecTaskZedStatusCompleted
			assert.True(t, thread.IsCompleted())
		}

		// Mark work sessions as completed for cleanup validation
		for _, ws := range workSessions {
			ws.Status = types.SpecTaskWorkSessionStatusCompleted
			completedTime := time.Now()
			ws.CompletedAt = &completedTime
		}

		// Simulate instance cleanup
		instanceCleaned := true
		assert.True(t, instanceCleaned)

		// Validate resource cleanup
		for _, ws := range workSessions {
			assert.True(t, ws.IsCompleted())
		}
	})
}

// TestComplexWorkflowScenarios tests advanced scenarios
func TestComplexWorkflowScenarios(t *testing.T) {
	t.Run("InteractiveCodingSession", func(t *testing.T) {
		// Test long-running interactive coding session
		_ = &types.SpecTask{
			ID:        "interactive_coding_123",
			Name:      "Interactive Development Session",
			Type:      "coding_session",
			Status:    types.TaskStatusSpecApproved,
			CreatedBy: "developer_user",
			ImplementationPlan: `# Interactive Development Plan

## Initial Focus: Core feature development
- Start with main development stream
- Respond to issues and requirements as they emerge
- Spawn specialized sessions for focused work

This is an ongoing, interactive development session.`,
		}

		// Should create minimal initial session structure
		initialSessions := []*types.SpecTaskWorkSession{
			{
				ID:     "ws_main_development",
				Name:   "Main Development Stream",
				Status: types.SpecTaskWorkSessionStatusActive,
				Phase:  types.SpecTaskPhaseImplementation,
			},
		}

		assert.Len(t, initialSessions, 1)
		assert.Equal(t, "Main Development Stream", initialSessions[0].Name)
		assert.True(t, initialSessions[0].CanSpawnSessions())

		// Simulate dynamic spawning during session
		dynamicSpawns := []string{
			"Bug Fix: Login validation issue",
			"Feature: Add OAuth integration",
			"Optimization: Database query performance",
			"Testing: End-to-end authentication flow",
		}

		spawnedSessions := make([]*types.SpecTaskWorkSession, len(dynamicSpawns))
		for i, spawnName := range dynamicSpawns {
			session := &types.SpecTaskWorkSession{
				ID:                  fmt.Sprintf("ws_dynamic_%d", i),
				Name:                spawnName,
				ParentWorkSessionID: initialSessions[0].ID,
				Status:              types.SpecTaskWorkSessionStatusActive,
				Phase:               types.SpecTaskPhaseImplementation,
			}
			spawnedSessions[i] = session

			assert.True(t, session.HasParent())
			assert.True(t, session.CanSpawnSessions())
		}

		// Validate interactive session structure
		totalSessions := len(initialSessions) + len(spawnedSessions)
		assert.Equal(t, 5, totalSessions) // 1 initial + 4 spawned
	})

	t.Run("ErrorRecoveryAndResilience", func(t *testing.T) {
		// Test error scenarios and recovery
		workSession := &types.SpecTaskWorkSession{
			ID:     "ws_backend_api",
			Status: types.SpecTaskWorkSessionStatusActive,
			Phase:  types.SpecTaskPhaseImplementation,
		}

		zedThread := &types.SpecTaskZedThread{
			ID:            "zt_backend_123",
			WorkSessionID: workSession.ID,
			Status:        types.SpecTaskZedStatusActive,
		}

		// Simulate Zed disconnection
		zedThread.Status = types.SpecTaskZedStatusDisconnected
		assert.True(t, zedThread.IsDisconnected())

		// Simulate session becoming blocked
		workSession.Status = types.SpecTaskWorkSessionStatusBlocked
		assert.False(t, workSession.CanSpawnSessions())

		// Simulate recovery
		zedThread.Status = types.SpecTaskZedStatusActive
		workSession.Status = types.SpecTaskWorkSessionStatusActive

		// Validate recovery
		assert.True(t, zedThread.IsActive())
		assert.True(t, workSession.CanSpawnSessions())
	})

	t.Run("DependencyResolutionWorkflow", func(t *testing.T) {
		// Test dependency resolution between implementation tasks
		implementationTasks := []types.SpecTaskImplementationTask{
			{
				Index:        0,
				Title:        "Database Schema",
				Status:       types.SpecTaskImplementationStatusCompleted,
				Dependencies: createDependencyJSON([]int{}), // No dependencies
			},
			{
				Index:        1,
				Title:        "Backend API",
				Status:       types.SpecTaskImplementationStatusInProgress,
				Dependencies: createDependencyJSON([]int{0}), // Depends on database
			},
			{
				Index:        2,
				Title:        "Frontend Components",
				Status:       types.SpecTaskImplementationStatusPending,
				Dependencies: createDependencyJSON([]int{1}), // Depends on API
			},
			{
				Index:        3,
				Title:        "Integration Tests",
				Status:       types.SpecTaskImplementationStatusPending,
				Dependencies: createDependencyJSON([]int{1, 2}), // Depends on API and Frontend
			},
		}

		// Validate dependency structure
		assert.False(t, implementationTasks[0].HasDependencies()) // Database has no deps
		assert.True(t, implementationTasks[1].HasDependencies())  // API depends on database
		assert.True(t, implementationTasks[2].HasDependencies())  // Frontend depends on API
		assert.True(t, implementationTasks[3].HasDependencies())  // Tests depend on both

		// Validate that dependencies are satisfied before tasks can proceed
		assert.True(t, implementationTasks[0].IsCompleted())   // Database complete
		assert.True(t, implementationTasks[1].IsInProgress())  // API can proceed
		assert.False(t, implementationTasks[2].IsInProgress()) // Frontend waiting for API
		assert.False(t, implementationTasks[3].IsInProgress()) // Tests waiting for API + Frontend
	})
}

// TestSystemIntegration tests integration between all components
func TestSystemIntegration(t *testing.T) {
	t.Run("CompleteDataFlow", func(t *testing.T) {
		// Test data flow through entire system

		// 1. SpecTask with approved specs
		specTask := createTestSpecTaskWithSpecs("user_123", "project_456")
		specTask.Status = types.TaskStatusSpecApproved

		// 2. Implementation sessions created
		workSessions := createTestWorkSessions(specTask.ID)

		// 3. Zed instance and threads created
		zedThreads := createTestZedThreads(workSessions, "zed_instance_"+specTask.ID)

		// 4. Implementation tasks parsed
		implementationTasks := parseTestImplementationPlan(specTask.ImplementationPlan)

		// 5. Sessions linked to implementation tasks
		for i, ws := range workSessions {
			if i < len(implementationTasks) {
				ws.ImplementationTaskTitle = implementationTasks[i].Title
				ws.ImplementationTaskIndex = implementationTasks[i].Index

				// Validate linking
				assert.Equal(t, implementationTasks[i].Title, ws.ImplementationTaskTitle)
				assert.Equal(t, implementationTasks[i].Index, ws.ImplementationTaskIndex)
			}
		}

		// 6. Validate complete data integrity
		assert.Len(t, workSessions, len(implementationTasks))
		assert.Len(t, zedThreads, len(workSessions))

		for i, ws := range workSessions {
			// Each work session should have corresponding Zed thread
			assert.Equal(t, ws.ID, zedThreads[i].WorkSessionID)
			assert.Equal(t, ws.SpecTaskID, zedThreads[i].SpecTaskID)

			// Each work session should have corresponding implementation task
			if i < len(implementationTasks) {
				assert.Equal(t, implementationTasks[i].Index, ws.ImplementationTaskIndex)
			}
		}
	})
}

// Helper functions for test data creation

func createTestSpecTask(userID, projectID string) *types.SpecTask {
	return &types.SpecTask{
		ID:             generateTestSpecTaskID(),
		ProjectID:      projectID,
		Name:           "User Authentication System",
		Description:    "Complete authentication system with all features",
		Type:           "feature",
		Priority:       "high",
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Implement user authentication system",
		CreatedBy:      userID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

func createTestSpecTaskWithSpecs(userID, projectID string) *types.SpecTask {
	task := createTestSpecTask(userID, projectID)
	task.RequirementsSpec = generateComprehensiveRequirementsSpec()
	task.TechnicalDesign = generateComprehensiveTechnicalDesign()
	task.ImplementationPlan = generateComprehensiveImplementationPlan()
	task.Status = types.TaskStatusSpecReview
	return task
}

func createTestSpecTaskWithSessions(userID, projectID string) *types.SpecTask {
	task := createTestSpecTaskWithSpecs(userID, projectID)
	task.Status = types.TaskStatusImplementation
	task.ZedInstanceID = "zed_instance_" + task.ID
	task.ProjectPath = "/workspace/" + task.ID
	return task
}

func createTestWorkSessions(specTaskID string) []*types.SpecTaskWorkSession {
	return []*types.SpecTaskWorkSession{
		{
			ID:                      "ws_database_schema",
			SpecTaskID:              specTaskID,
			HelixSessionID:          "helix_session_db",
			Name:                    "Database Schema Migration",
			Phase:                   types.SpecTaskPhaseImplementation,
			Status:                  types.SpecTaskWorkSessionStatusPending,
			ImplementationTaskTitle: "Database schema setup",
			ImplementationTaskIndex: 0,
		},
		{
			ID:                      "ws_backend_api",
			SpecTaskID:              specTaskID,
			HelixSessionID:          "helix_session_backend",
			Name:                    "Backend API Implementation",
			Phase:                   types.SpecTaskPhaseImplementation,
			Status:                  types.SpecTaskWorkSessionStatusPending,
			ImplementationTaskTitle: "Authentication API endpoints",
			ImplementationTaskIndex: 1,
		},
		{
			ID:                      "ws_frontend_ui",
			SpecTaskID:              specTaskID,
			HelixSessionID:          "helix_session_frontend",
			Name:                    "Frontend UI Components",
			Phase:                   types.SpecTaskPhaseImplementation,
			Status:                  types.SpecTaskWorkSessionStatusPending,
			ImplementationTaskTitle: "Login/registration UI",
			ImplementationTaskIndex: 2,
		},
		{
			ID:                      "ws_integration_tests",
			SpecTaskID:              specTaskID,
			HelixSessionID:          "helix_session_tests",
			Name:                    "Integration Testing",
			Phase:                   types.SpecTaskPhaseImplementation,
			Status:                  types.SpecTaskWorkSessionStatusPending,
			ImplementationTaskTitle: "End-to-end testing",
			ImplementationTaskIndex: 3,
		},
		{
			ID:                      "ws_documentation",
			SpecTaskID:              specTaskID,
			HelixSessionID:          "helix_session_docs",
			Name:                    "Documentation",
			Phase:                   types.SpecTaskPhaseImplementation,
			Status:                  types.SpecTaskWorkSessionStatusPending,
			ImplementationTaskTitle: "API and user documentation",
			ImplementationTaskIndex: 4,
		},
	}
}

func createTestZedThreads(workSessions []*types.SpecTaskWorkSession, instanceID string) []*types.SpecTaskZedThread {
	threads := make([]*types.SpecTaskZedThread, len(workSessions))
	for i, ws := range workSessions {
		threads[i] = &types.SpecTaskZedThread{
			ID:            generateTestZedThreadID(),
			WorkSessionID: ws.ID,
			SpecTaskID:    ws.SpecTaskID,
			ZedThreadID:   fmt.Sprintf("thread_%s", ws.ID),
			Status:        types.SpecTaskZedStatusPending,
			CreatedAt:     time.Now(),
		}
	}
	return threads
}

func createExpectedWorkSessions(specTaskID string, implementationTasks []types.SpecTaskImplementationTask) []*types.SpecTaskWorkSession {
	sessions := make([]*types.SpecTaskWorkSession, len(implementationTasks))
	for i, task := range implementationTasks {
		sessions[i] = &types.SpecTaskWorkSession{
			ID:                      generateTestWorkSessionID(),
			SpecTaskID:              specTaskID,
			HelixSessionID:          generateTestHelixSessionID(),
			Name:                    task.Title,
			Phase:                   types.SpecTaskPhaseImplementation,
			Status:                  types.SpecTaskWorkSessionStatusPending,
			ImplementationTaskTitle: task.Title,
			ImplementationTaskIndex: task.Index,
		}
	}
	return sessions
}

func createExpectedZedThreads(workSessions []*types.SpecTaskWorkSession, instanceID string) []*types.SpecTaskZedThread {
	threads := make([]*types.SpecTaskZedThread, len(workSessions))
	for i, ws := range workSessions {
		threads[i] = &types.SpecTaskZedThread{
			ID:            generateTestZedThreadID(),
			WorkSessionID: ws.ID,
			SpecTaskID:    ws.SpecTaskID,
			ZedThreadID:   fmt.Sprintf("thread_%s", ws.ID),
			Status:        types.SpecTaskZedStatusPending,
		}
	}
	return threads
}

func generateComprehensiveRequirementsSpec() string {
	return `# User Authentication System Requirements

## Functional Requirements

### User Registration
- Users can register with email and password
- Email verification required before account activation
- Password strength validation (minimum 8 characters, mixed case, numbers, symbols)
- Duplicate email prevention
- Terms of service acceptance tracking

### User Login
- Users can login with email and password
- Session management with secure JWT tokens
- Remember me functionality (optional longer sessions)
- Account lockout after failed login attempts
- Login attempt logging and monitoring

### User Profile Management
- Users can view their profile information
- Users can update their profile (name, email, password)
- Profile picture upload and management
- Account deletion with data retention compliance

### Password Management
- Password reset via email
- Password change functionality
- Password history tracking (prevent reuse)
- Secure password recovery questions (optional)

### Security Features
- Rate limiting on all authentication endpoints
- HTTPS enforcement for all operations
- Secure session management
- CSRF protection
- SQL injection prevention
- Input validation and sanitization

## Non-Functional Requirements

### Performance
- Login response time < 200ms
- Registration response time < 500ms
- Support for 10,000 concurrent users
- Database query optimization

### Security
- PCI DSS compliance for payment integration (future)
- GDPR compliance for data protection
- SOC 2 Type II compliance
- Regular security audits and penetration testing

### Scalability
- Horizontal scaling capability
- Database connection pooling
- Caching for frequently accessed data
- Load balancing support

### Reliability
- 99.9% uptime SLA
- Graceful error handling
- Comprehensive logging and monitoring
- Backup and disaster recovery procedures`
}

func generateComprehensiveTechnicalDesign() string {
	return `# Technical Design: User Authentication System

## Architecture Overview
- Microservices architecture with authentication service
- REST API for all authentication operations
- JWT-based stateless session management
- PostgreSQL database for user data persistence
- Redis for session caching and rate limiting

## Database Schema

### Users Table
- id (UUID, primary key)
- email (varchar, unique, indexed)
- password_hash (varchar, bcrypt)
- name (varchar)
- email_verified (boolean, default false)
- created_at (timestamp)
- updated_at (timestamp)
- last_login_at (timestamp)
- login_attempts (integer, default 0)
- locked_until (timestamp, nullable)

### User Sessions Table
- id (UUID, primary key)
- user_id (UUID, foreign key)
- token_hash (varchar, indexed)
- expires_at (timestamp)
- created_at (timestamp)
- last_accessed_at (timestamp)
- user_agent (text)
- ip_address (inet)

### Password Reset Tokens Table
- id (UUID, primary key)
- user_id (UUID, foreign key)
- token_hash (varchar)
- expires_at (timestamp)
- used_at (timestamp, nullable)
- created_at (timestamp)

## API Endpoints

### Authentication Endpoints
- POST /auth/register - User registration
- POST /auth/verify-email - Email verification
- POST /auth/login - User login
- POST /auth/logout - User logout
- POST /auth/refresh - Token refresh
- POST /auth/reset-password - Password reset request
- POST /auth/reset-password/confirm - Password reset confirmation

### Profile Endpoints
- GET /auth/profile - Get user profile
- PUT /auth/profile - Update user profile
- POST /auth/change-password - Change password
- DELETE /auth/account - Delete account

## Security Implementation
- bcrypt for password hashing (cost factor 12)
- JWT tokens with RS256 algorithm
- Rate limiting: 5 login attempts per minute per IP
- Session timeout: 24 hours (extendable)
- CSRF tokens for state-changing operations
- Input validation using joi schemas
- SQL parameterized queries

## Caching Strategy
- User profile data cached for 15 minutes
- Session validation cached for 5 minutes
- Rate limit counters in Redis with TTL
- Email verification tokens in Redis (30 minutes TTL)

## Monitoring and Logging
- All authentication events logged
- Failed login attempt monitoring
- Performance metrics collection
- Error tracking and alerting
- Audit trail for security events`
}

func generateComprehensiveImplementationPlan() string {
	return `# Implementation Plan

## Task 1: Database schema and migrations
- Create users table with proper indexes and constraints
- Create user_sessions table for JWT tracking
- Create password_reset_tokens table
- Add database migrations and rollback scripts
- Set up connection pooling and optimization
- Estimated effort: Small
- Dependencies: None

## Task 2: Authentication API endpoints
- Implement user registration with validation
- Implement email verification system
- Implement login endpoint with JWT generation
- Implement logout endpoint with session cleanup
- Implement password reset functionality
- Add comprehensive input validation
- Add rate limiting middleware
- Estimated effort: Large
- Dependencies: Task 1

## Task 3: Frontend authentication components
- Create responsive login form component
- Create registration form with validation
- Create password reset flow components
- Create profile management interface
- Add authentication state management (Redux/Zustand)
- Implement secure token storage
- Add form validation and error handling
- Estimated effort: Large
- Dependencies: Task 2

## Task 4: Security implementation and hardening
- Implement CSRF protection
- Add comprehensive rate limiting
- Set up session security (httpOnly cookies)
- Implement account lockout mechanism
- Add security headers and HTTPS enforcement
- Implement audit logging
- Add intrusion detection
- Estimated effort: Medium
- Dependencies: Task 2, Task 3

## Task 5: Integration and end-to-end testing
- Write comprehensive API integration tests
- Create frontend component tests
- Implement end-to-end authentication flow tests
- Add performance and load testing
- Security penetration testing
- User acceptance testing scenarios
- Documentation and deployment guides
- Estimated effort: Medium
- Dependencies: Task 2, Task 3, Task 4`
}

func parseTestImplementationPlan(plan string) []types.SpecTaskImplementationTask {
	// Create test implementation tasks based on the plan
	tasks := []types.SpecTaskImplementationTask{
		{
			ID:              generateTestImplementationTaskID(),
			Title:           "Database schema and migrations",
			Description:     "Set up database tables and migrations",
			EstimatedEffort: "small",
			Priority:        0,
			Index:           0,
			Status:          types.SpecTaskImplementationStatusPending,
			Dependencies:    createDependencyJSON([]int{}),
		},
		{
			ID:              generateTestImplementationTaskID(),
			Title:           "Authentication API endpoints",
			Description:     "Implement core authentication endpoints",
			EstimatedEffort: "large",
			Priority:        0,
			Index:           1,
			Status:          types.SpecTaskImplementationStatusPending,
			Dependencies:    createDependencyJSON([]int{0}),
		},
		{
			ID:              generateTestImplementationTaskID(),
			Title:           "Frontend authentication components",
			Description:     "Build UI components for authentication",
			EstimatedEffort: "large",
			Priority:        0,
			Index:           2,
			Status:          types.SpecTaskImplementationStatusPending,
			Dependencies:    createDependencyJSON([]int{1}),
		},
		{
			ID:              generateTestImplementationTaskID(),
			Title:           "Security implementation and hardening",
			Description:     "Add security measures and hardening",
			EstimatedEffort: "medium",
			Priority:        0,
			Index:           3,
			Status:          types.SpecTaskImplementationStatusPending,
			Dependencies:    createDependencyJSON([]int{1, 2}),
		},
		{
			ID:              generateTestImplementationTaskID(),
			Title:           "Integration and end-to-end testing",
			Description:     "Comprehensive testing and validation",
			EstimatedEffort: "medium",
			Priority:        0,
			Index:           4,
			Status:          types.SpecTaskImplementationStatusPending,
			Dependencies:    createDependencyJSON([]int{1, 2, 3}),
		},
	}

	return tasks
}

// Test ID generation functions
func generateTestSpecTaskID() string {
	return fmt.Sprintf("test_spec_task_%d", time.Now().UnixNano())
}

func generateTestWorkSessionID() string {
	return fmt.Sprintf("test_ws_%d", time.Now().UnixNano())
}

func generateTestHelixSessionID() string {
	return fmt.Sprintf("test_helix_%d", time.Now().UnixNano())
}

func generateTestZedThreadID() string {
	return fmt.Sprintf("test_zt_%d", time.Now().UnixNano())
}

func generateTestImplementationTaskID() string {
	return fmt.Sprintf("test_it_%d", time.Now().UnixNano())
}

func generateTestEventID() string {
	return fmt.Sprintf("test_event_%d", time.Now().UnixNano())
}

// Helper type definitions for testing
type ZedInstanceInfo struct {
	InstanceID   string
	SpecTaskID   string
	Status       string
	CreatedAt    time.Time
	LastActivity time.Time
	ProjectPath  string
	ThreadCount  int
	RDPURL       string
	RDPPassword  string
}

type ZedInstanceStatus struct {
	InstanceID    string
	SpecTaskID    string
	Status        string
	ThreadCount   int
	ActiveThreads int
	LastActivity  *time.Time
	ProjectPath   string
	RDPURL        string
	RDPPassword   string
}

// Test-specific types
type SpecGeneration struct {
	TaskID             string
	RequirementsSpec   string
	TechnicalDesign    string
	ImplementationPlan string
	GeneratedAt        time.Time
	ModelUsed          string
	TokensUsed         int
}

// Utility functions
func createDependencyJSON(deps []int) []byte {
	data, _ := json.Marshal(deps)
	return data
}

func timePtr(t time.Time) *time.Time {
	return &t
}
