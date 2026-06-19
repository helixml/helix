# Requirements: WidgetSync — Sync Widget Configurations Between Devices over WiFi

## Overview

Users can configure UI widgets (dashboard layouts, view modes, table settings) on one device and have those configurations automatically sync to other devices on the same WiFi network.

## User Stories

**US-1: Automatic sync on same network**
As a user, when I adjust a widget's configuration on my desktop, it should appear on my laptop within seconds if both are on the same WiFi network.

**US-2: Manual sync trigger**
As a user, I want a "Sync now" button so I can force an immediate sync without waiting for the automatic interval.

**US-3: Conflict resolution**
As a user, if I changed a widget on two devices while offline, I want the most-recently-modified version to win, and I want to be notified of the merge.

**US-4: Selective sync**
As a user, I want to choose which widget types to sync (e.g. view modes but not table column widths) in Settings.

**US-5: Sync status visibility**
As a user, I want to see the last-synced timestamp and whether sync is active or paused.

## Acceptance Criteria

- AC-1: Widget config changes propagate to other devices within 5 seconds when both are online and on the same local network.
- AC-2: Sync works over mDNS/local discovery — no cloud relay required.
- AC-3: Last-write-wins conflict resolution is applied automatically; a toast notification informs the user when a conflict was resolved.
- AC-4: Sync can be disabled per widget type from Settings → WidgetSync.
- AC-5: The sync status indicator is visible in the Settings page and shows: Active / Paused / Last synced at `<timestamp>`.
- AC-6: No data leaves the local network; all sync traffic is device-to-device over the LAN.

## Out of Scope

- Cloud-based sync (future work).
- Syncing between devices on different networks (VPN, etc.).
- Real-time collaborative editing of the same widget simultaneously.
