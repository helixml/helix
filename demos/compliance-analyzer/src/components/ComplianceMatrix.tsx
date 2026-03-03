import { useMemo } from "react";
import type { ComplianceControl } from "../data/compliance-controls";
import type { AnalysisResult } from "../App";

interface ComplianceMatrixProps {
  controls: ComplianceControl[];
  results: Map<string, AnalysisResult>;
  onSelectControl: (controlId: string) => void;
}

export function ComplianceMatrix({
  controls,
  results,
  onSelectControl,
}: ComplianceMatrixProps) {
  const summary = useMemo(() => {
    let covered = 0;
    let partial = 0;
    let gap = 0;
    for (const result of results.values()) {
      if (result.status === "covered") covered++;
      else if (result.status === "partial") partial++;
      else gap++;
    }
    const total = covered + partial + gap;
    return {
      covered,
      partial,
      gap,
      total,
      coveredPct: total ? Math.round((covered / total) * 100) : 0,
      partialPct: total ? Math.round((partial / total) * 100) : 0,
      gapPct: total ? Math.round((gap / total) * 100) : 0,
    };
  }, [results]);

  const grouped = useMemo(() => {
    const groups = new Map<string, ComplianceControl[]>();
    for (const control of controls) {
      const category = control.category;
      const existing = groups.get(category) ?? [];
      existing.push(control);
      groups.set(category, existing);
    }
    return groups;
  }, [controls]);

  return (
    <div className="space-y-6">
      {/* Summary bar */}
      <div className="card">
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold text-white">
              GDPR Compliance Results
            </h2>
            <p className="text-sm text-gray-400 mt-1">
              {summary.total} GDPR requirements checked against your privacy policy
            </p>
          </div>
          <div className="flex items-center gap-3">
            <span className="badge-compliant">
              {summary.coveredPct}% Covered ({summary.covered})
            </span>
            <span className="badge-partial">
              {summary.partialPct}% Partial ({summary.partial})
            </span>
            <span className="badge-non-compliant">
              {summary.gapPct}% Gaps ({summary.gap})
            </span>
          </div>
        </div>

        {/* Stacked progress bar */}
        <div className="mt-4 w-full h-3 bg-surface-400 rounded-full overflow-hidden flex">
          {summary.coveredPct > 0 && (
            <div
              className="h-full bg-compliance-green transition-all duration-500"
              style={{ width: `${summary.coveredPct}%` }}
            />
          )}
          {summary.partialPct > 0 && (
            <div
              className="h-full bg-compliance-amber transition-all duration-500"
              style={{ width: `${summary.partialPct}%` }}
            />
          )}
          {summary.gapPct > 0 && (
            <div
              className="h-full bg-compliance-red transition-all duration-500"
              style={{ width: `${summary.gapPct}%` }}
            />
          )}
        </div>
      </div>

      {/* Grid grouped by category */}
      {Array.from(grouped.entries()).map(([category, categoryControls]) => (
        <div key={category}>
          <h3 className="text-sm font-medium text-gray-400 uppercase tracking-wider mb-3">
            {category}
          </h3>
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-3">
            {categoryControls.map((control) => {
              const result = results.get(control.id);
              const status = result?.status ?? "gap";

              const borderColor =
                status === "covered"
                  ? "border-compliance-green/40 hover:border-compliance-green/70"
                  : status === "partial"
                    ? "border-compliance-amber/40 hover:border-compliance-amber/70"
                    : "border-compliance-red/40 hover:border-compliance-red/70";

              const bgColor =
                status === "covered"
                  ? "bg-compliance-green/5"
                  : status === "partial"
                    ? "bg-compliance-amber/5"
                    : "bg-compliance-red/5";

              const glowClass =
                status === "covered"
                  ? "hover:glow-green"
                  : status === "partial"
                    ? "hover:glow-amber"
                    : "hover:glow-red";

              const dotColor =
                status === "covered"
                  ? "bg-compliance-green"
                  : status === "partial"
                    ? "bg-compliance-amber"
                    : "bg-compliance-red";

              return (
                <button
                  key={control.id}
                  onClick={() => onSelectControl(control.id)}
                  className={`${bgColor} ${borderColor} ${glowClass} border rounded-xl p-3 text-left transition-all duration-200 cursor-pointer`}
                >
                  <div className="flex items-center gap-2 mb-1">
                    <span
                      className={`w-2 h-2 rounded-full ${dotColor} flex-shrink-0`}
                    />
                    <span className="text-xs font-mono text-gray-400">
                      {control.id}
                    </span>
                  </div>
                  <p className="text-sm text-white leading-tight line-clamp-2">
                    {control.title}
                  </p>
                </button>
              );
            })}
          </div>
        </div>
      ))}
    </div>
  );
}
