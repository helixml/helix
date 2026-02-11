import { useState } from 'react';
import { main } from '../../wailsjs/go/models';
import { SaveSettings, GetSettings, ResizeDataDisk } from '../../wailsjs/go/main/App';

interface SettingsViewProps {
  settings: main.AppSettings;
  onSettingsUpdated: (settings: main.AppSettings) => void;
  showToast: (msg: string) => void;
}

export function SettingsView({ settings: s, onSettingsUpdated, showToast }: SettingsViewProps) {
  const [cpus, setCpus] = useState(String(s.vm_cpus || 4));
  const [memory, setMemory] = useState(String(s.vm_memory_mb || 8192));
  const [diskPath, setDiskPath] = useState(s.vm_disk_path || '');
  const [dataDiskSize, setDataDiskSize] = useState(String(s.data_disk_size_gb || 256));
  const [sshPort, setSshPort] = useState(String(s.ssh_port || 2222));
  const [apiPort, setApiPort] = useState(String(s.api_port || 8080));
  const [resizing, setResizing] = useState(false);

  async function handleSave() {
    const settings = new main.AppSettings({
      vm_cpus: parseInt(cpus) || 4,
      vm_memory_mb: parseInt(memory) || 8192,
      vm_disk_path: diskPath,
      data_disk_size_gb: s.data_disk_size_gb || 256,
      ssh_port: parseInt(sshPort) || 2222,
      api_port: parseInt(apiPort) || 8080,
      video_port: s.video_port || 8765,
      auto_start_vm: false,
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
      setMemory(String(fresh.vm_memory_mb || 8192));
      setDiskPath(fresh.vm_disk_path || '');
      setDataDiskSize(String(fresh.data_disk_size_gb || 256));
      setSshPort(String(fresh.ssh_port || 2222));
      setApiPort(String(fresh.api_port || 8080));
      showToast('Settings reset');
    } catch (err) {
      console.error('Failed to reset settings:', err);
    }
  }

  async function handleResizeDataDisk() {
    const newSize = parseInt(dataDiskSize);
    const currentSize = s.data_disk_size_gb || 256;

    if (!newSize || newSize <= currentSize) {
      showToast(`Size must be larger than current (${currentSize} GB)`);
      return;
    }

    if (!confirm(`Resize data disk from ${currentSize} GB to ${newSize} GB?\n\nThe VM will be stopped and restarted if running.`)) {
      return;
    }

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
    <div className="view-container">
      <div className="view-header">
        <h1>Settings</h1>
        <p>Configure VM and network settings</p>
      </div>

      <div className="card">
        <div className="card-header">
          <h2>Virtual Machine</h2>
        </div>
        <div className="card-body">
          <div className="form-row">
            <div className="form-group">
              <label className="form-label" htmlFor="settingCPUs">
                CPU Cores
              </label>
              <input
                className="form-input"
                id="settingCPUs"
                type="text"
                value={cpus}
                onChange={(e) => setCpus(e.target.value)}
                placeholder="4"
              />
              <div className="form-hint">vCPUs allocated to the VM</div>
            </div>
            <div className="form-group">
              <label className="form-label" htmlFor="settingMemory">
                Memory (MB)
              </label>
              <input
                className="form-input"
                id="settingMemory"
                type="text"
                value={memory}
                onChange={(e) => setMemory(e.target.value)}
                placeholder="8192"
              />
              <div className="form-hint">RAM in megabytes (8192 = 8 GB)</div>
            </div>
          </div>
          <div className="form-group">
            <label className="form-label" htmlFor="settingDisk">
              VM Disk Path
            </label>
            <input
              className="form-input mono"
              id="settingDisk"
              type="text"
              value={diskPath}
              onChange={(e) => setDiskPath(e.target.value)}
              placeholder="~/.helix/vm/helix-ubuntu.qcow2"
            />
          </div>
        </div>
      </div>

      <div className="card">
        <div className="card-header">
          <h2>Data Disk</h2>
        </div>
        <div className="card-body">
          <div className="form-row">
            <div className="form-group">
              <label className="form-label" htmlFor="settingDataDisk">
                Data Disk Size (GB)
              </label>
              <input
                className="form-input"
                id="settingDataDisk"
                type="text"
                value={dataDiskSize}
                onChange={(e) => setDataDiskSize(e.target.value)}
                placeholder="256"
              />
              <div className="form-hint">
                Data disk for agent storage, workspaces, and swap. Can only be increased.
              </div>
            </div>
          </div>
          <button
            className="btn btn-secondary"
            style={{ marginTop: 4 }}
            onClick={handleResizeDataDisk}
            disabled={resizing}
          >
            {resizing ? 'Resizing...' : 'Resize Data Disk'}
          </button>
        </div>
      </div>

      <div className="card">
        <div className="card-header">
          <h2>Network Ports</h2>
        </div>
        <div className="card-body">
          <div className="form-row">
            <div className="form-group">
              <label className="form-label" htmlFor="settingSSH">
                SSH Port
              </label>
              <input
                className="form-input"
                id="settingSSH"
                type="text"
                value={sshPort}
                onChange={(e) => setSshPort(e.target.value)}
                placeholder="2222"
              />
            </div>
            <div className="form-group">
              <label className="form-label" htmlFor="settingAPI">
                API Port
              </label>
              <input
                className="form-input"
                id="settingAPI"
                type="text"
                value={apiPort}
                onChange={(e) => setApiPort(e.target.value)}
                placeholder="8080"
              />
            </div>
          </div>
        </div>
      </div>

      <div className="btn-group" style={{ marginTop: 8 }}>
        <button className="btn btn-primary" onClick={handleSave}>
          Save Settings
        </button>
        <button className="btn btn-secondary" onClick={handleReset}>
          Reset to Defaults
        </button>
      </div>
    </div>
  );
}
