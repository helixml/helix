# Implementation Tasks

- [ ] In `.drone.yml`, replace the `build-sandbox-amd64` pipeline trigger (`event: [push, tag]`) with `ref: include: [refs/heads/main, refs/tags/*]`
- [ ] In `.drone.yml`, replace the `build-sandbox-arm64` pipeline trigger (`event: [push, tag]`) with `ref: include: [refs/heads/main, refs/tags/*]`
- [ ] Commit and push the change
