import { useState, useEffect, useRef } from 'react';
import { main } from '../../wailsjs/go/models';
import {
  StartVM,
  DownloadVMImages,
  CancelDownload,
} from '../../wailsjs/go/main/App';
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime';
import { formatBytes } from '../lib/helpers';
import { LicenseCard } from '../components/LicenseCard';

const DEMO_VIDEO_URL = 'https://www.youtube.com/watch?v=_dv4V1pYbKU';

const BOOT_STAGES = [
  { prefix: 'Booting', startPct: 3, endPct: 25, durationMs: 45000 },
  { prefix: 'Setting up storage', startPct: 27, endPct: 45, durationMs: 60000 },
  { prefix: 'Configuring', startPct: 47, endPct: 55, durationMs: 15000 },
  { prefix: 'Starting Helix', startPct: 57, endPct: 75, durationMs: 30000 },
  { prefix: 'Starting app', startPct: 77, endPct: 90, durationMs: 120000 },
  { prefix: 'Waiting for sandbox', startPct: 92, endPct: 99, durationMs: 30000 },
];

interface HomeViewProps {
  vmStatus: main.VMStatus;
  helixURL: string;
  autoLoginURL: string;
  needsDownload: boolean;
  downloadProgress: main.DownloadProgress | null;
  licenseStatus: main.LicenseStatus;
  onProgressCleared: () => void;
  onLicenseUpdated: (status: main.LicenseStatus) => void;
  showToast: (msg: string) => void;
}

function shouldBlockVM(ls: main.LicenseStatus): boolean {
  return ls.state === 'trial_expired' || ls.state === 'no_trial';
}

export function HomeView({
  vmStatus,
  helixURL,
  autoLoginURL,
  needsDownload,
  downloadProgress: p,
  licenseStatus,
  onProgressCleared,
  onLicenseUpdated,
  showToast,
}: HomeViewProps) {
  // Show Helix UI when VM is running, API is ready, AND auth proxy URL is available.
  // Wait for autoLoginURL so the iframe goes straight to the cookie-setting endpoint
  // and redirects to / — no flash of the login screen.
  if (vmStatus.state === 'running' && vmStatus.api_ready && autoLoginURL) {
    console.log('[AUTH] Rendering iframe with src:', autoLoginURL);
    return (
      <div className="home-view">
        <iframe src={autoLoginURL} title="Helix" allow="clipboard-read; clipboard-write" />
      </div>
    );
  }

  // Show boot progress when VM is starting, or running but not yet fully ready
  // (API not ready, or API ready but auth proxy URL not yet available)
  if (vmStatus.state === 'starting' || (vmStatus.state === 'running' && (!vmStatus.api_ready || !autoLoginURL))) {
    return (
      <div className="home-view">
        <div className="home-placeholder download-screen">
          <img src="/helix-logo.png" alt="Helix" className="home-logo" />
          <h2>Starting Helix</h2>
          <BootProgress stage={vmStatus.boot_stage || 'Booting VM...'} />
          <div className="download-extras">
            <button
              className="demo-video-link"
              onClick={() => BrowserOpenURL(DEMO_VIDEO_URL)}
            >
              <div className="demo-video-thumb-wrap">
                <img src="/demo-thumb.jpg" alt="Demo video" className="demo-video-thumb" />
                <span className="demo-video-play-overlay">&#9654;</span>
              </div>
              <div className="demo-video-text">
                Watch the demo video while you wait
                <svg viewBox="0 0 16 16" width="12" height="12">
                  <path d="M5 3l6 5-6 5" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
                </svg>
              </div>
            </button>
            {licenseStatus.state !== 'licensed' && (
              <LicenseCard
                licenseStatus={licenseStatus}
                onLicenseUpdated={onLicenseUpdated}
                showToast={showToast}
              />
            )}
          </div>
        </div>
      </div>
    );
  }

  // First boot: need to download VM images
  if (needsDownload) {
    const isDownloading = p && (p.status === 'downloading' || p.status === 'verifying' || p.status === 'decompressing' || p.status === 'finalizing');
    return (
      <div className="home-view">
        <div className={`home-placeholder${isDownloading ? ' download-screen' : ''}`}>
          <img src="/helix-logo.png" alt="Helix" className="home-logo" />
          <h2>Helix Desktop</h2>
          <p className="home-tagline">Your Private Agent Swarm</p>
          {!isDownloading && (
            <p>
              Download the Helix environment to get started. This is a one-time download and
              may take a few minutes depending on your connection.
            </p>
          )}
          <HomeDownloadSection
            downloadProgress={p}
            onProgressCleared={onProgressCleared}
            showToast={showToast}
          />
          {isDownloading && (
            <div className="download-extras">
              <div className="download-extras-divider">
                <span>While you wait</span>
              </div>
              <button
                className="demo-video-link"
                onClick={() => BrowserOpenURL(DEMO_VIDEO_URL)}
              >
                <div className="demo-video-thumb-wrap">
                  <img src="/demo-thumb.jpg" alt="Demo video" className="demo-video-thumb" />
                  <span className="demo-video-play-overlay">&#9654;</span>
                </div>
                <div className="demo-video-text">
                  Watch the demo video
                  <svg viewBox="0 0 16 16" width="12" height="12">
                    <path d="M5 3l6 5-6 5" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
                  </svg>
                </div>
              </button>
              {licenseStatus.state !== 'licensed' && (
                <LicenseCard
                  licenseStatus={licenseStatus}
                  onLicenseUpdated={onLicenseUpdated}
                  showToast={showToast}
                />
              )}
            </div>
          )}
        </div>
      </div>
    );
  }

  // VM images ready but not running — show setup/start screen
  const blocked = shouldBlockVM(licenseStatus);

  return (
    <div className="home-view">
      <div className="home-placeholder">
        <img src="/helix-logo.png" alt="Helix" className="home-logo" />
        <h2>Helix Desktop</h2>
        <p className="home-tagline">Your Private Agent Swarm</p>

        {/* License gate */}
        {blocked && (
          <div style={{ width: '100%', maxWidth: 480 }}>
            <LicenseCard
              licenseStatus={licenseStatus}
              onLicenseUpdated={onLicenseUpdated}
              showToast={showToast}
            />
          </div>
        )}

        {!blocked && licenseStatus.state === 'trial_active' && (
          <LicenseCard
            licenseStatus={licenseStatus}
            onLicenseUpdated={onLicenseUpdated}
            showToast={showToast}
          />
        )}

        {!blocked && licenseStatus.state === 'licensed' && (
          <LicenseCard
            licenseStatus={licenseStatus}
            onLicenseUpdated={onLicenseUpdated}
            showToast={showToast}
          />
        )}

        {vmStatus.state === 'stopped' && !blocked && (
          <>
            <p>Ready to start.</p>
            <button
              className="btn btn-primary"
              onClick={async () => {
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
        {vmStatus.state === 'error' && (
          <>
            <div className="error-msg">{vmStatus.error_msg || 'Failed to start'}</div>
            <button
              className="btn btn-primary"
              onClick={async () => {
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
  if (p && (p.status === 'downloading' || p.status === 'verifying' || p.status === 'decompressing' || p.status === 'finalizing')) {
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
            {p.status === 'finalizing' ? 'Finalizing...' : p.status === 'verifying' ? 'Verifying...' : p.status === 'decompressing' ? 'Decompressing...' : p.eta ? `${p.eta} remaining` : `${(p.percent || 0).toFixed(1)}%`}
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
          onClick={async () => {
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
      onClick={async () => {
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

function BootProgress({ stage }: { stage: string }) {
  const stageInfo = BOOT_STAGES.find(s => stage.startsWith(s.prefix)) || BOOT_STAGES[0];
  const prevPrefix = useRef(stageInfo.prefix);
  const stageStartRef = useRef(Date.now());
  const [bootPercent, setBootPercent] = useState(stageInfo.startPct);

  // When stage changes, reset the timer and jump to the new start percentage
  useEffect(() => {
    if (stageInfo.prefix !== prevPrefix.current) {
      prevPrefix.current = stageInfo.prefix;
      stageStartRef.current = Date.now();
      setBootPercent(stageInfo.startPct);
    }
  }, [stageInfo.prefix, stageInfo.startPct]);

  // Smoothly animate towards the end percentage using easeOutCubic
  useEffect(() => {
    const interval = setInterval(() => {
      const elapsed = Date.now() - stageStartRef.current;
      const progress = Math.min(elapsed / stageInfo.durationMs, 1);
      const eased = 1 - Math.pow(1 - progress, 3);
      const pct = stageInfo.startPct + (stageInfo.endPct - stageInfo.startPct) * eased;
      setBootPercent(Math.round(pct));
    }, 100);
    return () => clearInterval(interval);
  }, [stageInfo.prefix]);

  return (
    <div className="boot-progress">
      <div className="progress-bar">
        <div className="progress-fill teal boot-progress-fill" style={{ width: `${bootPercent}%` }} />
      </div>
      <div className="boot-stage-label">{stage}</div>
    </div>
  );
}
