package services

import (
	"context"
	"errors"
	"fmt"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

func cloneCodeAgentExecutionConfig(config *types.CodeAgentExecutionConfig) *types.CodeAgentExecutionConfig {
	if config == nil {
		return nil
	}
	cloned := *config
	cloned.GooseRecipes = append([]types.AssistantGooseRecipe(nil), config.GooseRecipes...)
	return &cloned
}

func codeAgentRuntimeForSpecTask(task *types.SpecTask) types.CodeAgentRuntime {
	if task != nil {
		if config := task.ActiveCodeAgentConfig(); config != nil && config.Runtime != "" {
			return config.Runtime
		}
	}
	return types.CodeAgentRuntimeZedAgent
}

// migrateSpecTaskCodeAgentConfig is the one-way compatibility boundary for
// legacy App-backed SpecTasks. It runs immediately before a task starts,
// materializes both project and task execution configuration, and severs the
// task/session App link. Org-agent Apps remain linked to Worker projects
// because DefaultHelixAppID is still their Bot identity binding.
func (s *SpecDrivenTaskService) migrateSpecTaskCodeAgentConfig(
	ctx context.Context,
	task *types.SpecTask,
	project *types.Project,
) error {
	if task == nil {
		return fmt.Errorf("spec task is required")
	}

	legacyProjectAppID := ""
	if project != nil {
		legacyProjectAppID = project.DefaultHelixAppID
		if legacyProjectAppID != "" {
			app, err := s.store.GetApp(ctx, legacyProjectAppID)
			if err != nil {
				if !errors.Is(err, store.ErrNotFound) {
					return fmt.Errorf("load legacy project coding agent %s: %w", legacyProjectAppID, err)
				}
				project.DefaultHelixAppID = ""
				if err := s.store.UpdateProject(ctx, project); err != nil {
					return fmt.Errorf("clear missing legacy project coding agent: %w", err)
				}
				log.Warn().Str("project_id", project.ID).Str("legacy_app_id", legacyProjectAppID).
					Msg("Cleared missing legacy project App after code-agent config migration")
				legacyProjectAppID = ""
				app = nil
			}
			if app != nil {
				projectChanged := false
				if project.CodeAgentConfig == nil {
					config, err := external_agent.MaterializeCodeAgentConfig(app, nil)
					if err != nil {
						return fmt.Errorf("migrate project coding agent %s: %w", legacyProjectAppID, err)
					}
					project.CodeAgentConfig = config
					projectChanged = true
				}
				if app.AgentKind != types.AgentKindOrg {
					project.DefaultHelixAppID = ""
					projectChanged = true
				}
				if projectChanged {
					if err := s.store.UpdateProject(ctx, project); err != nil {
						return fmt.Errorf("save migrated project code-agent config: %w", err)
					}
					log.Info().
						Str("project_id", project.ID).
						Str("legacy_app_id", legacyProjectAppID).
						Bool("org_agent_link_retained", app.AgentKind == types.AgentKindOrg).
						Msg("Migrated project coding App to code-agent config")
				}
			}
		}
	}

	config := cloneCodeAgentExecutionConfig(task.CodeAgentConfig)
	legacyTaskAppID := task.HelixAppID
	legacyOverrides := task.CodeAgentOverrides != nil
	taskNeedsMigration := config == nil || task.PlanningCodeAgentConfig == nil || legacyTaskAppID != "" || legacyOverrides
	if config == nil && legacyTaskAppID != "" {
		app, err := s.store.GetApp(ctx, legacyTaskAppID)
		if err != nil {
			return fmt.Errorf("load legacy task coding agent %s: %w", legacyTaskAppID, err)
		}
		config, err = external_agent.MaterializeCodeAgentConfig(app, task.CodeAgentOverrides)
		if err != nil {
			return fmt.Errorf("migrate task coding agent %s: %w", legacyTaskAppID, err)
		}
	} else if config == nil && project != nil && project.CodeAgentConfig != nil {
		config = external_agent.ApplyExecutionOverrides(project.CodeAgentConfig, task.CodeAgentOverrides)
	} else if config == nil && legacyProjectAppID != "" {
		app, err := s.store.GetApp(ctx, legacyProjectAppID)
		if err != nil {
			return fmt.Errorf("load legacy project coding agent %s for task: %w", legacyProjectAppID, err)
		}
		config, err = external_agent.MaterializeCodeAgentConfig(app, task.CodeAgentOverrides)
		if err != nil {
			return fmt.Errorf("migrate project coding agent %s for task: %w", legacyProjectAppID, err)
		}
	} else {
		config = external_agent.ApplyExecutionOverrides(config, task.CodeAgentOverrides)
	}
	if config == nil {
		return fmt.Errorf("no coding agent is configured; select one for this task or project and retry")
	}
	planningConfig := cloneCodeAgentExecutionConfig(task.PlanningCodeAgentConfig)
	hadExplicitPlanningConfig := planningConfig != nil
	if planningConfig == nil && project != nil {
		planningConfig = cloneCodeAgentExecutionConfig(project.PlanningCodeAgentConfig)
		hadExplicitPlanningConfig = planningConfig != nil
	}
	if planningConfig == nil {
		planningConfig = cloneCodeAgentExecutionConfig(config)
	}

	task.CodeAgentConfig = config
	task.PlanningCodeAgentConfig = planningConfig
	if !hadExplicitPlanningConfig && task.PlanningGooseRecipeName == "" {
		task.PlanningGooseRecipeName = task.GooseRecipeName
		task.PlanningGooseRecipeParams = cloneStringMap(task.GooseRecipeParams)
	}
	task.HelixAppID = ""
	task.CodeAgentOverrides = nil
	if taskNeedsMigration {
		if err := s.store.UpdateSpecTask(ctx, task); err != nil {
			return fmt.Errorf("save migrated task code-agent config: %w", err)
		}
	}

	if task.PlanningSessionID != "" {
		session, err := s.store.GetSession(ctx, task.PlanningSessionID)
		if err != nil {
			return fmt.Errorf("load task session while clearing legacy App link: %w", err)
		}
		activeConfig := task.ActiveCodeAgentConfig()
		if session.ParentApp != "" || session.Metadata.CodeAgentOverrides != nil ||
			session.Metadata.CodeAgentRuntime != activeConfig.Runtime {
			session.ParentApp = ""
			session.Metadata.CodeAgentOverrides = nil
			session.Metadata.CodeAgentRuntime = activeConfig.Runtime
			if _, err := s.store.UpdateSession(ctx, *session); err != nil {
				return fmt.Errorf("clear task session legacy App link: %w", err)
			}
		}
	}

	if legacyTaskAppID != "" || legacyOverrides {
		log.Info().
			Str("task_id", task.ID).
			Str("legacy_app_id", legacyTaskAppID).
			Msg("Migrated SpecTask coding App to task-owned code-agent config")
	}
	return nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
