# Requirements: Fix VM Boot Hang on Helix Mac App

## Problem Statement

When users download the Helix Mac app, the startup screen sometimes gets permanently stuck at "Booting VM...". The VM never advances past the first boot stage, leaving the user unable to use Helix.

Logs from `qemu-helix-realize.log` show `virtio_gpu_device_realize` being called 3 times (once per QEMU/UEFI/kernel init phase), each completing successfully. The hang occurs *after* realize completes, during the Linux guest GPU driver initialization phase.

## User Stories

**US-1**: As a Mac user who just downloaded Helix, I want VM startup to complete reliably so I can use the app without needing to kill and restart it.

**US-2**: As a Mac user, I want the "Booting VM..." stage to either complete or show a clear error, not hang indefinitely.

## Acceptance Criteria

- AC-1: VM boot completes successfully (reaches SSH ready, then service ready stages) on 100% of cold starts in test environments, vs the current ~occasional failure rate.
- AC-2: If boot does fail, an error is surfaced within the existing timeout (10 min) rather than hanging silently.
- AC-3: No regression in boot speed or GPU performance.
- AC-4: The `virtio_gpu_device_realize` being called 3 times during startup is handled gracefully (this is normal UEFI → kernel init behavior).
