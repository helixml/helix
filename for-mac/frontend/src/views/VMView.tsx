import { main } from '../../wailsjs/go/models';
import { StartVM, StopVM, OpenHelixUI } from '../../wailsjs/go/main/App';
import { formatBytes, formatUptime } from '../lib/helpers';
import { DownloadCard } from '../components/DownloadCard';
import { LicenseCard } from '../components/LicenseCard';
import { ScanoutCard } from '../components/ScanoutCard';

interface VMViewProps {
  vmStatus: main.VMStatus;
  vmConfig: main.VMConfig;
  needsDownload: boolean;
  downloadProgress: main.DownloadProgress | null;
  licenseStatus: main.LicenseStatus;
  scanoutStats: main.ScanoutStats;
  onDownloadComplete: () => void;
  onProgressCleared: () => void;
  onLicenseUpdated: (status: main.LicenseStatus) => void;
  showToast: (msg: string) => void;
}

function shouldBlockVM(ls: main.LicenseStatus): boolean {
  return ls.state === 'trial_expired' || ls.state === 'no_trial';
}

export function VMView({
  vmStatus: s,
  vmConfig: config,
  needsDownload,
  downloadProgress,
  licenseStatus,
  scanoutStats,
  onDownloadComplete,
  onProgressCleared,
  onLicenseUpdated,
  showToast,
}: VMViewProps) {
  const stateLabel = s.state.charAt(0).toUpperCase() + s.state.slice(1);

  return (
    <div className="view-container">
      <div className="view-header">
        <h1>Environment</h1>
        <p>Manage the Helix runtime environment</p>
      </div>

      <DownloadCard
        needsDownload={needsDownload}
        downloadProgress={downloadProgress}
        onDownloadComplete={onDownloadComplete}
        onProgressCleared={onProgressCleared}
        showToast={showToast}
      />

      <LicenseCard
        licenseStatus={licenseStatus}
        onLicenseUpdated={onLicenseUpdated}
        showToast={showToast}
      />

      <div className="card">
        <div className="card-header">
          <h2>Status</h2>
          <span className={`status-badge ${s.state}`}>
            <span className={`status-indicator ${s.state}`} />
            {stateLabel}
          </span>
        </div>
        <div className="card-body">
          <div className="stats-grid">
            <div className="stat-item">
              <div className="stat-label">CPU Cores</div>
              <div className="stat-value">{config.cpus || 4}</div>
            </div>
            <div className="stat-item">
              <div className="stat-label">Memory</div>
              <div className="stat-value">
                {formatBytes((config.memory_mb || 8192) * 1024 * 1024)}
              </div>
            </div>
            <div className="stat-item">
              <div className="stat-label">Uptime</div>
              <div className="stat-value small">{formatUptime(s.uptime || 0)}</div>
            </div>
            <div className="stat-item">
              <div className="stat-label">Sessions</div>
              <div className="stat-value">{s.sessions || 0}</div>
            </div>
          </div>

          {s.error_msg && <div className="error-msg">{s.error_msg}</div>}

          <div className="btn-group" style={{ marginTop: 16 }}>
            {(s.state === 'stopped' || s.state === 'error') && (
              <button
                className="btn btn-success"
                disabled={needsDownload || shouldBlockVM(licenseStatus)}
                onClick={async (e) => {
                  const btn = e.currentTarget;
                  btn.disabled = true;
                  btn.textContent = 'Starting...';
                  try {
                    await StartVM();
                  } catch (err) {
                    console.error('Start VM failed:', err);
                  }
                }}
              >
                Start
              </button>
            )}
            {s.state === 'running' && (
              <button
                className="btn btn-danger"
                onClick={async (e) => {
                  const btn = e.currentTarget;
                  btn.disabled = true;
                  btn.textContent = 'Stopping...';
                  try {
                    await StopVM();
                  } catch (err) {
                    console.error('Stop VM failed:', err);
                  }
                }}
              >
                Stop
              </button>
            )}
            {s.state !== 'stopped' &&
              s.state !== 'error' &&
              s.state !== 'running' && (
                <button className="btn btn-secondary" disabled>
                  {stateLabel}...
                </button>
              )}
            {s.state === 'running' && s.api_ready && (
              <button className="btn btn-primary" onClick={() => OpenHelixUI()}>
                Open Helix UI
              </button>
            )}
          </div>
        </div>
      </div>

      <ScanoutCard scanoutStats={scanoutStats} vmState={s.state} />
    </div>
  );
}
