import { TypesInteraction } from "../../api/api";

/** Prefer the server's measured duration, with a timestamp fallback for older rows. */
export const getInteractionDurationMs = (
  interaction?: Pick<TypesInteraction, "duration_ms" | "created" | "completed"> | null,
) => {
  if (interaction?.duration_ms && interaction.duration_ms > 0) {
    return interaction.duration_ms;
  }

  if (!interaction?.created || !interaction.completed) return 0;

  const createdAt = Date.parse(interaction.created);
  const completedAt = Date.parse(interaction.completed);
  if (!Number.isFinite(createdAt) || !Number.isFinite(completedAt)) return 0;

  return Math.max(0, completedAt - createdAt);
};
