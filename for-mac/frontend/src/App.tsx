import { useState, useEffect, useCallback } from 'react';
import './style.css';
import { main } from '../wailsjs/go/models';
import {
  GetVMStatus,
  GetVMConfig,
  IsVMImageReady,
  GetHelixURL,
  GetSystemInfo,
  GetZFSStats,
  GetSettings,
  GetScanoutStats,
  GetLicenseStatus,
} from '../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime';
import { HomeView } from './views/HomeView';
import { VMView } from './views/VMView';
import { StorageView } from './views/StorageView';
import { SettingsView } from './views/SettingsView';

type View = 'home' | 'vm' | 'storage' | 'settings';

const defaultVMStatus = new main.VMStatus({
  state: 'stopped',
  cpu_percent: 0,
  memory_used: 0,
  uptime: 0,
  sessions: 0,
  api_ready: false,
  video_ready: false,
});

const defaultVMConfig = new main.VMConfig({
  name: '',
  cpus: 4,
  memory_mb: 8192,
  disk_path: '',
  vsock_cid: 0,
  ssh_port: 2222,
  api_port: 8080,
  video_port: 8765,
  qmp_port: 0,
});

const defaultZFSStats = new main.ZFSStats({
  pool_name: '',
  pool_size: 0,
  pool_used: 0,
  pool_available: 0,
  dedup_ratio: 1,
  compression_ratio: 1,
  last_updated: '',
});

const defaultScanoutStats = new main.ScanoutStats({
  total_connectors: 0,
  active_displays: 0,
  max_scanouts: 15,
  last_updated: '',
});

const defaultSettings = new main.AppSettings({
  vm_cpus: 4,
  vm_memory_mb: 8192,
  data_disk_size_gb: 256,
  ssh_port: 2222,
  api_port: 8080,
  video_port: 8765,
  auto_start_vm: false,
  vm_disk_path: '',
});

const defaultLicenseStatus = new main.LicenseStatus({
  state: 'no_trial',
});

const NAV_ITEMS: { view: View; label: string; icon: JSX.Element }[] = [
  {
    view: 'home',
    label: 'Home',
    icon: (
      <svg viewBox="0 0 20 20" width="18" height="18">
        <path d="M10 2L2 8v10h6v-6h4v6h6V8L10 2z" fill="currentColor" />
      </svg>
    ),
  },
  {
    view: 'vm',
    label: 'VM',
    icon: (
      <svg viewBox="0 0 20 20" width="18" height="18">
        <path
          d="M3 4h14a1 1 0 011 1v8a1 1 0 01-1 1H3a1 1 0 01-1-1V5a1 1 0 011-1zm4 12h6m-3-2v2"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
    ),
  },
  {
    view: 'storage',
    label: 'Storage',
    icon: (
      <svg viewBox="0 0 20 20" width="18" height="18">
        <path
          d="M4 5h12a1 1 0 011 1v2a1 1 0 01-1 1H4a1 1 0 01-1-1V6a1 1 0 011-1zm0 6h12a1 1 0 011 1v2a1 1 0 01-1 1H4a1 1 0 01-1-1v-2a1 1 0 011-1z"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
        />
        <circle cx="6" cy="7" r="1" fill="currentColor" />
        <circle cx="6" cy="13" r="1" fill="currentColor" />
      </svg>
    ),
  },
  {
    view: 'settings',
    label: 'Settings',
    icon: (
      <svg viewBox="0 0 20 20" width="18" height="18">
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
    ),
  },
];

export function App() {
  const [currentView, setCurrentView] = useState<View>('home');
  const [vmStatus, setVmStatus] = useState<main.VMStatus>(defaultVMStatus);
  const [vmConfig, setVmConfig] = useState<main.VMConfig>(defaultVMConfig);
  const [vmImageReady, setVmImageReady] = useState(false);
  const [needsDownload, setNeedsDownload] = useState(false);
  const [helixURL, setHelixURL] = useState('');
  const [zfsStats, setZfsStats] = useState<main.ZFSStats>(defaultZFSStats);
  const [scanoutStats, setScanoutStats] = useState<main.ScanoutStats>(defaultScanoutStats);
  const [settings, setSettings] = useState<main.AppSettings>(defaultSettings);
  const [downloadProgress, setDownloadProgress] = useState<main.DownloadProgress | null>(null);
  const [licenseStatus, setLicenseStatus] = useState<main.LicenseStatus>(defaultLicenseStatus);
  const [toastMessage, setToastMessage] = useState<string | null>(null);

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
        const [imageReady, _sysInfo, status, config, url, appSettings, license] =
          await Promise.all([
            IsVMImageReady(),
            GetSystemInfo(),
            GetVMStatus(),
            GetVMConfig(),
            GetHelixURL(),
            GetSettings(),
            GetLicenseStatus(),
          ]);
        setVmImageReady(imageReady);
        setNeedsDownload(!imageReady);
        setVmStatus(status);
        setVmConfig(config);
        setHelixURL(url);
        setSettings(appSettings);
        setLicenseStatus(license);
      } catch (err) {
        console.error('Failed to load initial state:', err);
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

    return () => {
      EventsOff('vm:status');
      EventsOff('download:progress');
    };
  }, []);

  // Poll for stats
  useEffect(() => {
    const interval = setInterval(async () => {
      try {
        const status = await GetVMStatus();
        setVmStatus(status);

        if (status.state === 'running') {
          const [zfs, scanout] = await Promise.all([
            GetZFSStats(),
            GetScanoutStats(),
          ]);
          setZfsStats(zfs);
          setScanoutStats(scanout);
        }
      } catch {
        // Silently ignore poll errors
      }
    }, 3000);

    return () => clearInterval(interval);
  }, []);

  const stateLabel = vmStatus.state.charAt(0).toUpperCase() + vmStatus.state.slice(1);
  const sessions = vmStatus.sessions || 0;

  return (
    <>
      <nav className="sidebar">
        <div className="sidebar-header" style={{ '--wails-draggable': 'drag' } as React.CSSProperties}>
          <div className="sidebar-logo">
            <img
              src="/helix-logo.png"
              alt="Helix"
              className="helix-icon-img"
              width="22"
              height="22"
            />
            <span className="sidebar-title">Helix</span>
          </div>
          <div className="sidebar-status" />
        </div>
        <div className="sidebar-nav">
          {NAV_ITEMS.map((item) => (
            <button
              key={item.view}
              className={`nav-item ${currentView === item.view ? 'active' : ''}`}
              onClick={() => setCurrentView(item.view)}
            >
              {item.icon}
              <span>{item.label}</span>
            </button>
          ))}
        </div>
        <div className="sidebar-footer">
          <div className="sidebar-vm-status">
            <span className={`status-indicator ${vmStatus.state}`} />
            <span>
              VM {stateLabel}
              {vmStatus.state === 'running' &&
                ` \u00B7 ${sessions} session${sessions !== 1 ? 's' : ''}`}
            </span>
          </div>
        </div>
      </nav>

      <main className="content">
        <div className="titlebar-drag" style={{ '--wails-draggable': 'drag' } as React.CSSProperties} />
        {currentView === 'home' && (
          <HomeView
            vmStatus={vmStatus}
            helixURL={helixURL}
            needsDownload={needsDownload}
            downloadProgress={downloadProgress}
            onProgressCleared={() => setDownloadProgress(null)}
            showToast={showToast}
          />
        )}
        {currentView === 'vm' && (
          <VMView
            vmStatus={vmStatus}
            vmConfig={vmConfig}
            needsDownload={needsDownload}
            downloadProgress={downloadProgress}
            licenseStatus={licenseStatus}
            scanoutStats={scanoutStats}
            onDownloadComplete={() => {
              setNeedsDownload(false);
              setVmImageReady(true);
              setDownloadProgress(null);
            }}
            onProgressCleared={() => setDownloadProgress(null)}
            onLicenseUpdated={setLicenseStatus}
            showToast={showToast}
          />
        )}
        {currentView === 'storage' && (
          <StorageView zfsStats={zfsStats} vmState={vmStatus.state} />
        )}
        {currentView === 'settings' && (
          <SettingsView
            settings={settings}
            onSettingsUpdated={setSettings}
            showToast={showToast}
          />
        )}
      </main>

      {toastMessage && <div className="toast">{toastMessage}</div>}
    </>
  );
}
