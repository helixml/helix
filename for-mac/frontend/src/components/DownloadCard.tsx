import { main } from '../../wailsjs/go/models';
import { DownloadVMImages, CancelDownload } from '../../wailsjs/go/main/App';
import { formatBytes } from '../lib/helpers';

interface DownloadCardProps {
  needsDownload: boolean;
  downloadProgress: main.DownloadProgress | null;
  onDownloadComplete: () => void;
  onProgressCleared: () => void;
  showToast: (msg: string) => void;
}

export function DownloadCard({
  needsDownload,
  downloadProgress: p,
  onProgressCleared,
  showToast,
}: DownloadCardProps) {
  if (!needsDownload && !p) return null;

  async function handleStartDownload(e: React.MouseEvent<HTMLButtonElement>) {
    const btn = e.currentTarget;
    btn.disabled = true;
    btn.textContent = 'Starting download...';
    try {
      await DownloadVMImages();
    } catch (err) {
      console.error('Download failed:', err);
      showToast('Failed to start download');
    }
  }

  function handleRetryDownload() {
    onProgressCleared();
    DownloadVMImages().catch((err) => {
      console.error('Retry download failed:', err);
    });
  }

  function handleCancelDownload() {
    CancelDownload();
    onProgressCleared();
  }

  // Download in progress
  if (p && (p.status === 'downloading' || p.status === 'verifying')) {
    return (
      <div className="card download-card">
        <div className="card-header">
          <h2>Downloading Helix Environment</h2>
          <span className="card-badge" style={{ color: 'var(--teal)' }}>
            {p.status === 'verifying' ? 'Verifying...' : `${(p.percent ?? 0).toFixed(1)}%`}
          </span>
        </div>
        <div className="card-body">
          <div className="download-file-info">
            <span className="download-filename">{p.file || ''}</span>
            <span className="download-size">
              {' '}&mdash; {formatBytes(p.bytes_done || 0)} / {formatBytes(p.bytes_total || 0)}
            </span>
          </div>
          <div className="progress-bar download-progress">
            <div
              className="progress-fill teal"
              style={{ width: `${p.percent || 0}%` }}
            />
          </div>
          <div className="download-stats">
            <span className="download-speed">{p.speed || '--'}</span>
            <span className="download-eta">ETA: {p.eta || '--'}</span>
          </div>
          <div className="btn-group" style={{ marginTop: 12 }}>
            <button className="btn btn-secondary btn-sm" onClick={handleCancelDownload}>
              Cancel
            </button>
          </div>
        </div>
      </div>
    );
  }

  // Download error
  if (p && p.status === 'error') {
    return (
      <div className="card download-card">
        <div className="card-header">
          <h2>Download Failed</h2>
        </div>
        <div className="card-body">
          <div className="error-msg">{p.error || 'Unknown error'}</div>
          <div className="btn-group" style={{ marginTop: 12 }}>
            <button className="btn btn-primary" onClick={handleRetryDownload}>
              Retry Download
            </button>
          </div>
        </div>
      </div>
    );
  }

  // Not yet started — show download prompt
  return (
    <div className="card download-card">
      <div className="card-header">
        <h2>Download Required</h2>
      </div>
      <div className="card-body">
        <p className="download-description">
          The Helix environment needs to be downloaded before you can get started.
          This is a one-time download of approximately 18 GB.
        </p>
        <div className="btn-group" style={{ marginTop: 12 }}>
          <button className="btn btn-primary" onClick={handleStartDownload}>
            Download
          </button>
        </div>
      </div>
    </div>
  );
}
