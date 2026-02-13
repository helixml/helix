import { useState, useEffect } from 'react';
import { main } from '../../wailsjs/go/models';
import { SaveSettings, GetSettings, ResizeDataDisk, GetHelixURL, FactoryReset as FactoryResetGo, GetLANAddress, ValidateLicenseKey, GetLicenseStatus } from '../../wailsjs/go/main/App';
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime';
import { formatBytes } from '../lib/helpers';

interface SettingsPanelProps {
  settings: main.AppSettings;
  zfsStats: main.ZFSStats;
  vmState: string;
  onSettingsUpdated: (settings: main.AppSettings) => void;
  onClose: () => void;
  showToast: (msg: string) => void;
}

// Eye-open SVG icon
const EyeIcon = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>
);

// Eye-off SVG icon
const EyeOffIcon = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94"/><path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19"/><line x1="1" y1="1" x2="23" y2="23"/></svg>
);

// Copy SVG icon
const CopyIcon = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
);

function SecretField({ value, onCopy }: { value: string; onCopy: () => void }) {
  const [visible, setVisible] = useState(false);
  return (
    <div className="secret-field">
      <input
        className="form-input secret-input"
        type={visible ? 'text' : 'password'}
        value={value}
        readOnly
      />
      <button className="secret-btn" onClick={() => setVisible(!visible)} title={visible ? 'Hide' : 'Show'}>
        {visible ? <EyeOffIcon /> : <EyeIcon />}
      </button>
      <button className="secret-btn" onClick={onCopy} title="Copy">
        <CopyIcon />
      </button>
    </div>
  );
}

export function SettingsPanel({
  settings: s,
  zfsStats: zfs,
  vmState,
  onSettingsUpdated,
  onClose,
  showToast,
}: SettingsPanelProps) {
  const [cpus, setCpus] = useState(String(s.vm_cpus || 4));
  const [memoryGB, setMemoryGB] = useState(String(Math.round((s.vm_memory_mb || 8192) / 1024)));
  const [dataDiskSize, setDataDiskSize] = useState(String(s.data_disk_size_gb || 256));
  const [apiPort, setApiPort] = useState(String(s.api_port || 8080));
  const [autoStart, setAutoStart] = useState(s.auto_start_vm || false);
  const [exposeOnNetwork, setExposeOnNetwork] = useState(s.expose_on_network || false);
  const [newUsersAdmin, setNewUsersAdmin] = useState(s.new_users_are_admin || false);
  const [allowRegistration, setAllowRegistration] = useState(s.allow_registration !== false);
  const [resizing, setResizing] = useState(false);
  const [loginURL, setLoginURL] = useState('');
  const [confirmReset, setConfirmReset] = useState(false);
  const [confirmResize, setConfirmResize] = useState(false);
  const [resetting, setResetting] = useState(false);
  const [fullWipe, setFullWipe] = useState(false);
  const [lanIP, setLanIP] = useState('');
  const [showAdminLogin, setShowAdminLogin] = useState(false);
  const [licenseKey, setLicenseKey] = useState(s.license_key || '');
  const [licenseStatus, setLicenseStatus] = useState('');
  const [licenseActivating, setLicenseActivating] = useState(false);

  useEffect(() => {
    GetHelixURL().then((url) => {
      setLoginURL(`${url}/api/v1/auth/desktop-callback?token=${s.desktop_secret || ''}`);
    }).catch(() => {});
    GetLANAddress().then(setLanIP).catch(() => {});
    GetLicenseStatus().then((status) => {
      setLicenseStatus(status.state || '');
    }).catch(() => {});
  }, []);

  const currentDiskSize = s.data_disk_size_gb || 256;
  const parsedDiskSize = parseInt(dataDiskSize);
  const diskSizeError =
    parsedDiskSize && parsedDiskSize < currentDiskSize
      ? `Cannot shrink below current size (${currentDiskSize} GB)`
      : '';

  const hasStorageData = zfs.pool_size > 0;
  const poolPct = hasStorageData ? Math.round((zfs.pool_used / zfs.pool_size) * 100) : 0;

  async function handleSave() {
    const settings = new main.AppSettings({
      vm_cpus: parseInt(cpus) || 4,
      vm_memory_mb: (parseInt(memoryGB) || 8) * 1024,
      vm_disk_path: s.vm_disk_path,
      data_disk_size_gb: s.data_disk_size_gb || 256,
      ssh_port: s.ssh_port || 41222,
      api_port: parseInt(apiPort) || 41080,
      auto_start_vm: autoStart,
      expose_on_network: exposeOnNetwork,
      new_users_are_admin: newUsersAdmin,
      allow_registration: allowRegistration,
    });

    try {
      await SaveSettings(settings);
      onSettingsUpdated(settings);
      showToast('Settings saved');
    } catch (err) {
      console.error('Failed to save settings:', err);
      showToast('Failed to save settings');
    }
  }

  async function handleReset() {
    try {
      const fresh = await GetSettings();
      onSettingsUpdated(fresh);
      setCpus(String(fresh.vm_cpus || 4));
      setMemoryGB(String(Math.round((fresh.vm_memory_mb || 8192) / 1024)));
      setDataDiskSize(String(fresh.data_disk_size_gb || 256));
      setApiPort(String(fresh.api_port || 41080));
      setAutoStart(fresh.auto_start_vm || false);
      setExposeOnNetwork(fresh.expose_on_network || false);
      setNewUsersAdmin(fresh.new_users_are_admin || false);
      setAllowRegistration(fresh.allow_registration !== false);
      showToast('Settings reset');
    } catch (err) {
      console.error('Failed to reset settings:', err);
    }
  }

  async function handleResizeDataDisk() {
    const newSize = parseInt(dataDiskSize);
    if (!newSize || newSize <= currentDiskSize) {
      showToast(`Size must be larger than current (${currentDiskSize} GB)`);
      return;
    }

    if (!confirmResize) {
      setConfirmResize(true);
      return;
    }

    setConfirmResize(false);
    setResizing(true);
    try {
      await ResizeDataDisk(newSize);
      const updated = new main.AppSettings({ ...s, data_disk_size_gb: newSize });
      onSettingsUpdated(updated);
      showToast(`Data disk resized to ${newSize} GB`);
    } catch (err) {
      console.error('Failed to resize data disk:', err);
      showToast('Failed to resize data disk: ' + err);
    } finally {
      setResizing(false);
    }
  }

  return (
    <>
      <div className="panel-overlay" onClick={onClose} />
      <div className="settings-panel">
        <div className="panel-header" style={{ ['--wails-draggable' as any]: 'drag' }}>
          <h2>Settings</h2>
          <button className="panel-close" onClick={onClose} style={{ ['--wails-draggable' as any]: 'no-drag' }}>&times;</button>
        </div>

        <div className="panel-body">
          {/* Resources */}
          <div className="panel-section">
            <div className="panel-section-title">Resources</div>
            <div className="form-row">
              <div className="form-group">
                <label className="form-label" htmlFor="settingCPUs">CPU Cores</label>
                <input
                  className="form-input"
                  id="settingCPUs"
                  type="text"
                  value={cpus}
                  onChange={(e) => setCpus(e.target.value)}
                  placeholder="4"
                />
              </div>
              <div className="form-group">
                <label className="form-label" htmlFor="settingMemory">Memory (GB)</label>
                <input
                  className="form-input"
                  id="settingMemory"
                  type="text"
                  value={memoryGB}
                  onChange={(e) => setMemoryGB(e.target.value)}
                  placeholder="8"
                />
              </div>
            </div>
          </div>

          {/* Storage */}
          <div className="panel-section">
            <div className="panel-section-title">Storage</div>
            <div className="form-row">
              <div className="form-group">
                <label className="form-label" htmlFor="settingDataDisk">Data Disk (GB)</label>
                <input
                  className="form-input"
                  id="settingDataDisk"
                  type="text"
                  value={dataDiskSize}
                  onChange={(e) => setDataDiskSize(e.target.value)}
                  placeholder="256"
                />
                {diskSizeError ? (
                  <div className="error-msg" style={{ marginTop: 4, fontSize: 12 }}>{diskSizeError}</div>
                ) : (
                  <div className="form-hint">Can only be increased</div>
                )}
              </div>
              <div className="form-group" style={{ display: 'flex', alignItems: 'flex-end', gap: 8 }}>
                {confirmResize && !resizing && (
                  <button
                    className="btn btn-secondary btn-sm"
                    onClick={() => setConfirmResize(false)}
                    style={{ marginBottom: 16 }}
                  >
                    Cancel
                  </button>
                )}
                <button
                  className={`btn btn-sm ${confirmResize ? 'btn-danger' : 'btn-secondary'}`}
                  onClick={handleResizeDataDisk}
                  disabled={resizing || !!diskSizeError || !parsedDiskSize || parsedDiskSize <= currentDiskSize}
                  style={{ marginBottom: 16 }}
                >
                  {resizing ? 'Resizing...' : confirmResize ? 'Confirm resize' : 'Resize'}
                </button>
              </div>
            </div>

            {hasStorageData && vmState === 'running' && (
              <div style={{ marginTop: 8 }}>
                <div className="progress-bar" style={{ marginBottom: 8 }}>
                  <div
                    className={`progress-fill ${poolPct > 90 ? 'error' : poolPct > 75 ? 'warning' : 'teal'}`}
                    style={{ width: `${poolPct}%` }}
                  />
                </div>
                <div className="storage-row">
                  <span className="label">Used</span>
                  <span className="value">{formatBytes(zfs.pool_used)}</span>
                </div>
                <div className="storage-row">
                  <span className="label">Available</span>
                  <span className="value">{formatBytes(zfs.pool_available)}</span>
                </div>
                {zfs.dedup_ratio > 1 && (
                  <div className="storage-row">
                    <span className="label">Dedup ratio</span>
                    <span className="value" style={{ color: 'var(--teal)' }}>{zfs.dedup_ratio.toFixed(2)}x</span>
                  </div>
                )}
              </div>
            )}
          </div>

          {/* Network */}
          <div className="panel-section">
            <div className="panel-section-title">Network</div>
            <div className="form-group">
              <label className="form-label toggle-label" htmlFor="settingExposeNetwork">
                <input
                  type="checkbox"
                  id="settingExposeNetwork"
                  checked={exposeOnNetwork}
                  onChange={(e) => setExposeOnNetwork(e.target.checked)}
                />
                <span>Share on network</span>
              </label>
              <div className="form-hint">
                Allow other devices on your network to access Helix.
              </div>
              {exposeOnNetwork && lanIP && s.expose_on_network && vmState === 'running' && (
                <div className="form-hint" style={{ marginTop: 4 }}>
                  Network URL:{' '}
                  <a
                    href="#"
                    style={{ color: 'var(--teal)', textDecoration: 'underline', cursor: 'pointer' }}
                    onClick={(e) => {
                      e.preventDefault();
                      const url = `http://${lanIP}:${s.api_port || 41080}`;
                      BrowserOpenURL(url);
                    }}
                  >
                    http://{lanIP}:{s.api_port || 41080}
                  </a>
                </div>
              )}
            </div>
            {exposeOnNetwork && (
              <>
                <div className="form-group">
                  <label className="form-label toggle-label" htmlFor="settingAllowRegistration">
                    <input
                      type="checkbox"
                      id="settingAllowRegistration"
                      checked={allowRegistration}
                      onChange={(e) => setAllowRegistration(e.target.checked)}
                    />
                    <span>Allow registration</span>
                  </label>
                  <div className="form-hint">
                    Allow new users to create accounts via the web UI.
                  </div>
                </div>
                <div className="form-group">
                  <label className="form-label toggle-label" htmlFor="settingNewUsersAdmin">
                    <input
                      type="checkbox"
                      id="settingNewUsersAdmin"
                      checked={newUsersAdmin}
                      onChange={(e) => setNewUsersAdmin(e.target.checked)}
                    />
                    <span>All users are admins</span>
                  </label>
                  <div className="form-hint">
                    Grant admin privileges to all users who access Helix.
                  </div>
                </div>
              </>
            )}
            <div className="form-group">
              <label className="form-label" htmlFor="settingAPI">API Port</label>
              <input
                className="form-input"
                id="settingAPI"
                type="text"
                value={apiPort}
                onChange={(e) => setApiPort(e.target.value)}
                placeholder="41080"
              />
            </div>
          </div>

          {/* Access */}
          <div className="panel-section">
            <div className="panel-section-title">Access</div>
            <div className="form-group">
              <label className="form-label">Helix URL</label>
              <div className="form-hint" style={{ marginBottom: 6 }}>
                {exposeOnNetwork && lanIP && s.expose_on_network && vmState === 'running'
                  ? 'Share this URL with others on your network. They can create their own accounts.'
                  : 'Access Helix in your browser.'}
              </div>
              {(() => {
                const port = s.api_port || 41080;
                const localURL = `http://localhost:${port}`;
                const networkURL = exposeOnNetwork && lanIP && s.expose_on_network ? `http://${lanIP}:${port}` : '';
                const displayURL = networkURL || localURL;
                return (
                  <div style={{ display: 'flex', gap: 6 }}>
                    <input
                      className="form-input"
                      type="text"
                      value={displayURL}
                      readOnly
                      style={{ flex: 1, fontSize: 12, fontFamily: 'monospace' }}
                      onClick={(e) => (e.target as HTMLInputElement).select()}
                    />
                    <button
                      className="btn btn-secondary btn-sm"
                      onClick={() => {
                        navigator.clipboard.writeText(displayURL);
                        showToast('URL copied');
                      }}
                      title="Copy"
                    >
                      <CopyIcon />
                    </button>
                    <button
                      className="btn btn-secondary btn-sm"
                      onClick={() => BrowserOpenURL(displayURL)}
                      title="Open in browser"
                    >
                      Open
                    </button>
                  </div>
                );
              })()}
            </div>
            <div className="form-group">
              <button
                className="btn btn-secondary btn-sm"
                onClick={() => setShowAdminLogin(!showAdminLogin)}
                style={{ fontSize: 11, padding: '4px 10px' }}
              >
                {showAdminLogin ? 'Hide' : 'Show'} admin login
              </button>
              {showAdminLogin && (
                <div style={{ marginTop: 10 }}>
                  <div className="form-hint" style={{ marginBottom: 8 }}>
                    Use this to log in as admin from another device on your network.
                  </div>
                  <div style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 8 }}>
                    <span className="form-label" style={{ marginBottom: 0, minWidth: 60 }}>User</span>
                    <input
                      className="form-input"
                      type="text"
                      value="admin@helix-desktop.local"
                      readOnly
                      style={{ flex: 1, fontSize: 12, fontFamily: 'monospace' }}
                      onClick={(e) => (e.target as HTMLInputElement).select()}
                    />
                    <button
                      className="btn btn-secondary btn-sm"
                      onClick={() => {
                        navigator.clipboard.writeText('admin@helix-desktop.local');
                        showToast('Username copied');
                      }}
                      title="Copy"
                    >
                      <CopyIcon />
                    </button>
                  </div>
                  <div className="form-hint" style={{ marginBottom: 4 }}>
                    Opens in your browser and logs you in automatically.
                  </div>
                  <button
                    className="btn btn-primary btn-sm"
                    onClick={() => BrowserOpenURL(loginURL)}
                  >
                    Log in as admin
                  </button>
                </div>
              )}
            </div>
            {s.console_password && (
              <div className="form-group">
                <label className="form-label">Console Password</label>
                <SecretField
                  value={s.console_password}
                  onCopy={() => {
                    navigator.clipboard.writeText(s.console_password || '');
                    showToast('Password copied');
                  }}
                />
                <div className="form-hint">
                  VM serial console login (user: ubuntu).
                </div>
              </div>
            )}
          </div>

          {/* General */}
          <div className="panel-section">
            <div className="panel-section-title">General</div>
            <div className="form-group">
              <label className="form-label toggle-label" htmlFor="settingAutoStart">
                <input
                  type="checkbox"
                  id="settingAutoStart"
                  checked={autoStart}
                  onChange={(e) => setAutoStart(e.target.checked)}
                />
                <span>Start VM when app opens</span>
              </label>
            </div>
          </div>

          {/* License */}
          <div className="panel-section">
            <div className="panel-section-title">License</div>
            <div className="form-group">
              <label className="form-label" htmlFor="settingLicenseKey">License Key</label>
              <div style={{ display: 'flex', gap: 6 }}>
                <input
                  className="form-input"
                  id="settingLicenseKey"
                  type="text"
                  value={licenseKey}
                  onChange={(e) => setLicenseKey(e.target.value)}
                  placeholder="Paste your license key"
                  style={{ flex: 1, fontSize: 12, fontFamily: 'monospace' }}
                />
                <button
                  className="btn btn-primary btn-sm"
                  disabled={licenseActivating || !licenseKey.trim()}
                  onClick={async () => {
                    setLicenseActivating(true);
                    try {
                      await ValidateLicenseKey(licenseKey.trim());
                      const status = await GetLicenseStatus();
                      setLicenseStatus(status.state || '');
                      showToast('License activated');
                    } catch (err) {
                      showToast('Invalid license key: ' + err);
                    } finally {
                      setLicenseActivating(false);
                    }
                  }}
                >
                  {licenseActivating ? 'Activating...' : 'Activate'}
                </button>
              </div>
              {licenseStatus === 'valid' && (
                <div className="form-hint" style={{ color: 'var(--teal)', marginTop: 4 }}>
                  License active
                </div>
              )}
              {licenseStatus === 'expired' && (
                <div className="form-hint" style={{ color: 'var(--warning)', marginTop: 4 }}>
                  License expired
                </div>
              )}
            </div>
          </div>

          {/* Danger Zone */}
          <div className="panel-section" style={{ borderColor: 'rgba(239, 68, 68, 0.3)' }}>
            <div className="panel-section-title" style={{ color: 'var(--error)' }}>Danger Zone</div>
            <div className="form-group">
              <div className="form-hint" style={{ marginBottom: 12 }}>
                Delete VM disk images and downloaded files. Your license key and settings will be preserved.
              </div>
              <div className="form-hint" style={{ marginBottom: 12, color: 'var(--text-faded)' }}>
                Hold Shift for full wipe including settings and license key.
              </div>
              {resetting ? (
                <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                  <div className="spinner" />
                  <span style={{ color: 'var(--text-secondary)', fontSize: 13 }}>Resetting...</span>
                </div>
              ) : !confirmReset ? (
                <button
                  className="btn btn-danger"
                  onClick={(e) => {
                    setFullWipe(e.shiftKey);
                    setConfirmReset(true);
                  }}
                >
                  Factory Reset
                </button>
              ) : (
                <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                  <span style={{ color: 'var(--error)', fontSize: 13 }}>
                    {fullWipe ? 'Delete everything including settings?' : 'Reset VM? Settings will be kept.'}
                  </span>
                  <button
                    className="btn btn-danger"
                    onClick={async () => {
                      setResetting(true);
                      setConfirmReset(false);
                      try {
                        await FactoryResetGo(fullWipe);
                      } catch (err) {
                        console.error('Factory reset failed:', err);
                        showToast('Factory reset failed: ' + err);
                        setResetting(false);
                      }
                    }}
                  >
                    {fullWipe ? 'Yes, delete everything' : 'Yes, reset VM'}
                  </button>
                  <button
                    className="btn btn-secondary"
                    onClick={() => setConfirmReset(false)}
                  >
                    Cancel
                  </button>
                </div>
              )}
            </div>
          </div>
        </div>

        <div className="panel-footer">
          <div className="btn-group">
            <button className="btn btn-primary" onClick={handleSave}>Save</button>
            <button className="btn btn-secondary" onClick={handleReset}>Reset</button>
          </div>
        </div>
      </div>
    </>
  );
}
