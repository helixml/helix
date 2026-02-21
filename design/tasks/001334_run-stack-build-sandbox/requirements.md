# Requirements: Run ./stack build-sandbox Timing

## Overview

Run the `./stack build-sandbox` command and capture timing results for performance benchmarking purposes. No code changes required.

## User Stories

### US1: Capture Build Timing
**As a** developer  
**I want to** run the sandbox build and see timing results  
**So that** I can understand the current build performance baseline

## Acceptance Criteria

- [ ] Run `./stack build-sandbox` to completion
- [ ] Capture and report the total build time
- [ ] Report timing for major build phases if available (Zed check, desktop builds, sandbox container, image transfer)
- [ ] No code changes made to the repository

## Out of Scope

- Modifying build scripts
- Optimizing build times
- Building experimental desktops (only production desktops: sway, ubuntu)