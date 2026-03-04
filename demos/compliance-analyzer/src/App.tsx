import { useState, useCallback } from "react";
import { SetupPhase } from "./components/SetupPhase";
import { UploadPhase } from "./components/UploadPhase";
import { AnalysisPhase } from "./components/AnalysisPhase";
import { ComplianceMatrix } from "./components/ComplianceMatrix";
import { DetailPanel } from "./components/DetailPanel";
import { complianceControls, getControlById } from "./data/compliance-controls";
import type { HelixConfig, ComplianceEvaluation } from "./lib/helix";

type Phase = "setup" | "upload" | "analyzing" | "results";

export interface AnalysisResult {
  controlId: string;
  status: "covered" | "partial" | "gap" | "error";
  evaluation?: ComplianceEvaluation;
}

function loadSavedConfig(): { config: HelixConfig; valid: boolean } {
  const baseUrl = localStorage.getItem("helix_base_url") ?? "";
  const apiKey = localStorage.getItem("helix_api_key") ?? "";
  return {
    config: { baseUrl, apiKey },
    valid: !!apiKey,
  };
}

export default function App() {
  const [phase, setPhase] = useState<Phase>(() => {
    const { valid } = loadSavedConfig();
    return valid ? "upload" : "setup";
  });
  const [helixConfig, setHelixConfig] = useState<HelixConfig>(() => {
    const { config, valid } = loadSavedConfig();
    return valid ? config : { baseUrl: "", apiKey: "" };
  });
  const [analysisResults, setAnalysisResults] = useState<
    Map<string, AnalysisResult>
  >(new Map());
  const [documentName, setDocumentName] = useState("");
  const [selectedControlId, setSelectedControlId] = useState<string | null>(
    null,
  );

  const handleSetupComplete = useCallback((config: HelixConfig) => {
    setHelixConfig(config);
    setPhase("upload");
  }, []);

  const handleUploadComplete = useCallback(
    (appId: string, fileName: string) => {
      setHelixConfig((prev) => ({ ...prev, appId }));
      setDocumentName(fileName);
      setPhase("analyzing");
    },
    [],
  );

  const handleAnalysisComplete = useCallback(
    (results: Map<string, AnalysisResult>) => {
      setAnalysisResults(results);
      setPhase("results");
    },
    [],
  );

  const handleStartOver = useCallback(() => {
    setAnalysisResults(new Map());
    setSelectedControlId(null);
    setDocumentName("");
    setPhase("upload");
  }, []);

  const handleShowSettings = useCallback(() => {
    setPhase("setup");
  }, []);

  const handleSelectControl = useCallback((controlId: string) => {
    setSelectedControlId(controlId);
  }, []);

  const handleCloseDetail = useCallback(() => {
    setSelectedControlId(null);
  }, []);

  const selectedControl = selectedControlId
    ? getControlById(selectedControlId)
    : undefined;
  const selectedResult = selectedControlId
    ? analysisResults.get(selectedControlId)
    : undefined;

  return (
    <div className="min-h-screen bg-surface-500 text-gray-100">
      {/* Header */}
      <header className="border-b border-surface-100/20 bg-surface-600">
        <div className="mx-auto max-w-7xl px-6 py-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-emerald-500 to-cyan-500 flex items-center justify-center text-white font-bold text-sm">
              H
            </div>
            <h1 className="text-xl font-semibold text-white">
              GDPR Privacy Policy Analyzer
            </h1>
          </div>
          <div className="flex items-center gap-4 text-sm text-gray-400">
            {phase !== "setup" && (
              <button
                onClick={handleShowSettings}
                className="text-gray-500 hover:text-gray-300 transition-colors mr-2"
                title="Change settings"
              >
                Settings
              </button>
            )}
            <span
              className={
                phase === "setup"
                  ? "text-white font-medium"
                  : "text-gray-500"
              }
            >
              1. Setup
            </span>
            <span className="text-gray-600">/</span>
            <span
              className={
                phase === "upload"
                  ? "text-white font-medium"
                  : "text-gray-500"
              }
            >
              2. Upload
            </span>
            <span className="text-gray-600">/</span>
            <span
              className={
                phase === "analyzing"
                  ? "text-white font-medium"
                  : "text-gray-500"
              }
            >
              3. Analysis
            </span>
            <span className="text-gray-600">/</span>
            <span
              className={
                phase === "results"
                  ? "text-white font-medium"
                  : "text-gray-500"
              }
            >
              4. Results
            </span>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="mx-auto max-w-7xl px-6 py-8">
        {phase === "setup" && (
          <SetupPhase onComplete={handleSetupComplete} />
        )}
        {phase === "upload" && (
          <UploadPhase
            config={helixConfig}
            onComplete={handleUploadComplete}
          />
        )}
        {phase === "analyzing" && (
          <AnalysisPhase
            config={helixConfig}
            controls={complianceControls}
            onComplete={handleAnalysisComplete}
          />
        )}
        {phase === "results" && (
          <ComplianceMatrix
            controls={complianceControls}
            results={analysisResults}
            documentName={documentName}
            onSelectControl={handleSelectControl}
            onStartOver={handleStartOver}
          />
        )}
      </main>

      {/* Detail Panel */}
      {selectedControl && selectedResult && (
        <DetailPanel
          control={selectedControl}
          result={selectedResult}
          onClose={handleCloseDetail}
        />
      )}
    </div>
  );
}
