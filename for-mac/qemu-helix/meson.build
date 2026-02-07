# Helix Frame Export for QEMU
#
# Add this to hw/display/meson.build in QEMU tree:
#   subdir('helix')
#
# Or copy files directly and add:
#   if host_machine.system() == 'darwin'
#     softmmu_ss.add(files('helix-frame-export.c'))
#     softmmu_ss.add(dependency('videotoolbox'))
#     softmmu_ss.add(dependency('corevideo'))
#     softmmu_ss.add(dependency('coremedia'))
#     softmmu_ss.add(dependency('iosurface'))
#   endif

helix_ss = ss.source_set()

if host_machine.system() == 'darwin'
  # Framework dependencies for macOS
  videotoolbox = dependency('VideoToolbox', required: true)
  corevideo = dependency('CoreVideo', required: true)
  coremedia = dependency('CoreMedia', required: true)
  iosurface = dependency('IOSurface', required: true)
  metal = dependency('Metal', required: true)
  foundation = dependency('Foundation', required: true)

  helix_ss.add(files('helix-frame-export.c'))
  helix_ss.add(videotoolbox)
  helix_ss.add(corevideo)
  helix_ss.add(coremedia)
  helix_ss.add(iosurface)
  helix_ss.add(metal)
  helix_ss.add(foundation)

  # Also need virglrenderer
  helix_ss.add(virgl)
endif

softmmu_ss.add_all(helix_ss)
