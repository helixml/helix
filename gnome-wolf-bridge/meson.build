project('gnome-wolf-bridge', 'c',
  version: '0.1.0',
  default_options: [
    'warning_level=2',
    'c_std=c11',
  ],
)

# Dependencies
wayland_client = dependency('wayland-client')
wayland_protocols = dependency('wayland-protocols', version: '>= 1.25')
pipewire = dependency('libpipewire-0.3', version: '>= 0.3.0')
spa = dependency('libspa-0.2')
gio = dependency('gio-2.0')
gio_unix = dependency('gio-unix-2.0')
libdrm = dependency('libdrm')
libei = dependency('libei-1.0', required: false)

# Wayland protocol scanner
wayland_scanner = find_program('wayland-scanner')

# Protocol directory
wl_protocol_dir = wayland_protocols.get_variable('pkgdatadir')

# Generate xdg-shell protocol
xdg_shell_xml = wl_protocol_dir / 'stable/xdg-shell/xdg-shell.xml'
xdg_shell_c = custom_target('xdg-shell-protocol.c',
  input: xdg_shell_xml,
  output: 'xdg-shell-protocol.c',
  command: [wayland_scanner, 'private-code', '@INPUT@', '@OUTPUT@'],
)
xdg_shell_h = custom_target('xdg-shell-client-protocol.h',
  input: xdg_shell_xml,
  output: 'xdg-shell-client-protocol.h',
  command: [wayland_scanner, 'client-header', '@INPUT@', '@OUTPUT@'],
)

# Generate linux-dmabuf protocol
dmabuf_xml = wl_protocol_dir / 'unstable/linux-dmabuf/linux-dmabuf-unstable-v1.xml'
dmabuf_c = custom_target('linux-dmabuf-protocol.c',
  input: dmabuf_xml,
  output: 'linux-dmabuf-unstable-v1-protocol.c',
  command: [wayland_scanner, 'private-code', '@INPUT@', '@OUTPUT@'],
)
dmabuf_h = custom_target('linux-dmabuf-client-protocol.h',
  input: dmabuf_xml,
  output: 'linux-dmabuf-unstable-v1-client-protocol.h',
  command: [wayland_scanner, 'client-header', '@INPUT@', '@OUTPUT@'],
)

sources = files(
  'main.c',
  'wayland-client.c',
  'screencast.c',
  'portal-screencast.c',
  'pipewire-stream.c',
)

if libei.found()
  sources += files('eis-input.c')
  add_project_arguments('-DHAVE_LIBEI', language: 'c')
endif

deps = [
  wayland_client,
  pipewire,
  spa,
  gio,
  gio_unix,
  libdrm,
]

if libei.found()
  deps += libei
endif

executable('gnome-wolf-bridge',
  sources,
  xdg_shell_c, xdg_shell_h,
  dmabuf_c, dmabuf_h,
  dependencies: deps,
  install: true,
)
