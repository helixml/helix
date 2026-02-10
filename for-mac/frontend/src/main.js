import './style.css';
import {
    GetVMStatus,
    GetVMConfig,
    SetVMConfig,
    StartVM,
    StopVM,
    GetEncoderStats,
    GetClientCount,
    CheckDependencies,
    IsVMImageReady,
    OpenHelixUI,
    GetHelixURL,
    GetSystemInfo,
    GetZFSStats,
    GetDiskUsage,
    GetSettings,
    SaveSettings,
    GetScanoutStats,
    DownloadVMImages,
    CancelDownload,
    GetDownloadStatus,
    GetLicenseStatus,
    ValidateLicenseKey,
    StartTrial,
} from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

// ---- State ----

let state = {
    currentView: 'home',
    vmStatus: { state: 'stopped' },
    vmConfig: {},
    encoderStats: {},
    clientCount: 0,
    dependencies: {},
    vmImageReady: false,
    systemInfo: {},
    helixURL: '',
    zfsStats: {},
    diskUsage: {},
    scanoutStats: {},
    settings: {},
    downloadProgress: null,
    needsDownload: false,
    licenseStatus: { state: 'no_trial' },
};

// ---- Init ----

async function init() {
    // Load initial state
    try {
        const [deps, imageReady, sysInfo, vmStatus, vmConfig, helixURL, settings, licenseStatus] = await Promise.all([
            CheckDependencies(),
            IsVMImageReady(),
            GetSystemInfo(),
            GetVMStatus(),
            GetVMConfig(),
            GetHelixURL(),
            GetSettings(),
            GetLicenseStatus(),
        ]);
        state.dependencies = deps;
        state.vmImageReady = imageReady;
        state.needsDownload = !imageReady;
        state.systemInfo = sysInfo;
        state.vmStatus = vmStatus;
        state.vmConfig = vmConfig;
        state.helixURL = helixURL;
        state.settings = settings;
        state.licenseStatus = licenseStatus;
    } catch (err) {
        console.error('Failed to load initial state:', err);
    }

    // Subscribe to VM status updates
    EventsOn('vm:status', (status) => {
        state.vmStatus = status;
        updateSidebarStatus();
        if (state.currentView === 'vm') renderContent();
    });

    // Subscribe to download progress events
    EventsOn('download:progress', (progress) => {
        state.downloadProgress = progress;
        if (progress.status === 'complete') {
            state.needsDownload = false;
            state.vmImageReady = true;
            state.downloadProgress = null;
        }
        if (state.currentView === 'vm') renderContent();
    });

    // Set up sidebar navigation
    document.querySelectorAll('.nav-item').forEach(btn => {
        btn.addEventListener('click', () => {
            const view = btn.dataset.view;
            if (view) switchView(view);
        });
    });

    // Poll for stats
    setInterval(pollStats, 3000);

    // Initial render
    updateSidebarStatus();
    renderContent();
}

async function pollStats() {
    try {
        state.vmStatus = await GetVMStatus();
        updateSidebarStatus();

        if (state.vmStatus.state === 'running') {
            const [stats, clients, zfs, disk, scanout] = await Promise.all([
                GetEncoderStats(),
                GetClientCount(),
                GetZFSStats(),
                GetDiskUsage(),
                GetScanoutStats(),
            ]);
            state.encoderStats = stats;
            state.clientCount = clients;
            state.zfsStats = zfs;
            state.diskUsage = disk;
            state.scanoutStats = scanout;

            // Update current view if it shows live data
            if (state.currentView === 'vm' || state.currentView === 'storage') {
                renderContent();
            }
        }
    } catch (err) {
        // Silently ignore poll errors
    }
}

// ---- Navigation ----

function switchView(view) {
    state.currentView = view;

    // Update active nav item
    document.querySelectorAll('.nav-item').forEach(btn => {
        btn.classList.toggle('active', btn.dataset.view === view);
    });

    renderContent();
}

// ---- Sidebar Status ----

function updateSidebarStatus() {
    const el = document.getElementById('sidebarVMStatus');
    if (!el) return;

    const s = state.vmStatus;
    const stateLabel = s.state.charAt(0).toUpperCase() + s.state.slice(1);
    const sessions = s.sessions || 0;

    el.innerHTML = `
        <span class="status-indicator ${s.state}"></span>
        <span>VM ${stateLabel}${s.state === 'running' ? ` &middot; ${sessions} session${sessions !== 1 ? 's' : ''}` : ''}</span>
    `;
}

// ---- Content Rendering ----

function renderContent() {
    const content = document.getElementById('content');
    if (!content) return;

    switch (state.currentView) {
        case 'home':
            content.innerHTML = renderHomeView();
            break;
        case 'vm':
            content.innerHTML = renderVMView();
            attachVMHandlers();
            break;
        case 'storage':
            content.innerHTML = renderStorageView();
            break;
        case 'settings':
            content.innerHTML = renderSettingsView();
            attachSettingsHandlers();
            break;
    }
}

// ---- Home View ----

function renderHomeView() {
    if (state.vmStatus.state === 'running' && state.vmStatus.api_ready) {
        return `
            <div class="home-view">
                <iframe src="${state.helixURL}" title="Helix"></iframe>
            </div>
        `;
    }

    return `
        <div class="home-view">
            <div class="home-placeholder">
                <svg viewBox="0 0 48 48" width="48" height="48">
                    <polygon points="24,4 44,16 44,32 24,44 4,32 4,16" fill="none" stroke="currentColor" stroke-width="2.5"/>
                </svg>
                <h2>Helix Desktop</h2>
                <p>Start the VM to access the Helix web interface. The full Helix UI will be embedded here once the VM is running.</p>
                ${state.vmStatus.state === 'stopped' ? `
                    <button class="btn btn-primary" onclick="document.querySelector('[data-view=vm]').click()">Go to VM</button>
                ` : state.vmStatus.state === 'starting' ? `
                    <div class="status-badge starting">
                        <span class="status-indicator starting"></span>
                        Starting VM...
                    </div>
                ` : ''}
            </div>
        </div>
    `;
}

// ---- VM View ----

function renderVMView() {
    const s = state.vmStatus;
    const config = state.vmConfig;
    const stats = state.encoderStats;
    const stateLabel = s.state.charAt(0).toUpperCase() + s.state.slice(1);

    // Show download card if VM images need downloading
    const downloadSection = renderDownloadSection();
    // Show license section if needed
    const licenseSection = renderLicenseSection();

    return `
        <div class="view-container">
            <div class="view-header">
                <h1>Virtual Machine</h1>
                <p>Manage the Helix development VM</p>
            </div>

            ${downloadSection}
            ${licenseSection}

            <div class="card">
                <div class="card-header">
                    <h2>Status</h2>
                    <span class="status-badge ${s.state}">
                        <span class="status-indicator ${s.state}"></span>
                        ${stateLabel}
                    </span>
                </div>
                <div class="card-body">
                    <div class="stats-grid">
                        <div class="stat-item">
                            <div class="stat-label">CPU Cores</div>
                            <div class="stat-value">${config.cpus || 4}</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-label">Memory</div>
                            <div class="stat-value">${formatBytes((config.memory_mb || 8192) * 1024 * 1024)}</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-label">Uptime</div>
                            <div class="stat-value small">${formatUptime(s.uptime || 0)}</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-label">Sessions</div>
                            <div class="stat-value">${s.sessions || 0}</div>
                        </div>
                    </div>

                    ${s.error_msg ? `<div class="error-msg">${s.error_msg}</div>` : ''}

                    <div class="btn-group" style="margin-top: 16px;">
                        ${s.state === 'stopped' || s.state === 'error' ?
                            `<button class="btn btn-success" id="startVMBtn" ${state.needsDownload || shouldBlockVM() ? 'disabled' : ''}>Start VM</button>` :
                         s.state === 'running' ?
                            `<button class="btn btn-danger" id="stopVMBtn">Stop VM</button>` :
                            `<button class="btn btn-secondary" disabled>${stateLabel}...</button>`
                        }
                        ${s.state === 'running' && s.api_ready ?
                            `<button class="btn btn-primary" id="openUIBtn">Open Helix UI</button>` : ''
                        }
                    </div>
                </div>
            </div>

            <div class="card">
                <div class="card-header">
                    <h2>Video Encoding</h2>
                    <span class="card-badge" style="color: var(--text-faded);">VideoToolbox H.264</span>
                </div>
                <div class="card-body">
                    <div class="stats-grid">
                        <div class="stat-item">
                            <div class="stat-label">FPS</div>
                            <div class="stat-value teal">${(stats.current_fps || 0).toFixed(1)}</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-label">Clients</div>
                            <div class="stat-value">${state.clientCount}</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-label">Frames Encoded</div>
                            <div class="stat-value small">${formatNumber(stats.frames_encoded || 0)}</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-label">Dropped</div>
                            <div class="stat-value small ${(stats.frames_dropped || 0) > 0 ? 'warning' : ''}">${stats.frames_dropped || 0}</div>
                        </div>
                    </div>
                </div>
            </div>

            ${renderScanoutCard()}

            <div class="card">
                <div class="card-header">
                    <h2>Quick Actions</h2>
                </div>
                <div class="card-body">
                    <div class="action-list">
                        <div class="action-item">
                            <div class="action-info">
                                <h3>SSH to VM</h3>
                                <p>ssh -p ${config.ssh_port || 2222} helix@localhost</p>
                            </div>
                            <button class="btn btn-secondary btn-sm" id="copySSHBtn">Copy</button>
                        </div>
                        <div class="action-item">
                            <div class="action-info">
                                <h3>API Endpoint</h3>
                                <p>http://localhost:${config.api_port || 8080}</p>
                            </div>
                            <button class="btn btn-secondary btn-sm" id="copyAPIBtn">Copy</button>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    `;
}

// ---- Download Section ----

function renderDownloadSection() {
    if (!state.needsDownload && !state.downloadProgress) {
        return '';
    }

    const p = state.downloadProgress;

    // Download in progress
    if (p && (p.status === 'downloading' || p.status === 'verifying')) {
        return `
            <div class="card download-card">
                <div class="card-header">
                    <h2>Downloading VM Images</h2>
                    <span class="card-badge" style="color: var(--teal);">${p.status === 'verifying' ? 'Verifying...' : `${p.percent?.toFixed(1) || 0}%`}</span>
                </div>
                <div class="card-body">
                    <div class="download-file-info">
                        <span class="download-filename">${p.file || ''}</span>
                        <span class="download-size">${formatBytes(p.bytes_done || 0)} / ${formatBytes(p.bytes_total || 0)}</span>
                    </div>
                    <div class="progress-bar download-progress">
                        <div class="progress-fill teal" style="width: ${p.percent || 0}%;"></div>
                    </div>
                    <div class="download-stats">
                        <span class="download-speed">${p.speed || '--'}</span>
                        <span class="download-eta">ETA: ${p.eta || '--'}</span>
                    </div>
                    <div class="btn-group" style="margin-top: 12px;">
                        <button class="btn btn-secondary btn-sm" id="cancelDownloadBtn">Cancel</button>
                    </div>
                </div>
            </div>
        `;
    }

    // Download error
    if (p && p.status === 'error') {
        return `
            <div class="card download-card">
                <div class="card-header">
                    <h2>Download Failed</h2>
                </div>
                <div class="card-body">
                    <div class="error-msg">${p.error || 'Unknown error'}</div>
                    <div class="btn-group" style="margin-top: 12px;">
                        <button class="btn btn-primary" id="retryDownloadBtn">Retry Download</button>
                    </div>
                </div>
            </div>
        `;
    }

    // Not yet started — show download prompt
    return `
        <div class="card download-card">
            <div class="card-header">
                <h2>VM Images Required</h2>
            </div>
            <div class="card-body">
                <p class="download-description">
                    The VM disk images need to be downloaded before you can start the virtual machine.
                    This is a one-time download of approximately 18 GB.
                </p>
                <div class="btn-group" style="margin-top: 12px;">
                    <button class="btn btn-primary" id="startDownloadBtn">Download VM Images</button>
                </div>
            </div>
        </div>
    `;
}

// ---- License Section ----

function shouldBlockVM() {
    const ls = state.licenseStatus;
    return ls.state === 'trial_expired' || ls.state === 'no_trial';
}

function renderLicenseSection() {
    const ls = state.licenseStatus;

    if (ls.state === 'licensed') {
        return `
            <div class="license-badge licensed">
                <span class="license-badge-icon">&#10003;</span>
                Licensed to: ${ls.licensed_to || 'Unknown'}
                ${ls.expires_at ? `<span class="license-expires">(expires ${new Date(ls.expires_at).toLocaleDateString()})</span>` : ''}
            </div>
        `;
    }

    if (ls.state === 'trial_active') {
        const remaining = getTrialRemaining(ls.trial_ends_at);
        return `
            <div class="license-badge trial-active">
                <span class="license-badge-icon">&#9201;</span>
                Trial: ${remaining} remaining
            </div>
        `;
    }

    if (ls.state === 'trial_expired') {
        return `
            <div class="card license-card">
                <div class="card-header">
                    <h2>Trial Expired</h2>
                </div>
                <div class="card-body">
                    <p class="license-description">
                        Your 24-hour trial has expired. Enter a license key to continue using Helix Desktop.
                    </p>
                    <div class="form-group" style="margin-top: 12px;">
                        <label class="form-label" for="licenseKeyInput">License Key</label>
                        <input class="form-input mono" id="licenseKeyInput" type="text" placeholder="Paste your license key here...">
                    </div>
                    <div class="btn-group" style="margin-top: 8px;">
                        <button class="btn btn-primary" id="activateLicenseBtn">Activate License</button>
                    </div>
                    <div class="license-links">
                        <a href="#" id="getLicenseLink" class="license-link">Get a license at deploy.helix.ml</a>
                    </div>
                    <div id="licenseError" class="error-msg" style="display: none; margin-top: 8px;"></div>
                </div>
            </div>
        `;
    }

    // no_trial — show trial start button
    return `
        <div class="card license-card">
            <div class="card-header">
                <h2>Get Started</h2>
            </div>
            <div class="card-body">
                <p class="license-description">
                    Start a free 24-hour trial to try Helix Desktop, or enter a license key.
                </p>
                <div class="btn-group" style="margin-top: 12px;">
                    <button class="btn btn-primary" id="startTrialBtn">Start 24-Hour Free Trial</button>
                </div>
                <div style="margin-top: 16px; border-top: 1px solid var(--border); padding-top: 16px;">
                    <div class="form-group">
                        <label class="form-label" for="licenseKeyInput">Or enter a license key</label>
                        <input class="form-input mono" id="licenseKeyInput" type="text" placeholder="Paste your license key here...">
                    </div>
                    <div class="btn-group" style="margin-top: 8px;">
                        <button class="btn btn-secondary" id="activateLicenseBtn">Activate License</button>
                    </div>
                </div>
                <div id="licenseError" class="error-msg" style="display: none; margin-top: 8px;"></div>
            </div>
        </div>
    `;
}

function getTrialRemaining(endsAt) {
    if (!endsAt) return '--';
    const end = new Date(endsAt);
    const now = new Date();
    const diff = end - now;
    if (diff <= 0) return 'Expired';
    const hours = Math.floor(diff / (1000 * 60 * 60));
    const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));
    if (hours > 0) return `${hours}h ${minutes}m`;
    return `${minutes}m`;
}

function attachVMHandlers() {
    const startBtn = document.getElementById('startVMBtn');
    const stopBtn = document.getElementById('stopVMBtn');
    const openUIBtn = document.getElementById('openUIBtn');
    const copySSHBtn = document.getElementById('copySSHBtn');
    const copyAPIBtn = document.getElementById('copyAPIBtn');

    // Download handlers
    const startDownloadBtn = document.getElementById('startDownloadBtn');
    const retryDownloadBtn = document.getElementById('retryDownloadBtn');
    const cancelDownloadBtn = document.getElementById('cancelDownloadBtn');

    // License handlers
    const startTrialBtn = document.getElementById('startTrialBtn');
    const activateLicenseBtn = document.getElementById('activateLicenseBtn');
    const getLicenseLink = document.getElementById('getLicenseLink');

    if (startBtn) {
        startBtn.addEventListener('click', async () => {
            startBtn.disabled = true;
            startBtn.textContent = 'Starting...';
            try { await StartVM(); } catch (err) { console.error('Start VM failed:', err); }
        });
    }

    if (stopBtn) {
        stopBtn.addEventListener('click', async () => {
            stopBtn.disabled = true;
            stopBtn.textContent = 'Stopping...';
            try { await StopVM(); } catch (err) { console.error('Stop VM failed:', err); }
        });
    }

    if (openUIBtn) {
        openUIBtn.addEventListener('click', () => OpenHelixUI());
    }

    if (copySSHBtn) {
        copySSHBtn.addEventListener('click', () => {
            const port = state.vmConfig.ssh_port || 2222;
            navigator.clipboard.writeText(`ssh -p ${port} helix@localhost`);
            showToast('SSH command copied');
        });
    }

    if (copyAPIBtn) {
        copyAPIBtn.addEventListener('click', () => {
            const port = state.vmConfig.api_port || 8080;
            navigator.clipboard.writeText(`http://localhost:${port}`);
            showToast('API endpoint copied');
        });
    }

    // Download buttons
    if (startDownloadBtn) {
        startDownloadBtn.addEventListener('click', async () => {
            startDownloadBtn.disabled = true;
            startDownloadBtn.textContent = 'Starting download...';
            try {
                await DownloadVMImages();
            } catch (err) {
                console.error('Download failed:', err);
                showToast('Failed to start download');
            }
        });
    }

    if (retryDownloadBtn) {
        retryDownloadBtn.addEventListener('click', async () => {
            state.downloadProgress = null;
            renderContent();
            try {
                await DownloadVMImages();
            } catch (err) {
                console.error('Retry download failed:', err);
            }
        });
    }

    if (cancelDownloadBtn) {
        cancelDownloadBtn.addEventListener('click', () => {
            CancelDownload();
            state.downloadProgress = null;
            renderContent();
        });
    }

    // License buttons
    if (startTrialBtn) {
        startTrialBtn.addEventListener('click', async () => {
            startTrialBtn.disabled = true;
            startTrialBtn.textContent = 'Starting trial...';
            try {
                await StartTrial();
                state.licenseStatus = await GetLicenseStatus();
                renderContent();
                showToast('24-hour trial started');
            } catch (err) {
                console.error('Start trial failed:', err);
                showToast('Failed to start trial');
            }
        });
    }

    if (activateLicenseBtn) {
        activateLicenseBtn.addEventListener('click', async () => {
            const input = document.getElementById('licenseKeyInput');
            const errorEl = document.getElementById('licenseError');
            if (!input || !input.value.trim()) {
                if (errorEl) {
                    errorEl.textContent = 'Please enter a license key';
                    errorEl.style.display = 'block';
                }
                return;
            }

            activateLicenseBtn.disabled = true;
            activateLicenseBtn.textContent = 'Validating...';

            try {
                await ValidateLicenseKey(input.value.trim());
                state.licenseStatus = await GetLicenseStatus();
                renderContent();
                showToast('License activated');
            } catch (err) {
                if (errorEl) {
                    errorEl.textContent = err.toString().replace('Error: ', '');
                    errorEl.style.display = 'block';
                }
                activateLicenseBtn.disabled = false;
                activateLicenseBtn.textContent = 'Activate License';
            }
        });
    }

    if (getLicenseLink) {
        getLicenseLink.addEventListener('click', (e) => {
            e.preventDefault();
            // Use Wails to open in default browser
            window.open('https://deploy.helix.ml/licenses/new', '_blank');
        });
    }
}

// ---- Scanout Display Card ----

function renderScanoutCard() {
    const sc = state.scanoutStats;
    const hasData = sc.total_connectors > 0;
    const active = sc.active_displays || 0;
    const max = sc.max_scanouts || 15;
    const usagePct = Math.round((active / max) * 100);

    if (!hasData && state.vmStatus.state !== 'running') {
        return '';
    }

    return `
        <div class="card">
            <div class="card-header">
                <h2>Displays</h2>
                <span class="card-badge" style="color: var(--text-faded);">DRM Scanout</span>
            </div>
            <div class="card-body">
                ${hasData ? `
                    <div class="scanout-usage">
                        <div class="scanout-count">
                            <span class="scanout-active teal">${active}</span>
                            <span class="scanout-sep">/</span>
                            <span class="scanout-max">${max}</span>
                        </div>
                        <div class="scanout-label">displays in use</div>
                    </div>
                    <div class="progress-bar" style="margin: 12px 0;">
                        <div class="progress-fill ${usagePct > 90 ? 'warning' : 'teal'}" style="width: ${usagePct}%;"></div>
                    </div>
                    ${sc.displays && sc.displays.length > 0 ? `
                        <div class="display-list">
                            ${sc.displays.filter(d => d.connected).map(d => `
                                <div class="display-item">
                                    <span class="status-indicator running"></span>
                                    <span class="display-name">${d.name}</span>
                                    ${d.width > 0 ? `<span class="display-res">${d.width}x${d.height}</span>` : ''}
                                </div>
                            `).join('')}
                        </div>
                    ` : ''}
                ` : `
                    <div class="empty-state">
                        ${sc.error ? `Unable to fetch display stats: ${sc.error}` : 'Loading display stats...'}
                    </div>
                `}
            </div>
        </div>
    `;
}

// ---- Storage View ----

function renderStorageView() {
    const zfs = state.zfsStats;
    const disk = state.diskUsage;
    const hasData = zfs.pool_size > 0;

    const poolPct = hasData ? Math.round((zfs.pool_used / zfs.pool_size) * 100) : 0;
    const rootPct = disk.root_disk_total > 0 ? Math.round((disk.root_disk_used / disk.root_disk_total) * 100) : 0;

    return `
        <div class="view-container">
            <div class="view-header">
                <h1>Storage</h1>
                <p>ZFS deduplication and disk usage</p>
            </div>

            ${hasData ? `
                <div class="dedup-highlight">
                    <div>
                        <div class="dedup-value">${(zfs.dedup_ratio || 1).toFixed(2)}x</div>
                        <div class="dedup-label">Dedup Ratio</div>
                    </div>
                    <div style="margin-left: 24px;">
                        <div class="dedup-value" style="font-size: 22px;">${(zfs.compression_ratio || 1).toFixed(2)}x</div>
                        <div class="dedup-label">Compression</div>
                    </div>
                </div>
            ` : ''}

            <div class="card">
                <div class="card-header">
                    <h2>ZFS Pool</h2>
                    <span class="card-badge" style="color: var(--text-faded);">${zfs.pool_name || 'helix'}</span>
                </div>
                <div class="card-body">
                    ${hasData ? `
                        <div class="progress-bar" style="margin-bottom: 16px;">
                            <div class="progress-fill ${poolPct > 90 ? 'error' : poolPct > 75 ? 'warning' : 'teal'}" style="width: ${poolPct}%;"></div>
                        </div>
                        <div class="stats-grid">
                            <div class="stat-item">
                                <div class="stat-label">Pool Size</div>
                                <div class="stat-value small">${formatBytes(zfs.pool_size)}</div>
                            </div>
                            <div class="stat-item">
                                <div class="stat-label">Used</div>
                                <div class="stat-value small">${formatBytes(zfs.pool_used)}</div>
                            </div>
                            <div class="stat-item">
                                <div class="stat-label">Available</div>
                                <div class="stat-value small success">${formatBytes(zfs.pool_available)}</div>
                            </div>
                            <div class="stat-item">
                                <div class="stat-label">Usage</div>
                                <div class="stat-value small ${poolPct > 90 ? 'warning' : ''}">${poolPct}%</div>
                            </div>
                        </div>
                    ` : `
                        <div class="empty-state">
                            ${state.vmStatus.state !== 'running' ?
                                'Start the VM to view ZFS storage stats.' :
                                zfs.error ? `Unable to fetch ZFS stats: ${zfs.error}` : 'Loading ZFS stats...'
                            }
                        </div>
                    `}
                </div>
            </div>

            <div class="card">
                <div class="card-header">
                    <h2>Disk Usage</h2>
                </div>
                <div class="card-body">
                    ${disk.root_disk_total > 0 ? `
                        <div class="storage-breakdown">
                            <div class="storage-row">
                                <span class="label">Root Disk</span>
                                <span class="value">${formatBytes(disk.root_disk_used)} / ${formatBytes(disk.root_disk_total)}</span>
                            </div>
                            <div class="progress-bar">
                                <div class="progress-fill ${rootPct > 90 ? 'error' : rootPct > 75 ? 'warning' : 'teal'}" style="width: ${rootPct}%;"></div>
                            </div>
                            ${disk.host_actual > 0 ? `
                                <div class="storage-row" style="margin-top: 8px;">
                                    <span class="label">Host Actual (after dedup)</span>
                                    <span class="value">${formatBytes(disk.host_actual)}</span>
                                </div>
                            ` : ''}
                        </div>
                    ` : `
                        <div class="empty-state">
                            ${state.vmStatus.state !== 'running' ? 'Start the VM to view disk usage.' : 'Loading disk usage...'}
                        </div>
                    `}
                </div>
            </div>

            ${zfs.last_updated ? `
                <div style="text-align: right; font-size: 11px; color: var(--text-faded); margin-top: 8px;">
                    Last updated: ${new Date(zfs.last_updated).toLocaleTimeString()}
                </div>
            ` : ''}
        </div>
    `;
}

// ---- Settings View ----

function renderSettingsView() {
    const s = state.settings;

    return `
        <div class="view-container">
            <div class="view-header">
                <h1>Settings</h1>
                <p>Configure VM and network settings</p>
            </div>

            <div class="card">
                <div class="card-header">
                    <h2>Virtual Machine</h2>
                </div>
                <div class="card-body">
                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label" for="settingCPUs">CPU Cores</label>
                            <input class="form-input" id="settingCPUs" type="text" value="${s.vm_cpus || 4}" placeholder="4">
                            <div class="form-hint">Number of vCPUs allocated to the VM</div>
                        </div>
                        <div class="form-group">
                            <label class="form-label" for="settingMemory">Memory (MB)</label>
                            <input class="form-input" id="settingMemory" type="text" value="${s.vm_memory_mb || 8192}" placeholder="8192">
                            <div class="form-hint">RAM in megabytes (e.g., 8192 = 8 GB)</div>
                        </div>
                    </div>
                    <div class="form-group">
                        <label class="form-label" for="settingDisk">VM Disk Path</label>
                        <input class="form-input mono" id="settingDisk" type="text" value="${s.vm_disk_path || ''}" placeholder="~/.helix/vm/helix-ubuntu.qcow2">
                    </div>
                </div>
            </div>

            <div class="card">
                <div class="card-header">
                    <h2>Network</h2>
                </div>
                <div class="card-body">
                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label" for="settingSSH">SSH Port</label>
                            <input class="form-input" id="settingSSH" type="text" value="${s.ssh_port || 2222}" placeholder="2222">
                        </div>
                        <div class="form-group">
                            <label class="form-label" for="settingAPI">API Port</label>
                            <input class="form-input" id="settingAPI" type="text" value="${s.api_port || 8080}" placeholder="8080">
                        </div>
                    </div>
                    <div class="form-group">
                        <label class="form-label" for="settingVideo">Video Port</label>
                        <input class="form-input" id="settingVideo" type="text" value="${s.video_port || 8765}" placeholder="8765">
                        <div class="form-hint">WebSocket port for video streaming</div>
                    </div>
                </div>
            </div>

            <div class="btn-group" style="margin-top: 8px;">
                <button class="btn btn-primary" id="saveSettingsBtn">Save Settings</button>
                <button class="btn btn-secondary" id="resetSettingsBtn">Reset to Defaults</button>
            </div>
        </div>
    `;
}

function attachSettingsHandlers() {
    const saveBtn = document.getElementById('saveSettingsBtn');
    const resetBtn = document.getElementById('resetSettingsBtn');

    if (saveBtn) {
        saveBtn.addEventListener('click', async () => {
            const settings = {
                vm_cpus: parseInt(document.getElementById('settingCPUs').value) || 4,
                vm_memory_mb: parseInt(document.getElementById('settingMemory').value) || 8192,
                vm_disk_path: document.getElementById('settingDisk').value,
                ssh_port: parseInt(document.getElementById('settingSSH').value) || 2222,
                api_port: parseInt(document.getElementById('settingAPI').value) || 8080,
                video_port: parseInt(document.getElementById('settingVideo').value) || 8765,
                auto_start_vm: false,
            };

            try {
                await SaveSettings(settings);
                state.settings = settings;
                showToast('Settings saved');
            } catch (err) {
                console.error('Failed to save settings:', err);
                showToast('Failed to save settings');
            }
        });
    }

    if (resetBtn) {
        resetBtn.addEventListener('click', async () => {
            try {
                state.settings = await GetSettings();
                renderContent();
                showToast('Settings reset');
            } catch (err) {
                console.error('Failed to reset settings:', err);
            }
        });
    }
}

// ---- Helpers ----

function formatUptime(seconds) {
    if (!seconds || seconds < 0) return '--';
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const s = Math.floor(seconds % 60);
    if (h > 0) return `${h}h ${m}m`;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
}

function formatNumber(num) {
    if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
    if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
    return num.toString();
}

function formatBytes(bytes) {
    if (!bytes || bytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    const val = bytes / Math.pow(1024, i);
    return `${val.toFixed(i > 1 ? 1 : 0)} ${units[i]}`;
}

function showToast(message) {
    // Remove existing toast
    const existing = document.querySelector('.toast');
    if (existing) existing.remove();

    const toast = document.createElement('div');
    toast.className = 'toast';
    toast.textContent = message;
    document.body.appendChild(toast);
    setTimeout(() => toast.remove(), 3000);
}

// ---- Start ----

init();
