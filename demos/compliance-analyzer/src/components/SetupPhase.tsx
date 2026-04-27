import { useState, useCallback } from "react";
import type { HelixConfig } from "../lib/helix";
import { testConnection } from "../lib/helix";

interface SetupPhaseProps {
  onComplete: (config: HelixConfig) => void;
}

export function SetupPhase({ onComplete }: SetupPhaseProps) {
  const [baseUrl, setBaseUrl] = useState(
    () => localStorage.getItem("helix_base_url") ?? "",
  );
  const [apiKey, setApiKey] = useState(
    () => localStorage.getItem("helix_api_key") ?? "",
  );
  const hasSavedCredentials = !!localStorage.getItem("helix_api_key");
  const [connectionStatus, setConnectionStatus] = useState<
    "idle" | "testing" | "connected" | "error"
  >("idle");
  const [connectionError, setConnectionError] = useState("");

  const handleTestConnection = useCallback(async () => {
    setConnectionStatus("testing");
    setConnectionError("");
    try {
      const config: HelixConfig = {
        baseUrl: baseUrl.replace(/\/+$/, ""),
        apiKey,
      };
      await testConnection(config);
      setConnectionStatus("connected");
    } catch (err) {
      setConnectionStatus("error");
      setConnectionError(
        err instanceof Error ? err.message : "Connection failed",
      );
    }
  }, [baseUrl, apiKey]);

  const handleNext = useCallback(() => {
    localStorage.setItem("helix_base_url", baseUrl.replace(/\/+$/, ""));
    localStorage.setItem("helix_api_key", apiKey);
    onComplete({
      baseUrl: baseUrl.replace(/\/+$/, ""),
      apiKey,
    });
  }, [onComplete, baseUrl, apiKey]);

  const handleClearCredentials = useCallback(() => {
    localStorage.removeItem("helix_base_url");
    localStorage.removeItem("helix_api_key");
    setBaseUrl("");
    setApiKey("");
    setConnectionStatus("idle");
  }, []);

  const isConnected = connectionStatus === "connected";

  return (
    <div className="max-w-2xl mx-auto space-y-8">
      <div className="card">
        <h2 className="text-lg font-semibold text-white mb-2">
          Connect to Helix
        </h2>
        <p className="text-sm text-gray-400 mb-6">
          Enter your Helix instance details. A new app will be created
          automatically for each analysis to keep documents isolated.
        </p>

        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-400 mb-1">
              Helix URL
            </label>
            <input
              type="text"
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
              className="w-full px-3 py-2 bg-surface-400 border border-surface-100/20 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-emerald-500/50"
              placeholder="https://app.tryhelix.ai (empty = localhost proxy)"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-400 mb-1">
              API Key
            </label>
            <input
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              className="w-full px-3 py-2 bg-surface-400 border border-surface-100/20 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-emerald-500/50"
              placeholder="hl-..."
            />
          </div>

          <div className="flex items-center gap-3 pt-2">
            <button
              onClick={handleTestConnection}
              disabled={!apiKey}
              className="px-4 py-2 bg-surface-100 hover:bg-gray-600 text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            >
              {connectionStatus === "testing"
                ? "Testing..."
                : "Test Connection"}
            </button>
            {connectionStatus === "connected" && (
              <span className="text-sm text-emerald-400 flex items-center gap-1.5">
                <span className="w-2 h-2 rounded-full bg-emerald-400 inline-block" />
                Connected
              </span>
            )}
            {connectionStatus === "error" && (
              <span className="text-sm text-rose-400">{connectionError}</span>
            )}
          </div>

          {isConnected && (
            <button
              onClick={handleNext}
              className="w-full px-4 py-3 bg-emerald-600 hover:bg-emerald-500 text-white font-medium rounded-lg transition-colors mt-4"
            >
              Next: Upload Documents
            </button>
          )}

          {hasSavedCredentials && (
            <button
              onClick={handleClearCredentials}
              className="text-xs text-gray-500 hover:text-gray-300 transition-colors mt-2"
            >
              Clear saved credentials
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
