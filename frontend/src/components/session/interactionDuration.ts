import { TypesInteraction } from "../../api/api";

/** Return the server-recorded request time, or a stable caller-provided fallback. */
export const getInteractionRequestTimeMs = (
  interaction: Pick<TypesInteraction, "created">,
  fallbackMs: number,
) => {
  const requestTimeMs = Date.parse(interaction.created || "");
  return Number.isFinite(requestTimeMs) ? requestTimeMs : fallbackMs;
};

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
