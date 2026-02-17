import { useState, useEffect, useCallback } from "react";
import "./style.css";
import { main } from "../wailsjs/go/models";
import {
  GetVMStatus,
  GetVMConfig,
  IsVMImageReady,
  GetHelixURL,
  GetAutoLoginURL,
  GetSystemInfo,
  GetZFSStats,
  GetSettings,
  GetScanoutStats,
  GetLicenseStatus,
  GetDesktopQuota,
  GetAppVersion,
  GetUpdateInfo,
  CheckForUpdate,
  StartVM,
} from "../wailsjs/go/main/App";
import {
  EventsOn,
  EventsOff,
  WindowToggleMaximise,
  WindowIsFullscreen,
  BrowserOpenURL,
  ClipboardSetText,
  ClipboardGetText,
} from "../wailsjs/runtime/runtime";
import { HomeView } from "./views/HomeView";
import { SettingsPanel } from "./components/SettingsPanel";
import { ConsoleDrawer } from "./components/ConsoleDrawer";
import { getTrialRemaining } from "./lib/helpers";

const defaultVMStatus = new main.VMStatus({
  state: "stopped",
  cpu_percent: 0,
  memory_used: 0,
  uptime: 0,
  sessions: 0,
  api_ready: false,
});

const defaultVMConfig = new main.VMConfig({
  name: "",
  cpus: 4,
  memory_mb: 8192,
  disk_path: "",
  vsock_cid: 0,
  ssh_port: 2222,
  api_port: 8080,
  qmp_port: 0,
});

const defaultZFSStats = new main.ZFSStats({
  pool_name: "",
  pool_size: 0,
  pool_used: 0,
  pool_available: 0,
  dedup_ratio: 1,
  compression_ratio: 1,
  dedup_saved_bytes: 0,
  datasets: [],
  last_updated: "",
});

const defaultSettings = new main.AppSettings({
  vm_cpus: 4,
  vm_memory_mb: 8192,
  data_disk_size_gb: 256,
  ssh_port: 41222,
  api_port: 41080,
  auto_start_vm: false,
  vm_disk_path: "",
});

const defaultLicenseStatus = new main.LicenseStatus({
  state: "no_trial",
});

export function App() {
  const [vmStatus, setVmStatus] = useState<main.VMStatus>(defaultVMStatus);
  const [vmConfig, setVmConfig] = useState<main.VMConfig>(defaultVMConfig);
  const [vmImageReady, setVmImageReady] = useState(false);
  const [needsDownload, setNeedsDownload] = useState(false);
  const [helixURL, setHelixURL] = useState("");
  const [autoLoginURL, setAutoLoginURL] = useState("");
  const [zfsStats, setZfsStats] = useState<main.ZFSStats>(defaultZFSStats);
  const [settings, setSettings] = useState<main.AppSettings>(defaultSettings);
  const [downloadProgress, setDownloadProgress] =
    useState<main.DownloadProgress | null>(null);
  const [licenseStatus, setLicenseStatus] =
    useState<main.LicenseStatus>(defaultLicenseStatus);
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const [desktopQuota, setDesktopQuota] = useState({ active: 0, max: 0 });
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [consoleOpen, setConsoleOpen] = useState(false);
  const [initialized, setInitialized] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [appVersion, setAppVersion] = useState("");
  const [updateInfo, setUpdateInfo] = useState<main.UpdateInfo | null>(null);
  const [vmUpdateProgress, setVmUpdateProgress] = useState<{
    phase: string;
    bytes_done: number;
    bytes_total: number;
    percent: number;
    speed?: string;
    eta?: string;
    error?: string;
  } | null>(null);
  const [vmUpdateReady, setVmUpdateReady] = useState(false);

  const showToast = useCallback((msg: string) => {
    setToastMessage(msg);
  }, []);

  // Auto-clear toast after 3s
  useEffect(() => {
    if (!toastMessage) return;
    const timer = setTimeout(() => setToastMessage(null), 3000);
    return () => clearTimeout(timer);
  }, [toastMessage]);

  // Load initial state
  useEffect(() => {
    (async () => {
      try {
        const [
          imageReady,
          _sysInfo,
          status,
          config,
          url,
          loginURL,
          appSettings,
          license,
          version,
          existingUpdateInfo,
        ] = await Promise.all([
          IsVMImageReady(),
          GetSystemInfo(),
          GetVMStatus(),
          GetVMConfig(),
          GetHelixURL(),
          GetAutoLoginURL(),
          GetSettings(),
          GetLicenseStatus(),
          GetAppVersion(),
          GetUpdateInfo(),
        ]);
        setVmImageReady(imageReady);
        setNeedsDownload(!imageReady);
        setVmStatus(status);
        setVmConfig(config);
        setHelixURL(url);
        setAutoLoginURL(loginURL);
        setSettings(appSettings);
        setLicenseStatus(license);
        setAppVersion(version);
        if (existingUpdateInfo?.available) {
          setUpdateInfo(existingUpdateInfo);
        }
      } catch (err) {
        console.error("Failed to load initial state:", err);
      } finally {
        setInitialized(true);
      }
    })();
  }, []);

  // Subscribe to events
  useEffect(() => {
    EventsOn("vm:status", (status: main.VMStatus) => {
      setVmStatus(status);
    });

    EventsOn("download:progress", (progress: main.DownloadProgress) => {
      if (progress.status === "complete") {
        setNeedsDownload(false);
        setVmImageReady(true);
        setDownloadProgress(null);
        // Auto-start VM after download completes
        StartVM().catch((err: unknown) => console.error("Auto-start after download failed:", err));
      } else if (progress.status === "error") {
        setDownloadProgress(progress);
      } else {
        setDownloadProgress(progress);
      }
    });

    // Listen for settings:show event from system tray
    EventsOn("settings:show", () => {
      setSettingsOpen(true);
    });

    // Update events
    EventsOn("update:available", (info: main.UpdateInfo) => {
      if (info?.available) {
        setUpdateInfo(info);
      }
    });

    EventsOn("update:vm-progress", (progress: any) => {
      if (progress?.error) {
        setVmUpdateProgress(null);
      } else if (progress?.phase === "ready") {
        setVmUpdateProgress(null);
        setVmUpdateReady(true);
      } else {
        setVmUpdateProgress(progress);
      }
    });

    EventsOn("update:vm-ready", () => {
      setVmUpdateReady(true);
    });

    // Auth proxy ready — fetch and set the auto-login URL immediately
    EventsOn("auth:ready", async () => {
      const loginURL = await GetAutoLoginURL();
      if (loginURL) {
        setAutoLoginURL(loginURL);
      }
    });

    // Listen for messages from the Helix iframe
    const handleMessage = (event: MessageEvent) => {
      if (event.data?.type === 'open-external-url' && typeof event.data.url === 'string') {
        BrowserOpenURL(event.data.url);
      }
      // Clipboard bridge: WKWebView blocks navigator.clipboard in iframes,
      // so the iframe sends postMessage and we use the Wails runtime clipboard
      // which accesses NSPasteboard directly.
      if (event.data?.type === 'helix-clipboard-write' && typeof event.data.text === 'string') {
        ClipboardSetText(event.data.text);
      }
      if (event.data?.type === 'helix-clipboard-read' && event.data.id) {
        ClipboardGetText().then(text => {
          const iframe = document.querySelector('.home-view iframe') as HTMLIFrameElement;
          if (iframe?.contentWindow) {
            iframe.contentWindow.postMessage(
              { type: 'helix-clipboard-response', id: event.data.id, text },
              '*'
            );
          }
        });
      }
    };
    window.addEventListener('message', handleMessage);

    return () => {
      EventsOff("vm:status");
      EventsOff("download:progress");
      EventsOff("settings:show");
      EventsOff("auth:ready");
      EventsOff("update:available");
      EventsOff("update:vm-progress");
      EventsOff("update:vm-ready");
      window.removeEventListener('message', handleMessage);
    };
  }, []);

  // Detect macOS fullscreen mode (traffic lights disappear, so collapse padding)
  useEffect(() => {
    let prev = false;
    const interval = setInterval(async () => {
      try {
        const fs = await WindowIsFullscreen();
        if (fs !== prev) {
          prev = fs;
          setIsFullscreen(fs);
        }
      } catch {
        // ignore
      }
    }, 200);
    return () => clearInterval(interval);
  }, []);

  // Poll for stats, and re-fetch autoLoginURL when API becomes ready
  useEffect(() => {
    let prevAPIReady = false;

    const interval = setInterval(async () => {
      try {
        const status = await GetVMStatus();
        setVmStatus(status);

        // Poll license status so UI reacts to trial expiry mid-session
        const license = await GetLicenseStatus();
        setLicenseStatus(license);

        if (status.state === "running") {
          const [zfs, quota] = await Promise.all([
            GetZFSStats(),
            GetDesktopQuota(),
          ]);
          setZfsStats(zfs);
          setDesktopQuota(quota);
        }

        // Re-fetch autoLoginURL when API transitions to ready
        if (status.api_ready && !prevAPIReady) {
          console.log("[AUTH] API became ready, fetching autoLoginURL...");
          const loginURL = await GetAutoLoginURL();
          console.log("[AUTH] autoLoginURL:", loginURL);
          setAutoLoginURL(loginURL);
        }
        prevAPIReady = status.api_ready;
      } catch {
        // Silently ignore poll errors
      }
    }, 3000);

    return () => clearInterval(interval);
  }, []);

  const stateLabel =
    vmStatus.state.charAt(0).toUpperCase() + vmStatus.state.slice(1);

  // Don't render content until initial state is loaded to avoid flashing
  // stale defaults (e.g., license form when a key is already configured)
  if (!initialized) {
    return (
      <>
        <header
          className={`titlebar${isFullscreen ? " fullscreen" : ""}`}
          onDoubleClick={WindowToggleMaximise}
        >
          <div className="titlebar-logo">
            <img src="/helix-logo.png" alt="Helix" />
            <span>Helix</span>
          </div>
          <div className="titlebar-spacer" />
        </header>
        <main className="content">
          <div className="home-view">
            <div className="home-placeholder">
              <img src="/helix-logo.png" alt="Helix" className="home-logo" />
              <h2>Helix</h2>
              <p className="home-tagline">Your Private Agent Swarm</p>
              <div className="spinner" />
            </div>
          </div>
        </main>
      </>
    );
  }

  return (
    <>
      {/* Titlebar control bar */}
      <header
        className={`titlebar${isFullscreen ? " fullscreen" : ""}`}
        onDoubleClick={WindowToggleMaximise}
      >
        <div className="titlebar-logo">
          <img src="/helix-logo.png" alt="Helix" />
          <span>Helix</span>
        </div>

        <div
          className="titlebar-status"
          onDoubleClick={(e) => e.stopPropagation()}
        >
          <span className={`status-pill ${vmStatus.state}`} title={stateLabel}>
            <span className={`status-indicator ${vmStatus.state}`} />
            {stateLabel}
          </span>
          {vmStatus.state === "running" && vmStatus.api_ready && (
            <>
              <button
                className="titlebar-btn"
                onClick={() => {
                  const iframe = document.querySelector(
                    ".home-view iframe",
                  ) as HTMLIFrameElement;
                  if (iframe) {
                    const url = new URL(iframe.src);
                    iframe.src = url.origin + "/";
                  }
                }}
                title="Home"
              >
                <svg viewBox="0 0 20 20" width="14" height="14">
                  <path
                    d="M3 10l7-7 7 7M5 8.5V16a1 1 0 001 1h3v-4h2v4h3a1 1 0 001-1V8.5"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="1.5"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  />
                </svg>
              </button>
              <button
                className="titlebar-btn refresh-btn"
                onClick={() => {
                  const iframe = document.querySelector(
                    ".home-view iframe",
                  ) as HTMLIFrameElement;
                  if (iframe) {
                    iframe.src = iframe.src;
                  }
                }}
                title="Refresh"
              >
                <svg viewBox="0 0 20 20" width="14" height="14">
                  <path
                    d="M14.5 5.5A6 6 0 004.05 9M5.5 14.5A6 6 0 0015.95 11"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="1.5"
                    strokeLinecap="round"
                  />
                  <path
                    d="M14 2v4h-4M6 18v-4h4"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="1.5"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  />
                </svg>
              </button>
            </>
          )}
        </div>

        {vmStatus.state === "running" && vmStatus.api_ready && desktopQuota.max > 0 && (
          <div
            className="quota-pill"
            onDoubleClick={(e) => e.stopPropagation()}
            title={`${desktopQuota.active} of ${desktopQuota.max} desktop sessions`}
          >
            <div className="quota-segments">
              {Array.from({ length: desktopQuota.max }, (_, i) => (
                <div
                  key={i}
                  className={`quota-segment ${
                    i < desktopQuota.active
                      ? desktopQuota.active / desktopQuota.max >= 1
                        ? 'full'
                        : desktopQuota.active / desktopQuota.max >= 0.8
                          ? 'warn'
                          : 'active'
                      : ''
                  }`}
                />
              ))}
            </div>
            <span className="quota-label">{desktopQuota.active}/{desktopQuota.max}</span>
          </div>
        )}

        {/* Update badges */}
        {updateInfo?.available && !vmUpdateReady && !vmUpdateProgress && (
          <button
            className="update-badge"
            onDoubleClick={(e) => e.stopPropagation()}
            onClick={() => setSettingsOpen(true)}
            title={`Update available: v${updateInfo.latest_version}`}
          >
            Update v{updateInfo.latest_version}
          </button>
        )}

        {vmUpdateProgress && (
          <div
            className="update-progress-pill"
            onDoubleClick={(e) => e.stopPropagation()}
            title={`VM update: ${Math.round(vmUpdateProgress.percent || 0)}%`}
          >
            <div className="update-progress-bar">
              <div
                className="update-progress-fill"
                style={{ width: `${vmUpdateProgress.percent || 0}%` }}
              />
            </div>
            <span className="update-progress-label">
              {Math.round(vmUpdateProgress.percent || 0)}%
            </span>
          </div>
        )}

        {vmUpdateReady && (
          <button
            className="update-ready-badge"
            onDoubleClick={(e) => e.stopPropagation()}
            onClick={() => setSettingsOpen(true)}
            title="VM update downloaded — restart to apply"
          >
            Restart to update
          </button>
        )}

        <div className="titlebar-spacer" />

        <button
          className="discord-link"
          onDoubleClick={(e) => e.stopPropagation()}
          onClick={() => BrowserOpenURL("https://discord.gg/VJftd844GE")}
          title="Chat with us on Discord"
        >
          <svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor">
            <path d="M20.317 4.37a19.791 19.791 0 00-4.885-1.515.074.074 0 00-.079.037c-.21.375-.444.864-.608 1.25a18.27 18.27 0 00-5.487 0 12.64 12.64 0 00-.617-1.25.077.077 0 00-.079-.037A19.736 19.736 0 003.677 4.37a.07.07 0 00-.032.027C.533 9.046-.32 13.58.099 18.057a.082.082 0 00.031.057 19.9 19.9 0 005.993 3.03.078.078 0 00.084-.028c.462-.63.874-1.295 1.226-1.994a.076.076 0 00-.041-.106 13.107 13.107 0 01-1.872-.892.077.077 0 01-.008-.128 10.2 10.2 0 00.372-.292.074.074 0 01.077-.01c3.928 1.793 8.18 1.793 12.062 0a.074.074 0 01.078.01c.12.098.246.198.373.292a.077.077 0 01-.006.127 12.299 12.299 0 01-1.873.892.077.077 0 00-.041.107c.36.698.772 1.362 1.225 1.993a.076.076 0 00.084.028 19.839 19.839 0 006.002-3.03.077.077 0 00.032-.054c.5-5.177-.838-9.674-3.549-13.66a.061.061 0 00-.031-.03zM8.02 15.33c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.956-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.956 2.418-2.157 2.418zm7.975 0c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.955-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.946 2.418-2.157 2.418z" />
          </svg>
        </button>

        {licenseStatus.state === "trial_active" && (() => {
          const remaining = getTrialRemaining(licenseStatus.trial_ends_at);
          const endsAt = licenseStatus.trial_ends_at ? new Date(licenseStatus.trial_ends_at).getTime() : 0;
          const hoursLeft = endsAt ? (endsAt - Date.now()) / (1000 * 60 * 60) : 24;
          const urgent = hoursLeft < 4;
          return (
            <>
              <button
                className={`trial-badge-titlebar${urgent ? ' trial-urgent' : ''}`}
                onDoubleClick={(e) => e.stopPropagation()}
                onClick={() => BrowserOpenURL("https://deploy.helix.ml/licenses")}
              >
                Trial: {remaining}
              </button>
              <button
                className="buy-license-btn"
                onDoubleClick={(e) => e.stopPropagation()}
                onClick={() => BrowserOpenURL("https://deploy.helix.ml/licenses")}
              >
                Get a License
              </button>
            </>
          );
        })()}

        <button
          className="upsell-banner"
          onDoubleClick={(e) => e.stopPropagation()}
          onClick={() => BrowserOpenURL("https://helix.ml/docs/on-prem")}
          title="Deploy Helix on your own infrastructure"
        >
          <span className="upsell-sparkle">&#x2728;</span>
          Deploy for your team &mdash; your infra, your IP
          <svg viewBox="0 0 16 16" width="12" height="12">
            <path
              d="M5 3l6 5-6 5"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        </button>

        <div
          className="titlebar-actions"
          onDoubleClick={(e) => e.stopPropagation()}
        >
          {/* Console toggle */}
          <button
            className={`titlebar-btn ${consoleOpen ? "active" : ""}`}
            onClick={() => setConsoleOpen((v) => !v)}
            title="Console"
          >
            <svg viewBox="0 0 20 20" width="16" height="16">
              <path
                d="M3 4h14a1 1 0 011 1v10a1 1 0 01-1 1H3a1 1 0 01-1-1V5a1 1 0 011-1z"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.5"
              />
              <path
                d="M6 8l3 2-3 2M11 12h3"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
          </button>

          {/* Settings gear */}
          <button
            className={`titlebar-btn ${settingsOpen ? "active" : ""}`}
            onClick={() => setSettingsOpen((v) => !v)}
            title="Settings"
          >
            <svg viewBox="0 0 20 20" width="16" height="16">
              <path
                d="M10 13a3 3 0 100-6 3 3 0 000 6z"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.5"
              />
              <path
                d="M17 10a7 7 0 01-.4 2.3l1.5 1.2-1.4 2.4-1.8-.5a7 7 0 01-2 1.2L12.5 18h-2.8l-.4-1.4a7 7 0 01-2-1.2l-1.8.5-1.4-2.4 1.5-1.2a7 7 0 010-4.6L4.1 6.5l1.4-2.4 1.8.5a7 7 0 012-1.2L9.7 2h2.8l.4 1.4a7 7 0 012 1.2l1.8-.5 1.4 2.4-1.5 1.2A7 7 0 0117 10z"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.5"
              />
            </svg>
          </button>
        </div>
      </header>

      {/* Main content + console drawer */}
      <main className="content">
        <HomeView
          vmStatus={vmStatus}
          helixURL={helixURL}
          autoLoginURL={autoLoginURL}
          needsDownload={needsDownload}
          downloadProgress={downloadProgress}
          licenseStatus={licenseStatus}
          onProgressCleared={() => setDownloadProgress(null)}
          onLicenseUpdated={setLicenseStatus}
          showToast={showToast}
        />

        {consoleOpen && (
          <ConsoleDrawer
            vmState={vmStatus.state}
            onClose={() => setConsoleOpen(false)}
          />
        )}
      </main>

      {/* Settings slide-over panel */}
      {settingsOpen && (
        <SettingsPanel
          settings={settings}
          zfsStats={zfsStats}
          vmState={vmStatus.state}
          onSettingsUpdated={setSettings}
          onLicenseUpdated={setLicenseStatus}
          onClose={() => setSettingsOpen(false)}
          showToast={showToast}
          appVersion={appVersion}
          updateInfo={updateInfo}
          vmUpdateProgress={vmUpdateProgress}
          vmUpdateReady={vmUpdateReady}
        />
      )}

      {toastMessage && <div className="toast">{toastMessage}</div>}
    </>
  );
}
