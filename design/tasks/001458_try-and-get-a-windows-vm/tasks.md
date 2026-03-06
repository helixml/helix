# Implementation Tasks

- [x] Install QEMU and dependencies (`qemu-system-x86`, `qemu-utils`, `ovmf`, `libvirglrenderer1`)
- [x] Fix `/dev/kvm` permissions (add user to kvm group or chmod)
- [x] Download VirtIO drivers ISO from Fedora
- [~] Download Windows 11 evaluation ISO from Microsoft
- [ ] Create 80GB qcow2 disk image
- [ ] Boot Windows installer with VirtIO-GPU and VirtIO disk
- [ ] Load VirtIO storage driver during Windows install
- [ ] Complete Windows 11 installation
- [ ] Install VirtIO GPU driver (viogpudo) in Windows Device Manager
- [ ] Verify GPU acceleration working (check Device Manager shows VirtIO GPU)
- [ ] Test display access (SDL with GL, or VNC fallback)
- [ ] Verify host GPU still accessible (`nvidia-smi` works)
- [ ] Document final working QEMU command line