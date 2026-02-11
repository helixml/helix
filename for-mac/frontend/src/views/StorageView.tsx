import { main } from '../../wailsjs/go/models';
import { formatBytes } from '../lib/helpers';

interface StorageViewProps {
  zfsStats: main.ZFSStats;
  diskUsage: main.DiskUsage;
  vmState: string;
}

export function StorageView({ zfsStats: zfs, diskUsage: disk, vmState }: StorageViewProps) {
  const hasData = zfs.pool_size > 0;
  const poolPct = hasData ? Math.round((zfs.pool_used / zfs.pool_size) * 100) : 0;
  const rootPct =
    disk.root_disk_total > 0
      ? Math.round((disk.root_disk_used / disk.root_disk_total) * 100)
      : 0;

  return (
    <div className="view-container">
      <div className="view-header">
        <h1>Storage</h1>
        <p>Deduplicated agent storage and disk usage</p>
      </div>

      {hasData && (
        <div className="dedup-highlight">
          <div>
            <div className="dedup-value">{(zfs.dedup_ratio || 1).toFixed(2)}x</div>
            <div className="dedup-label">Deduplication</div>
          </div>
          <div style={{ marginLeft: 24 }}>
            <div className="dedup-value" style={{ fontSize: 22 }}>
              {(zfs.compression_ratio || 1).toFixed(2)}x
            </div>
            <div className="dedup-label">Compression</div>
          </div>
        </div>
      )}

      <div className="card">
        <div className="card-header">
          <h2>Agent Storage Pool</h2>
          <span className="card-badge" style={{ color: 'var(--text-faded)' }}>
            Deduplicated
          </span>
        </div>
        <div className="card-body">
          {hasData ? (
            <>
              <div className="progress-bar" style={{ marginBottom: 16 }}>
                <div
                  className={`progress-fill ${
                    poolPct > 90 ? 'error' : poolPct > 75 ? 'warning' : 'teal'
                  }`}
                  style={{ width: `${poolPct}%` }}
                />
              </div>
              <div className="stats-grid">
                <div className="stat-item">
                  <div className="stat-label">Pool Size</div>
                  <div className="stat-value small">{formatBytes(zfs.pool_size)}</div>
                </div>
                <div className="stat-item">
                  <div className="stat-label">Used</div>
                  <div className="stat-value small">{formatBytes(zfs.pool_used)}</div>
                </div>
                <div className="stat-item">
                  <div className="stat-label">Available</div>
                  <div className="stat-value small success">
                    {formatBytes(zfs.pool_available)}
                  </div>
                </div>
                <div className="stat-item">
                  <div className="stat-label">Usage</div>
                  <div className={`stat-value small ${poolPct > 90 ? 'warning' : ''}`}>
                    {poolPct}%
                  </div>
                </div>
              </div>
            </>
          ) : (
            <div className="empty-state">
              {vmState !== 'running'
                ? 'Start the VM to view storage stats.'
                : zfs.error
                  ? `Unable to fetch storage stats: ${zfs.error}`
                  : 'Loading storage stats...'}
            </div>
          )}
        </div>
      </div>

      <div className="card">
        <div className="card-header">
          <h2>Host Disk</h2>
        </div>
        <div className="card-body">
          {disk.root_disk_total > 0 ? (
            <div className="storage-breakdown">
              <div className="storage-row">
                <span className="label">Root Disk</span>
                <span className="value">
                  {formatBytes(disk.root_disk_used)} / {formatBytes(disk.root_disk_total)}
                </span>
              </div>
              <div className="progress-bar">
                <div
                  className={`progress-fill ${
                    rootPct > 90 ? 'error' : rootPct > 75 ? 'warning' : 'teal'
                  }`}
                  style={{ width: `${rootPct}%` }}
                />
              </div>
              {disk.host_actual > 0 && (
                <div className="storage-row" style={{ marginTop: 8 }}>
                  <span className="label">Actual on disk (after deduplication)</span>
                  <span className="value">{formatBytes(disk.host_actual)}</span>
                </div>
              )}
            </div>
          ) : (
            <div className="empty-state">
              {vmState !== 'running' ? 'Start the VM to view disk usage.' : 'Loading disk usage...'}
            </div>
          )}
        </div>
      </div>

      {zfs.last_updated && (
        <div
          style={{
            textAlign: 'right',
            fontSize: 11,
            color: 'var(--text-faded)',
            marginTop: 8,
          }}
        >
          Last updated: {new Date(zfs.last_updated).toLocaleTimeString()}
        </div>
      )}
    </div>
  );
}
