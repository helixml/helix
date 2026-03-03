import { useState, useEffect, useRef, useCallback } from "react";
import type { HelixConfig, Knowledge, KnowledgeState } from "../lib/helix";
import {
  listKnowledge,
  getKnowledge,
  uploadFiles,
  completeKnowledge,
  refreshKnowledge,
} from "../lib/helix";

interface UploadPhaseProps {
  config: HelixConfig;
  onComplete: () => void;
}

type UploadState =
  | "loading" // fetching knowledge sources
  | "ready" // ready for file selection
  | "uploading" // files being uploaded
  | "indexing" // waiting for indexing to complete
  | "indexed" // indexing done, ready to analyze
  | "error";

export function UploadPhase({ config, onComplete }: UploadPhaseProps) {
  const [state, setState] = useState<UploadState>("loading");
  const [error, setError] = useState("");
  const [knowledge, setKnowledge] = useState<Knowledge | null>(null);
  const [filestorePath, setFilestorePath] = useState("");
  const [selectedFiles, setSelectedFiles] = useState<File[]>([]);
  const [indexProgress, setIndexProgress] = useState(0);
  const [indexStep, setIndexStep] = useState("");
  const [knowledgeState, setKnowledgeState] = useState<KnowledgeState | "">("");
  const fileInputRef = useRef<HTMLInputElement>(null);
  const pollRef = useRef<ReturnType<typeof setInterval>>();

  // Load knowledge sources on mount
  useEffect(() => {
    async function loadKnowledge() {
      try {
        const sources = await listKnowledge(config);
        const filestoreSource = sources.find((k) => k.source.filestore);
        if (!filestoreSource) {
          setError(
            "No filestore knowledge source found on this app. Create one in the Helix dashboard first.",
          );
          setState("error");
          return;
        }
        setKnowledge(filestoreSource);
        setFilestorePath(
          `apps/${config.appId}/${filestoreSource.source.filestore!.path}`,
        );

        // If knowledge is already ready, allow skipping upload
        if (filestoreSource.state === "ready") {
          setKnowledgeState("ready");
        }
        setState("ready");
      } catch (err) {
        setError(
          err instanceof Error ? err.message : "Failed to load knowledge sources",
        );
        setState("error");
      }
    }
    loadKnowledge();
  }, [config]);

  // Clean up polling on unmount
  useEffect(() => {
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, []);

  const handleFileSelect = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      if (e.target.files) {
        setSelectedFiles(Array.from(e.target.files));
      }
    },
    [],
  );

  const handleRemoveFile = useCallback((index: number) => {
    setSelectedFiles((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const handleUpload = useCallback(async () => {
    if (!knowledge || selectedFiles.length === 0) return;

    setState("uploading");
    setError("");

    try {
      await uploadFiles(config, filestorePath, selectedFiles);

      // Trigger indexing — use /complete for first-time upload, /refresh for re-index
      setState("indexing");
      setIndexProgress(0);
      setIndexStep("Queued for indexing...");

      if (knowledge.state === "preparing") {
        await completeKnowledge(config, knowledge.id);
      } else {
        await refreshKnowledge(config, knowledge.id);
      }

      // Poll for indexing completion
      pollRef.current = setInterval(async () => {
        try {
          const k = await getKnowledge(config, knowledge.id);
          setKnowledgeState(k.state);

          if (k.progress) {
            setIndexProgress(k.progress.progress);
            setIndexStep(k.progress.step || k.progress.message || "Indexing...");
          }

          if (k.state === "ready") {
            clearInterval(pollRef.current);
            setIndexProgress(100);
            setIndexStep("Indexing complete");
            setSelectedFiles([]);
            setState("indexed");
          } else if (k.state === "error") {
            clearInterval(pollRef.current);
            setError(k.message || "Indexing failed");
            setState("error");
          }
        } catch (err) {
          clearInterval(pollRef.current);
          setError(
            err instanceof Error ? err.message : "Failed to check indexing status",
          );
          setState("error");
        }
      }, 2000);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Upload failed");
      setState("error");
    }
  }, [config, knowledge, filestorePath, selectedFiles]);

  const formatFileSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      <div className="card">
        <h2 className="text-lg font-semibold text-white mb-2">
          Upload Privacy Policy
        </h2>
        <p className="text-sm text-gray-400 mb-6">
          Upload the privacy policy document(s) you want to analyze for GDPR
          compliance. Supported formats: PDF, Markdown, plain text.
        </p>

        {state === "loading" && (
          <div className="flex items-center gap-3 text-gray-400">
            <svg
              className="w-5 h-5 animate-spin"
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
            <span className="text-sm">Loading knowledge sources...</span>
          </div>
        )}

        {state === "error" && (
          <div className="rounded-lg bg-rose-500/10 border border-rose-500/20 p-4">
            <p className="text-sm text-rose-400">{error}</p>
            <button
              onClick={() => {
                setState("ready");
                setError("");
              }}
              className="mt-3 text-sm text-rose-300 underline hover:text-rose-200"
            >
              Try again
            </button>
          </div>
        )}

        {state === "ready" && (
          <>
            {/* File selection area */}
            <div
              onClick={() => fileInputRef.current?.click()}
              className="border-2 border-dashed border-surface-100/30 rounded-lg p-8 text-center cursor-pointer hover:border-emerald-500/40 hover:bg-surface-400/50 transition-colors"
            >
              <svg
                className="w-10 h-10 mx-auto text-gray-500 mb-3"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={1.5}
                  d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"
                />
              </svg>
              <p className="text-sm text-gray-400 mb-1">
                Click to select files
              </p>
              <p className="text-xs text-gray-500">
                PDF, Markdown, or plain text
              </p>
              <input
                ref={fileInputRef}
                type="file"
                multiple
                accept=".pdf,.md,.txt,.docx"
                onChange={handleFileSelect}
                className="hidden"
              />
            </div>

            {/* Selected files */}
            {selectedFiles.length > 0 && (
              <div className="mt-4 space-y-2">
                <h3 className="text-sm font-medium text-gray-400">
                  Selected files
                </h3>
                {selectedFiles.map((file, idx) => (
                  <div
                    key={`${file.name}-${idx}`}
                    className="flex items-center justify-between px-3 py-2 rounded-lg bg-surface-400/50"
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      <svg
                        className="w-4 h-4 text-gray-500 flex-shrink-0"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
                        />
                      </svg>
                      <span className="text-sm text-white truncate">
                        {file.name}
                      </span>
                      <span className="text-xs text-gray-500 flex-shrink-0">
                        {formatFileSize(file.size)}
                      </span>
                    </div>
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        handleRemoveFile(idx);
                      }}
                      className="p-1 text-gray-500 hover:text-rose-400 transition-colors"
                    >
                      <svg
                        className="w-4 h-4"
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
                ))}

                <button
                  onClick={handleUpload}
                  className="w-full px-4 py-3 bg-emerald-600 hover:bg-emerald-500 text-white font-medium rounded-lg transition-colors mt-2"
                >
                  Upload & Index ({selectedFiles.length}{" "}
                  {selectedFiles.length === 1 ? "file" : "files"})
                </button>
              </div>
            )}

            {/* Skip to analysis if already indexed */}
            {knowledgeState === "ready" && (
              <div className="mt-4 pt-4 border-t border-surface-100/20">
                <p className="text-sm text-gray-400 mb-3">
                  Knowledge is already indexed. You can upload new documents or
                  proceed with the existing ones.
                </p>
                <button
                  onClick={onComplete}
                  className="w-full px-4 py-3 bg-surface-100 hover:bg-gray-600 text-white font-medium rounded-lg transition-colors"
                >
                  Skip — Use Existing Documents
                </button>
              </div>
            )}
          </>
        )}

        {state === "uploading" && (
          <div className="space-y-3">
            <div className="flex items-center gap-3 text-gray-400">
              <svg
                className="w-5 h-5 animate-spin"
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
              <span className="text-sm text-white">Uploading files...</span>
            </div>
          </div>
        )}

        {state === "indexing" && (
          <div className="space-y-4">
            <div className="flex items-center gap-3">
              <svg
                className="w-5 h-5 animate-spin text-emerald-400"
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
              <span className="text-sm text-white">
                Indexing documents for RAG...
              </span>
            </div>

            <div>
              <div className="flex items-center justify-between text-sm mb-2">
                <span className="text-gray-400 capitalize">{indexStep}</span>
                <span className="text-white font-medium">{indexProgress}%</span>
              </div>
              <div className="progress-track">
                <div
                  className="progress-fill-green"
                  style={{ width: `${indexProgress}%` }}
                />
              </div>
            </div>
          </div>
        )}

        {state === "indexed" && (
          <div className="space-y-4">
            <div className="flex items-center gap-2 text-emerald-400">
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
                  d="M5 13l4 4L19 7"
                />
              </svg>
              <span className="text-sm font-medium">
                Documents indexed and ready
              </span>
            </div>
            <button
              onClick={onComplete}
              className="w-full px-4 py-3 bg-emerald-600 hover:bg-emerald-500 text-white font-medium rounded-lg transition-colors"
            >
              Run GDPR Analysis
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
