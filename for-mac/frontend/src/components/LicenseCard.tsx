import { main } from '../../wailsjs/go/models';
import { StartTrial, ValidateLicenseKey, GetLicenseStatus } from '../../wailsjs/go/main/App';
import { getTrialRemaining } from '../lib/helpers';
import { useState } from 'react';

interface LicenseCardProps {
  licenseStatus: main.LicenseStatus;
  onLicenseUpdated: (status: main.LicenseStatus) => void;
  showToast: (msg: string) => void;
}

export function LicenseCard({ licenseStatus: ls, onLicenseUpdated, showToast }: LicenseCardProps) {
  const [licenseKey, setLicenseKey] = useState('');
  const [licenseError, setLicenseError] = useState('');
  const [activating, setActivating] = useState(false);
  const [startingTrial, setStartingTrial] = useState(false);

  async function handleActivateLicense() {
    if (!licenseKey.trim()) {
      setLicenseError('Please enter a license key');
      return;
    }
    setActivating(true);
    setLicenseError('');
    try {
      await ValidateLicenseKey(licenseKey.trim());
      const status = await GetLicenseStatus();
      onLicenseUpdated(status);
      showToast('License activated');
    } catch (err) {
      setLicenseError(String(err).replace('Error: ', ''));
      setActivating(false);
    }
  }

  async function handleStartTrial() {
    setStartingTrial(true);
    try {
      await StartTrial();
      const status = await GetLicenseStatus();
      onLicenseUpdated(status);
      showToast('24-hour trial started');
    } catch (err) {
      console.error('Start trial failed:', err);
      showToast('Failed to start trial');
      setStartingTrial(false);
    }
  }

  if (ls.state === 'licensed') {
    return (
      <div className="license-badge licensed">
        <span className="license-badge-icon">&#10003;</span>
        Licensed to: {ls.licensed_to || 'Unknown'}
        {ls.expires_at && (
          <span className="license-expires">
            (expires {new Date(ls.expires_at).toLocaleDateString()})
          </span>
        )}
      </div>
    );
  }

  if (ls.state === 'trial_active') {
    const remaining = getTrialRemaining(ls.trial_ends_at);
    return (
      <div className="license-badge trial-active">
        <span className="license-badge-icon">&#9201;</span>
        Trial: {remaining} remaining
      </div>
    );
  }

  if (ls.state === 'trial_expired') {
    return (
      <div className="card license-card">
        <div className="card-header">
          <h2>Trial Expired</h2>
        </div>
        <div className="card-body">
          <p className="license-description">
            Your 24-hour trial has expired. Enter a license key to continue using Helix Desktop.
          </p>
          <div className="form-group" style={{ marginTop: 12 }}>
            <label className="form-label" htmlFor="licenseKeyInput">
              License Key
            </label>
            <input
              className="form-input mono"
              id="licenseKeyInput"
              type="text"
              placeholder="Paste your license key here..."
              value={licenseKey}
              onChange={(e) => setLicenseKey(e.target.value)}
            />
          </div>
          <div className="btn-group" style={{ marginTop: 8 }}>
            <button
              className="btn btn-primary"
              onClick={handleActivateLicense}
              disabled={activating}
            >
              {activating ? 'Validating...' : 'Activate License'}
            </button>
          </div>
          <div className="license-links">
            <a
              href="#"
              className="license-link"
              onClick={(e) => {
                e.preventDefault();
                window.open('https://helix.ml/licenses', '_blank');
              }}
            >
              Get a license at helix.ml
            </a>
          </div>
          {licenseError && (
            <div className="error-msg" style={{ marginTop: 8 }}>
              {licenseError}
            </div>
          )}
        </div>
      </div>
    );
  }

  // no_trial — show trial start + license key entry
  return (
    <div className="card license-card">
      <div className="card-header">
        <h2>Get Started</h2>
      </div>
      <div className="card-body">
        <p className="license-description">
          Enter your license key to activate Helix Desktop, or start a free 24-hour trial.
        </p>
        <div className="form-group" style={{ marginTop: 16 }}>
          <label className="form-label" htmlFor="licenseKeyInput">
            License Key
          </label>
          <input
            className="form-input mono"
            id="licenseKeyInput"
            type="text"
            placeholder="Paste your license key here..."
            value={licenseKey}
            onChange={(e) => setLicenseKey(e.target.value)}
          />
        </div>
        <div className="btn-group" style={{ marginTop: 8 }}>
          <button
            className="btn btn-primary"
            onClick={handleActivateLicense}
            disabled={activating}
          >
            {activating ? 'Validating...' : 'Activate License'}
          </button>
          <a
            href="#"
            className="btn btn-secondary"
            onClick={(e) => {
              e.preventDefault();
              window.open('https://helix.ml/licenses', '_blank');
            }}
          >
            Buy a License
          </a>
        </div>
        <div
          style={{
            marginTop: 16,
            borderTop: '1px solid var(--border)',
            paddingTop: 16,
          }}
        >
          <p className="license-description">No license key? Try it free for 24 hours.</p>
          <div className="btn-group" style={{ marginTop: 8 }}>
            <button
              className="btn btn-secondary"
              onClick={handleStartTrial}
              disabled={startingTrial}
            >
              {startingTrial ? 'Starting trial...' : 'Start 24-Hour Free Trial'}
            </button>
          </div>
        </div>
        {licenseError && (
          <div className="error-msg" style={{ marginTop: 8 }}>
            {licenseError}
          </div>
        )}
      </div>
    </div>
  );
}
