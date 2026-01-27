# Requirements: Install Go in Helix Startup Script

## Overview

Install Go toolchain in the Helix `./stack` startup script to ensure developers have the correct Go version available when working on the project.

## User Stories

1. **As a developer**, I want Go to be automatically installed when I run `./stack start`, so I don't have to manually set up the Go toolchain.

2. **As a developer**, I want the installed Go version to match what Helix uses (Go 1.25), so I avoid version mismatch issues.

## Acceptance Criteria

- [ ] Running `./stack start` ensures Go 1.25 is available in PATH
- [ ] If Go 1.25 is already installed, skip installation
- [ ] Installation works on Linux (primary platform)
- [ ] Clear error message if installation fails
- [ ] Go version matches `go.mod` requirement (1.25.0)