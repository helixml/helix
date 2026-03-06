# Implementation Tasks

## Completed
- [x] Install QEMU and dependencies (`qemu-system-x86`, `qemu-utils`, `ovmf`, `libvirglrenderer1`)
- [x] Fix `/dev/kvm` permissions (chmod 666 /dev/kvm)
- [x] Download VirtIO drivers ISO from Fedora (753MB)
- [x] Download Windows 11 evaluation ISO from Microsoft (5.8GB)
- [x] Create 80GB qcow2 disk image
- [x] Install swtpm for TPM 2.0 emulation
- [x] Boot Windows installer - reached setup screen

## In Progress
- [~] Download Microsoft Hyper-V pre-built VM (21GB) - converting to qcow2 avoids install hassle
- [~] Windows 11 requirement bypass (TPM/SecureBoot check failing)

## Remaining
- [ ] Convert VHDX to qcow2 (`qemu-img convert -f vhdx -O qcow2`)
- [ ] Boot pre-built Windows VM with TPM + virtio-gpu
- [ ] Install VirtIO GPU driver in Windows (viogpudo)
- [ ] Verify GPU acceleration working
- [ ] Verify host GPU still accessible (`nvidia-smi`)
- [ ] Document final working QEMU command line

## Key Learnings

1. **Windows 11 requires TPM 2.0 + Secure Boot** - use `swtpm` for TPM emulation and OVMF secboot firmware
2. **Pre-built images are faster** - Microsoft's Hyper-V eval VM (WinDev2407Eval) can be converted to qcow2
3. **Boot timing is tricky** - need to spam keys early to catch "Press any key to boot from CD"
4. **Avoid sketchy qcow2 downloads** - Google Drive links claiming pre-built Windows images are malware

## Working QEMU Command (with TPM)

```bash
# Start swtpm first
swtpm socket --tpmstate dir=/tmp/windows-vm/tpm \
  --ctrl type=unixio,path=/tmp/windows-vm/tpm/swtpm-sock \
  --tpm2 --daemon

# Then QEMU with TPM + Secure Boot
qemu-system-x86_64 \
  -enable-kvm -m 16G -smp 8 -cpu host \
  -machine q35,accel=kvm,smm=on \
  -global driver=cfi.pflash01,property=secure,value=on \
  -device virtio-vga \
  -drive file=windows11.qcow2,if=virtio,format=qcow2 \
  -drive if=pflash,format=raw,readonly=on,file=/usr/share/OVMF/OVMF_CODE_4M.secboot.fd \
  -drive if=pflash,format=raw,file=OVMF_VARS_SECBOOT.fd \
  -chardev socket,id=chrtpm,path=/tmp/windows-vm/tpm/swtpm-sock \
  -tpmdev emulator,id=tpm0,chardev=chrtpm \
  -device tpm-tis,tpmdev=tpm0 \
  -device virtio-net,netdev=net0 \
  -netdev user,id=net0 \
  -usb -device usb-tablet \
  -vnc :0
```
