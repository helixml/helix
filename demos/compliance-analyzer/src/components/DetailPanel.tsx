import type { ComplianceControl } from "../data/compliance-controls";
import type { AnalysisResult } from "../App";

interface DetailPanelProps {
  control: ComplianceControl;
  result: AnalysisResult;
  onClose: () => void;
}

export function DetailPanel({ control, result, onClose }: DetailPanelProps) {
  const statusLabel =
    result.status === "covered"
      ? "Covered"
      : result.status === "partial"
        ? "Partial"
        : "Gap";

  const badgeClass =
    result.status === "covered"
      ? "badge-compliant"
      : result.status === "partial"
        ? "badge-partial"
        : "badge-non-compliant";

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/50 z-40"
        onClick={onClose}
      />

      {/* Panel */}
      <div className="fixed top-0 right-0 h-full w-full max-w-lg bg-surface-400 border-l border-surface-100/20 z-50 overflow-y-auto shadow-2xl animate-slide-in">
        {/* Header */}
        <div className="sticky top-0 bg-surface-400 border-b border-surface-100/20 px-6 py-4 flex items-start justify-between">
          <div className="flex-1 min-w-0 mr-4">
            <div className="flex items-center gap-2 mb-1">
              <span className="text-xs font-mono text-gray-400">
                {control.id}
              </span>
              <span className={badgeClass}>{statusLabel}</span>
            </div>
            <h2 className="text-lg font-semibold text-white">
              {control.title}
            </h2>
          </div>
          <button
            onClick={onClose}
            className="p-1 text-gray-400 hover:text-white transition-colors flex-shrink-0"
          >
            <svg
              className="w-5 h-5"
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
          </button>
        </div>

        {/* Body */}
        <div className="px-6 py-5 space-y-6">
          {/* GDPR requirement */}
          <div>
            <h3 className="text-sm font-medium text-gray-400 mb-2">
              GDPR Requirement
            </h3>
            <p className="text-sm text-gray-300 leading-relaxed">
              {control.description}
            </p>
          </div>

          {/* AI Analysis */}
          {result.evaluation && (
            <div>
              <h3 className="text-sm font-medium text-gray-400 mb-2">
                AI Analysis
              </h3>
              <div
                className={`rounded-lg p-4 border ${
                  result.status === "covered"
                    ? "bg-emerald-500/10 border-emerald-500/20"
                    : result.status === "partial"
                      ? "bg-amber-500/10 border-amber-500/20"
                      : "bg-rose-500/10 border-rose-500/20"
                }`}
              >
                <p className="text-sm text-gray-200 leading-relaxed">
                  {result.evaluation.explanation}
                </p>
              </div>
            </div>
          )}

          {/* Missing elements */}
          {result.evaluation &&
            result.evaluation.missingElements.length > 0 && (
              <div>
                <h3 className="text-sm font-medium text-gray-400 mb-2">
                  Recommendations
                </h3>
                <ul className="space-y-2">
                  {result.evaluation.missingElements.map((item, idx) => (
                    <li
                      key={idx}
                      className="flex items-start gap-2 text-sm text-gray-300"
                    >
                      <svg
                        className="w-4 h-4 text-amber-400 flex-shrink-0 mt-0.5"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M9 5l7 7-7 7"
                        />
                      </svg>
                      {item}
                    </li>
                  ))}
                </ul>
              </div>
            )}

          {/* What compliance looks like */}
          <div>
            <h3 className="text-sm font-medium text-gray-400 mb-2">
              What Compliance Looks Like
            </h3>
            <p className="text-sm text-gray-300 leading-relaxed">
              {control.coveredLooksLike}
            </p>
          </div>
        </div>
      </div>
    </>
  );
}
