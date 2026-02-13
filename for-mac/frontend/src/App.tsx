import { useState, useEffect, useCallback } from 'react';
import './style.css';
import { main } from '../wailsjs/go/models';
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
} from '../wailsjs/go/main/App';
import { EventsOn, EventsOff, WindowToggleMaximise, WindowIsFullscreen, BrowserOpenURL } from '../wailsjs/runtime/runtime';
import { HomeView } from './views/HomeView';
import { SettingsPanel } from './components/SettingsPanel';
import { ConsoleDrawer } from './components/ConsoleDrawer';
import { getTrialRemaining } from './lib/helpers';

const defaultVMStatus = new main.VMStatus({
  state: 'stopped',
  cpu_percent: 0,
  memory_used: 0,
  uptime: 0,
  sessions: 0,
  api_ready: false,
});

const defaultVMConfig = new main.VMConfig({
  name: '',
  cpus: 4,
  memory_mb: 8192,
  disk_path: '',
  vsock_cid: 0,
  ssh_port: 2222,
  api_port: 8080,
  qmp_port: 0,
});

const defaultZFSStats = new main.ZFSStats({
  pool_name: '',
  pool_size: 0,
  pool_used: 0,
  pool_available: 0,
  dedup_ratio: 1,
  compression_ratio: 1,
  dedup_saved_bytes: 0,
  datasets: [],
  last_updated: '',
});

const defaultSettings = new main.AppSettings({
  vm_cpus: 4,
  vm_memory_mb: 8192,
  data_disk_size_gb: 256,
  ssh_port: 41222,
  api_port: 41080,
  auto_start_vm: false,
  vm_disk_path: '',
});

const defaultLicenseStatus = new main.LicenseStatus({
  state: 'no_trial',
});

export function App() {
  const [vmStatus, setVmStatus] = useState<main.VMStatus>(defaultVMStatus);
  const [vmConfig, setVmConfig] = useState<main.VMConfig>(defaultVMConfig);
  const [vmImageReady, setVmImageReady] = useState(false);
  const [needsDownload, setNeedsDownload] = useState(false);
  const [helixURL, setHelixURL] = useState('');
  const [autoLoginURL, setAutoLoginURL] = useState('');
  const [zfsStats, setZfsStats] = useState<main.ZFSStats>(defaultZFSStats);
  const [settings, setSettings] = useState<main.AppSettings>(defaultSettings);
  const [downloadProgress, setDownloadProgress] = useState<main.DownloadProgress | null>(null);
  const [licenseStatus, setLicenseStatus] = useState<main.LicenseStatus>(defaultLicenseStatus);
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [consoleOpen, setConsoleOpen] = useState(false);
  const [initialized, setInitialized] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);

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
        const [imageReady, _sysInfo, status, config, url, loginURL, appSettings, license] =
          await Promise.all([
            IsVMImageReady(),
            GetSystemInfo(),
            GetVMStatus(),
            GetVMConfig(),
            GetHelixURL(),
            GetAutoLoginURL(),
            GetSettings(),
            GetLicenseStatus(),
          ]);
        setVmImageReady(imageReady);
        setNeedsDownload(!imageReady);
        setVmStatus(status);
        setVmConfig(config);
        setHelixURL(url);
        setAutoLoginURL(loginURL);
        setSettings(appSettings);
        setLicenseStatus(license);
      } catch (err) {
        console.error('Failed to load initial state:', err);
      } finally {
        setInitialized(true);
      }
    })();
  }, []);

  // Subscribe to events
  useEffect(() => {
    EventsOn('vm:status', (status: main.VMStatus) => {
      setVmStatus(status);
    });

    EventsOn('download:progress', (progress: main.DownloadProgress) => {
      if (progress.status === 'complete') {
        setNeedsDownload(false);
        setVmImageReady(true);
        setDownloadProgress(null);
      } else if (progress.status === 'error') {
        setDownloadProgress(progress);
      } else {
        setDownloadProgress(progress);
      }
    });

    // Listen for settings:show event from system tray
    EventsOn('settings:show', () => {
      setSettingsOpen(true);
    });

    return () => {
      EventsOff('vm:status');
      EventsOff('download:progress');
      EventsOff('settings:show');
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

        if (status.state === 'running') {
          const zfs = await GetZFSStats();
          setZfsStats(zfs);
        }

        // Re-fetch autoLoginURL when API transitions to ready
        if (status.api_ready && !prevAPIReady) {
          console.log('[AUTH] API became ready, fetching autoLoginURL...');
          const loginURL = await GetAutoLoginURL();
          console.log('[AUTH] autoLoginURL:', loginURL);
          setAutoLoginURL(loginURL);
        }
        prevAPIReady = status.api_ready;
      } catch {
        // Silently ignore poll errors
      }
    }, 3000);

    return () => clearInterval(interval);
  }, []);

  const stateLabel = vmStatus.state.charAt(0).toUpperCase() + vmStatus.state.slice(1);

  // Don't render content until initial state is loaded to avoid flashing
  // stale defaults (e.g., license form when a key is already configured)
  if (!initialized) {
    return (
      <>
        <header className={`titlebar${isFullscreen ? ' fullscreen' : ''}`} onDoubleClick={WindowToggleMaximise}>
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
      <header className={`titlebar${isFullscreen ? ' fullscreen' : ''}`} onDoubleClick={WindowToggleMaximise}>
        <div className="titlebar-logo">
          <img src="/helix-logo.png" alt="Helix" />
          <span>Helix</span>
        </div>

        <div className="titlebar-status" onDoubleClick={e => e.stopPropagation()}>
          <span className={`status-pill ${vmStatus.state}`} title={stateLabel}>
            <span className={`status-indicator ${vmStatus.state}`} />
            {stateLabel}
          </span>
          {vmStatus.state === 'running' && vmStatus.api_ready && (
            <button
              className="titlebar-btn refresh-btn"
              onClick={() => {
                const iframe = document.querySelector('.home-view iframe') as HTMLIFrameElement;
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
          )}
        </div>

        <div className="titlebar-spacer" />

        {licenseStatus.state === 'trial_active' && (
          <span className="trial-badge-titlebar" onDoubleClick={e => e.stopPropagation()}>
            Trial: {getTrialRemaining(licenseStatus.trial_ends_at)}
          </span>
        )}

        <button
          className="upsell-banner"
          onDoubleClick={e => e.stopPropagation()}
          onClick={() => BrowserOpenURL('https://helix.ml/cluster')}
          title="Deploy Helix on your own infrastructure"
        >
          <span className="upsell-sparkle">&#x2728;</span>
          Deploy for your team &mdash; your servers, your data
          <svg viewBox="0 0 16 16" width="12" height="12">
            <path d="M5 3l6 5-6 5" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
          </svg>
        </button>

        <div className="titlebar-actions" onDoubleClick={e => e.stopPropagation()}>
          {/* Console toggle */}
          <button
            className={`titlebar-btn ${consoleOpen ? 'active' : ''}`}
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
            className={`titlebar-btn ${settingsOpen ? 'active' : ''}`}
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
          onClose={() => setSettingsOpen(false)}
          showToast={showToast}
        />
      )}

      {toastMessage && <div className="toast">{toastMessage}</div>}
    </>
  );
}
