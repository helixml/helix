import './style.css';
import {
    GetVMStatus,
    GetVMConfig,
    StartVM,
    StopVM,
    GetEncoderStats,
    GetClientCount,
    CheckDependencies,
    IsVMImageReady,
    OpenHelixUI,
    GetHelixURL,
    GetSystemInfo
} from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

// App State
let state = {
    vmStatus: { state: 'stopped' },
    vmConfig: {},
    encoderStats: {},
    clientCount: 0,
    dependencies: {},
    vmImageReady: false,
    systemInfo: {}
};

// Initialize app
async function init() {
    console.log('Helix for Mac initializing...');

    // Load initial state
    try {
        state.dependencies = await CheckDependencies();
        state.vmImageReady = await IsVMImageReady();
        state.systemInfo = await GetSystemInfo();
        state.vmStatus = await GetVMStatus();
        state.vmConfig = await GetVMConfig();
    } catch (err) {
        console.error('Failed to load initial state:', err);
    }

    // Subscribe to VM status updates
    EventsOn('vm:status', (status) => {
        state.vmStatus = status;
        render();
    });

    // Poll for stats
    setInterval(async () => {
        if (state.vmStatus.state === 'running') {
            try {
                state.encoderStats = await GetEncoderStats();
                state.clientCount = await GetClientCount();
                state.vmStatus = await GetVMStatus();
                render();
            } catch (err) {
                console.error('Failed to poll stats:', err);
            }
        }
    }, 2000);

    render();
}

// Render the UI
function render() {
    const app = document.getElementById('app');

    // Check if setup is needed
    const needsSetup = !state.vmImageReady || !state.dependencies.qemu;

    if (needsSetup) {
        app.innerHTML = renderSetupScreen();
    } else {
        app.innerHTML = renderDashboard();
    }

    // Attach event handlers
    attachEventHandlers();
}

// Render setup screen
function renderSetupScreen() {
    const deps = state.dependencies;

    return `
        <div class="setup-screen">
            <h1>🚀 Helix for Mac</h1>
            <p>GPU-accelerated AI development environment for macOS</p>

            <div class="deps-list">
                <div class="dep-item">
                    <span class="dep-name">QEMU/UTM</span>
                    <span class="dep-status">${deps.qemu ? '✅' : '❌'}</span>
                </div>
                <div class="dep-item">
                    <span class="dep-name">GStreamer</span>
                    <span class="dep-status">${deps.gstreamer ? '✅' : '❌'}</span>
                </div>
                <div class="dep-item">
                    <span class="dep-name">VideoToolbox (H.264)</span>
                    <span class="dep-status">${deps.videotoolbox ? '✅' : '❌'}</span>
                </div>
                <div class="dep-item">
                    <span class="dep-name">VM Image</span>
                    <span class="dep-status">${state.vmImageReady ? '✅' : '❌'}</span>
                </div>
            </div>

            ${!state.vmImageReady ? `
                <p style="color: var(--warning); margin-bottom: 20px;">
                    VM image not found. You'll need to create or download one.
                </p>
                <button class="btn btn-primary" onclick="window.setupVM()">
                    Download VM Image
                </button>
            ` : ''}

            ${!deps.qemu ? `
                <p style="color: var(--error); margin-bottom: 20px;">
                    QEMU not found. Please install UTM from the App Store or run:<br>
                    <code style="background: var(--bg-card); padding: 4px 8px; border-radius: 4px;">brew install qemu</code>
                </p>
            ` : ''}

            <div style="margin-top: 24px; font-size: 12px; color: var(--text-secondary);">
                Platform: ${state.systemInfo.platform || 'unknown'}
            </div>
        </div>
    `;
}

// Render main dashboard
function renderDashboard() {
    const status = state.vmStatus;
    const config = state.vmConfig;
    const stats = state.encoderStats;

    return `
        <div class="header">
            <h1>
                <span style="font-size: 24px;">⬡</span>
                Helix for Mac
            </h1>
            <div class="header-actions">
                <span class="status-badge ${status.state}">
                    <span class="status-dot ${status.state === 'starting' || status.state === 'stopping' ? 'pulse' : ''}"></span>
                    ${status.state.charAt(0).toUpperCase() + status.state.slice(1)}
                </span>
            </div>
        </div>

        <div class="main">
            <div class="card">
                <div class="card-header">
                    <h2>VM Status</h2>
                    ${status.state === 'stopped' ?
                        `<button class="btn btn-success btn-sm" id="startBtn">Start VM</button>` :
                        status.state === 'running' ?
                        `<button class="btn btn-danger btn-sm" id="stopBtn">Stop VM</button>` :
                        `<button class="btn btn-secondary btn-sm" disabled>${status.state}...</button>`
                    }
                </div>
                <div class="card-body">
                    <div class="stats-grid">
                        <div class="stat-item">
                            <div class="stat-label">CPU Cores</div>
                            <div class="stat-value">${config.cpus || 2}</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-label">Memory</div>
                            <div class="stat-value">${((config.memory_mb || 4096) / 1024).toFixed(0)} GB</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-label">Uptime</div>
                            <div class="stat-value">${formatUptime(status.uptime || 0)}</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-label">Sessions</div>
                            <div class="stat-value">${status.sessions || 0}</div>
                        </div>
                    </div>
                    ${status.error_msg ? `
                        <div style="margin-top: 16px; padding: 12px; background: rgba(239,68,68,0.1); border-radius: 8px; color: var(--error); font-size: 13px;">
                            ${status.error_msg}
                        </div>
                    ` : ''}
                </div>
            </div>

            <div class="card">
                <div class="card-header">
                    <h2>Video Encoding</h2>
                    <span style="font-size: 12px; color: var(--text-secondary);">VideoToolbox H.264</span>
                </div>
                <div class="card-body">
                    <div class="stats-grid">
                        <div class="stat-item">
                            <div class="stat-label">Current FPS</div>
                            <div class="stat-value">${(stats.current_fps || 0).toFixed(1)}</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-label">Clients</div>
                            <div class="stat-value">${state.clientCount}</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-label">Frames Encoded</div>
                            <div class="stat-value" style="font-size: 16px;">${formatNumber(stats.frames_encoded || 0)}</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-label">Frames Dropped</div>
                            <div class="stat-value" style="font-size: 16px;">${stats.frames_dropped || 0}</div>
                        </div>
                    </div>
                </div>
            </div>

            <div class="card" style="grid-column: span 2;">
                <div class="card-header">
                    <h2>Quick Actions</h2>
                </div>
                <div class="card-body">
                    <div class="quick-actions">
                        <div class="action-item">
                            <div class="action-info">
                                <h3>Open Helix UI</h3>
                                <p>Launch the Helix web interface in your browser</p>
                            </div>
                            <button class="btn btn-primary btn-sm" id="openUIBtn" ${status.state !== 'running' || !status.api_ready ? 'disabled' : ''}>
                                Open
                            </button>
                        </div>
                        <div class="action-item">
                            <div class="action-info">
                                <h3>SSH to VM</h3>
                                <p>ssh -p ${config.ssh_port || 2222} helix@localhost</p>
                            </div>
                            <button class="btn btn-secondary btn-sm" onclick="navigator.clipboard.writeText('ssh -p ${config.ssh_port || 2222} helix@localhost')">
                                Copy
                            </button>
                        </div>
                        <div class="action-item">
                            <div class="action-info">
                                <h3>API Endpoint</h3>
                                <p>http://localhost:${config.api_port || 8080}</p>
                            </div>
                            <button class="btn btn-secondary btn-sm" onclick="navigator.clipboard.writeText('http://localhost:${config.api_port || 8080}')">
                                Copy
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    `;
}

// Attach event handlers
function attachEventHandlers() {
    const startBtn = document.getElementById('startBtn');
    const stopBtn = document.getElementById('stopBtn');
    const openUIBtn = document.getElementById('openUIBtn');

    if (startBtn) {
        startBtn.addEventListener('click', async () => {
            try {
                await StartVM();
            } catch (err) {
                console.error('Failed to start VM:', err);
            }
        });
    }

    if (stopBtn) {
        stopBtn.addEventListener('click', async () => {
            try {
                await StopVM();
            } catch (err) {
                console.error('Failed to stop VM:', err);
            }
        });
    }

    if (openUIBtn) {
        openUIBtn.addEventListener('click', async () => {
            try {
                await OpenHelixUI();
            } catch (err) {
                console.error('Failed to open UI:', err);
            }
        });
    }
}

// Helper functions
function formatUptime(seconds) {
    if (!seconds || seconds < 0) return '0s';
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

// Global functions for inline handlers
window.setupVM = async function() {
    alert('VM setup not yet implemented.\n\nPlease create a VM image manually at:\n~/.helix/vm/helix-ubuntu.qcow2');
};

// Start the app
init();
