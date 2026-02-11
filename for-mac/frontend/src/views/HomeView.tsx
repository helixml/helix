import { main } from '../../wailsjs/go/models';
import {
  StartVM,
  DownloadVMImages,
  CancelDownload,
} from '../../wailsjs/go/main/App';
import { formatBytes } from '../lib/helpers';

interface HomeViewProps {
  vmStatus: main.VMStatus;
  helixURL: string;
  needsDownload: boolean;
  downloadProgress: main.DownloadProgress | null;
  onProgressCleared: () => void;
  showToast: (msg: string) => void;
}

export function HomeView({
  vmStatus,
  helixURL,
  needsDownload,
  downloadProgress: p,
  onProgressCleared,
  showToast,
}: HomeViewProps) {
  // Show Helix UI when VM is running and API is ready
  if (vmStatus.state === 'running' && vmStatus.api_ready) {
    return (
      <div className="home-view">
        <iframe src={helixURL} title="Helix" />
      </div>
    );
  }

  // First boot: need to download VM images
  if (needsDownload) {
    return (
      <div className="home-view">
        <div className="home-placeholder">
          <img src="/helix-logo.png" alt="Helix" className="home-logo" />
          <h2>Welcome to Helix</h2>
          <p>
            Download the Helix environment to get started. This is a one-time download of
            approximately 18 GB.
          </p>
          <HomeDownloadSection
            downloadProgress={p}
            onProgressCleared={onProgressCleared}
            showToast={showToast}
          />
        </div>
      </div>
    );
  }

  // VM images ready but not running
  return (
    <div className="home-view">
      <div className="home-placeholder">
        <img src="/helix-logo.png" alt="Helix" className="home-logo" />
        <h2>Helix Desktop</h2>
        {vmStatus.state === 'stopped' && (
          <>
            <p>Ready to start.</p>
            <button
              className="btn btn-primary"
              onClick={async (e) => {
                const btn = e.currentTarget;
                btn.disabled = true;
                btn.textContent = 'Starting...';
                try {
                  await StartVM();
                } catch (err) {
                  console.error('Start VM failed:', err);
                  showToast('Failed to start');
                }
              }}
            >
              Start Helix
            </button>
          </>
        )}
        {vmStatus.state === 'starting' && (
          <div className="status-badge starting">
            <span className="status-indicator starting" />
            Starting...
          </div>
        )}
        {vmStatus.state === 'error' && (
          <>
            <div className="error-msg">{vmStatus.error_msg || 'Failed to start'}</div>
            <button
              className="btn btn-primary"
              onClick={async (e) => {
                const btn = e.currentTarget;
                btn.disabled = true;
                btn.textContent = 'Starting...';
                try {
                  await StartVM();
                } catch (err) {
                  console.error('Start VM failed:', err);
                  showToast('Failed to start');
                }
              }}
            >
              Retry
            </button>
          </>
        )}
      </div>
    </div>
  );
}

function HomeDownloadSection({
  downloadProgress: p,
  onProgressCleared,
  showToast,
}: {
  downloadProgress: main.DownloadProgress | null;
  onProgressCleared: () => void;
  showToast: (msg: string) => void;
}) {
  if (p && (p.status === 'downloading' || p.status === 'verifying')) {
    return (
      <div className="home-download-progress">
        <div className="download-file-info">
          <span className="download-filename">{p.file || ''}</span>
          <span className="download-size">
            {' '}&mdash; {formatBytes(p.bytes_done || 0)} / {formatBytes(p.bytes_total || 0)}
          </span>
        </div>
        <div className="progress-bar download-progress">
          <div className="progress-fill teal" style={{ width: `${p.percent || 0}%` }} />
        </div>
        <div className="download-stats">
          <span className="download-speed">{p.speed || '--'}</span>
          <span className="download-eta">
            {p.status === 'verifying' ? 'Verifying...' : `${(p.percent || 0).toFixed(1)}%`}
          </span>
        </div>
        <button
          className="btn btn-secondary btn-sm"
          style={{ marginTop: 12 }}
          onClick={() => {
            CancelDownload();
            onProgressCleared();
          }}
        >
          Cancel
        </button>
      </div>
    );
  }

  if (p && p.status === 'error') {
    return (
      <>
        <div className="error-msg" style={{ marginBottom: 12 }}>
          {p.error || 'Download failed'}
        </div>
        <button
          className="btn btn-primary"
          onClick={async (e) => {
            const btn = e.currentTarget;
            btn.disabled = true;
            btn.textContent = 'Starting download...';
            try {
              await DownloadVMImages();
            } catch (err) {
              console.error('Download failed:', err);
              showToast('Failed to start download');
            }
          }}
        >
          Retry Download
        </button>
      </>
    );
  }

  return (
    <button
      className="btn btn-primary"
      onClick={async (e) => {
        const btn = e.currentTarget;
        btn.disabled = true;
        btn.textContent = 'Starting download...';
        try {
          await DownloadVMImages();
        } catch (err) {
          console.error('Download failed:', err);
          showToast('Failed to start download');
        }
      }}
    >
      Download
    </button>
  );
}
