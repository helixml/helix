import { main } from '../../wailsjs/go/models';

interface ScanoutCardProps {
  scanoutStats: main.ScanoutStats;
  vmState: string;
}

export function ScanoutCard({ scanoutStats: sc, vmState }: ScanoutCardProps) {
  const hasData = sc.total_connectors > 0;
  const active = sc.active_displays || 0;
  const max = sc.max_scanouts || 15;
  const usagePct = Math.round((active / max) * 100);

  if (!hasData && vmState !== 'running') return null;

  return (
    <div className="card">
      <div className="card-header">
        <h2>Displays</h2>
        <span className="card-badge" style={{ color: 'var(--text-faded)' }}>
          DRM Scanout
        </span>
      </div>
      <div className="card-body">
        {hasData ? (
          <>
            <div className="scanout-usage">
              <div className="scanout-count">
                <span className="scanout-active teal">{active}</span>
                <span className="scanout-sep">/</span>
                <span className="scanout-max">{max}</span>
              </div>
              <div className="scanout-label">displays in use</div>
            </div>
            <div className="progress-bar" style={{ margin: '12px 0' }}>
              <div
                className={`progress-fill ${usagePct > 90 ? 'warning' : 'teal'}`}
                style={{ width: `${usagePct}%` }}
              />
            </div>
            {sc.displays && sc.displays.length > 0 && (
              <div className="display-list">
                {sc.displays
                  .filter((d) => d.connected)
                  .map((d) => (
                    <div className="display-item" key={d.name}>
                      <span className="status-indicator running" />
                      <span className="display-name">{d.name}</span>
                      {d.width > 0 && (
                        <span className="display-res">
                          {d.width}x{d.height}
                        </span>
                      )}
                    </div>
                  ))}
              </div>
            )}
          </>
        ) : (
          <div className="empty-state">
            {sc.error
              ? `Unable to fetch display stats: ${sc.error}`
              : 'Loading display stats...'}
          </div>
        )}
      </div>
    </div>
  );
}
