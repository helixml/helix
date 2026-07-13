import {
  TypesInteraction,
  TypesServerConfigForFrontend,
  TypesSession,
} from "../../api/api";

export interface InteractionDebugEnvironment {
  capturedAt: string;
  sourceUrl: string;
  userAgent: string;
}

export const buildInteractionDebugContext = (
  interaction: TypesInteraction,
  session: TypesSession,
  sessionSteps: unknown[],
  serverConfig: TypesServerConfigForFrontend,
  environment: InteractionDebugEnvironment,
): string => {
  const config = session.config;

  return JSON.stringify(
    {
      format: "helix-interaction-debug-context/v1",
      captured_at: environment.capturedAt,
      source_url: environment.sourceUrl,
      browser_user_agent: environment.userAgent,
      helix: {
        version: serverConfig.version,
        edition: serverConfig.edition,
        deployment_id: serverConfig.deployment_id,
      },
      session: {
        id: session.id,
        name: session.name,
        created: session.created,
        updated: session.updated,
        generation_id: session.generation_id,
        mode: session.mode,
        type: session.type,
        trigger: session.trigger,
        model_name: session.model_name,
        provider: session.provider,
        owner: session.owner,
        owner_type: session.owner_type,
        organization_id: session.organization_id,
        project_id: session.project_id,
        parent_session: session.parent_session,
        sandbox_id: session.sandbox_id,
        config: config
          ? {
              active_tools: config.active_tools,
              agent_switched_at: config.agent_switched_at,
              agent_type: config.agent_type,
              assistant_id: config.assistant_id,
              auto_restart_count: config.auto_restart_count,
              auto_restart_on_crash: config.auto_restart_on_crash,
              code_agent_runtime: config.code_agent_runtime,
              container_id: config.container_id,
              container_name: config.container_name,
              dev_container_id: config.dev_container_id,
              executor_mode: config.executor_mode,
              external_agent_config: config.external_agent_config,
              external_agent_id: config.external_agent_id,
              external_agent_status: config.external_agent_status,
              forked_at: config.forked_at,
              forked_at_interaction_id: config.forked_at_interaction_id,
              gpu_vendor: config.gpu_vendor,
              helix_version: config.helix_version,
              implementation_task_index: config.implementation_task_index,
              last_auto_restart_at: config.last_auto_restart_at,
              parent_session_id: config.parent_session_id,
              paused: config.paused,
              paused_at: config.paused_at,
              paused_reason: config.paused_reason,
              phase: config.phase,
              project_id: config.project_id,
              render_node: config.render_node,
              session_role: config.session_role,
              spec_task_id: config.spec_task_id,
              status_message: config.status_message,
              sway_version: config.sway_version,
              work_session_id: config.work_session_id,
              zed_agent_name: config.zed_agent_name,
              zed_instance_id: config.zed_instance_id,
              zed_thread_id: config.zed_thread_id,
            }
          : undefined,
      },
      interaction,
      session_steps: sessionSteps,
    },
    null,
    2,
  );
};
