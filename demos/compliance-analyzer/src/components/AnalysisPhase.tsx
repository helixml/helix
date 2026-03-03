import { useState, useEffect, useRef } from "react";
import type { HelixConfig, ComplianceEvaluation } from "../lib/helix";
import { evaluateRequirement } from "../lib/helix";
import type { ComplianceControl } from "../data/compliance-controls";
import type { AnalysisResult } from "../App";

interface AnalysisPhaseProps {
  config: HelixConfig;
  controls: ComplianceControl[];
  onComplete: (results: Map<string, AnalysisResult>) => void;
}

type ControlStatus = "pending" | "running" | "done";

interface ControlProgress {
  controlId: string;
  status: ControlStatus;
  result?: AnalysisResult;
}

const BATCH_SIZE = 2;

export function AnalysisPhase({
  config,
  controls,
  onComplete,
}: AnalysisPhaseProps) {
  const [progress, setProgress] = useState<ControlProgress[]>(() =>
    controls.map((c) => ({ controlId: c.id, status: "pending" })),
  );
  const [completedCount, setCompletedCount] = useState(0);
  const startedRef = useRef(false);

  useEffect(() => {
    if (startedRef.current) return;
    startedRef.current = true;

    async function analyzeControl(
      control: ComplianceControl,
    ): Promise<AnalysisResult> {
      setProgress((prev) =>
        prev.map((p) =>
          p.controlId === control.id ? { ...p, status: "running" } : p,
        ),
      );

      let evaluation: ComplianceEvaluation;
      try {
        const timeout = new Promise<never>((_, reject) =>
          setTimeout(() => reject(new Error("Request timed out after 120s")), 120_000),
        );
        evaluation = await Promise.race([
          evaluateRequirement(config, control),
          timeout,
        ]);
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        console.error(`Evaluation failed for ${control.id}:`, msg);
        evaluation = {
          status: "gap",
          explanation: `Evaluation failed: ${msg}`,
          missingElements: [],
        };
      }

      return {
        controlId: control.id,
        status: evaluation.status,
        evaluation,
      };
    }

    async function runAnalysis() {
      const allResults = new Map<string, AnalysisResult>();
      let completed = 0;

      for (let i = 0; i < controls.length; i += BATCH_SIZE) {
        const batch = controls.slice(i, i + BATCH_SIZE);

        setProgress((prev) =>
          prev.map((p) =>
            batch.some((c) => c.id === p.controlId)
              ? { ...p, status: "running" }
              : p,
          ),
        );

        const batchResults = await Promise.all(
          batch.map((control) => analyzeControl(control)),
        );

        for (const result of batchResults) {
          allResults.set(result.controlId, result);
        }
        completed += batchResults.length;
        setCompletedCount(completed);

        setProgress((prev) =>
          prev.map((p) => {
            const result = batchResults.find(
              (r) => r.controlId === p.controlId,
            );
            return result ? { ...p, status: "done", result } : p;
          }),
        );
      }

      await new Promise((r) => setTimeout(r, 500));
      onComplete(allResults);
    }

    runAnalysis();
  }, [config, controls, onComplete]);

  const pct = Math.round((completedCount / controls.length) * 100);

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      <div className="card">
        <h2 className="text-lg font-semibold text-white mb-2">
          Analyzing Privacy Policy
        </h2>
        <p className="text-sm text-gray-400 mb-6">
          Evaluating your privacy policy against {controls.length} GDPR
          requirements using AI...
        </p>

        {/* Progress bar */}
        <div className="mb-6">
          <div className="flex items-center justify-between text-sm mb-2">
            <span className="text-gray-400">Progress</span>
            <span className="text-white font-medium">
              {completedCount} / {controls.length} ({pct}%)
            </span>
          </div>
          <div className="progress-track">
            <div
              className="progress-fill-green"
              style={{ width: `${pct}%` }}
            />
          </div>
        </div>

        {/* Control list */}
        <div className="space-y-2 max-h-[28rem] overflow-y-auto pr-1">
          {progress.map((p) => {
            const control = controls.find((c) => c.id === p.controlId);
            if (!control) return null;

            return (
              <div
                key={p.controlId}
                className="flex items-center gap-3 px-3 py-2 rounded-lg bg-surface-400/50"
              >
                {/* Status icon */}
                <div className="w-5 h-5 flex-shrink-0 flex items-center justify-center">
                  {p.status === "pending" && (
                    <span className="w-2 h-2 rounded-full bg-gray-600" />
                  )}
                  {p.status === "running" && (
                    <svg
                      className="w-4 h-4 text-amber-400 animate-spin"
                      fill="none"
                      viewBox="0 0 24 24"
                    >
                      <circle
                        className="opacity-25"
                        cx="12"
                        cy="12"
                        r="10"
                        stroke="currentColor"
                        strokeWidth="4"
                      />
                      <path
                        className="opacity-75"
                        fill="currentColor"
                        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                      />
                    </svg>
                  )}
                  {p.status === "done" && p.result?.status === "covered" && (
                    <svg
                      className="w-4 h-4 text-emerald-400"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M5 13l4 4L19 7"
                      />
                    </svg>
                  )}
                  {p.status === "done" && p.result?.status === "partial" && (
                    <span className="w-3.5 h-3.5 rounded-full border-2 border-amber-400 bg-amber-400/20" />
                  )}
                  {p.status === "done" && p.result?.status === "gap" && (
                    <svg
                      className="w-4 h-4 text-rose-400"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M6 18L18 6M6 6l12 12"
                      />
                    </svg>
                  )}
                </div>

                {/* Control info */}
                <div className="flex-1 min-w-0">
                  <span className="text-sm text-white font-medium">
                    {control.id}
                  </span>
                  <span className="text-sm text-gray-400 ml-2 truncate">
                    {control.title}
                  </span>
                </div>

                {/* Status label */}
                {p.status === "running" && (
                  <span className="text-xs text-amber-400">Evaluating...</span>
                )}
                {p.status === "done" && p.result && (
                  <span
                    className={
                      p.result.status === "covered"
                        ? "badge-compliant"
                        : p.result.status === "partial"
                          ? "badge-partial"
                          : "badge-non-compliant"
                    }
                  >
                    {p.result.status === "covered"
                      ? "Covered"
                      : p.result.status === "partial"
                        ? "Partial"
                        : "Gap"}
                  </span>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
