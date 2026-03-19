# Implementation Tasks

- [x] In `.drone.yml`, replace the `build-sandbox-amd64` pipeline trigger (`event: [push, tag]`) with `ref: include: [refs/heads/main, refs/tags/*]`
- [x] In `.drone.yml`, replace the `build-sandbox-arm64` pipeline trigger (`event: [push, tag]`) with `ref: include: [refs/heads/main, refs/tags/*]`
- [x] Commit and push the change
