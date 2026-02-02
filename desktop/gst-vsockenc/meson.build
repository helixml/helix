project('gst-vsockenc', 'c',
  version : '1.0.0',
  license : 'LGPL-2.1-or-later',
  meson_version : '>= 0.60',
  default_options : ['warning_level=2', 'c_std=gnu11']
)

gst_dep = dependency('gstreamer-1.0', version : '>=1.20')
gst_base_dep = dependency('gstreamer-base-1.0', version : '>=1.20')
gst_video_dep = dependency('gstreamer-video-1.0', version : '>=1.20')
gst_allocators_dep = dependency('gstreamer-allocators-1.0', version : '>=1.20')
libdrm_dep = dependency('libdrm')

vsockenc_sources = [
  'gstvsockenc.c',
]

vsockenc_deps = [
  gst_dep,
  gst_base_dep,
  gst_video_dep,
  gst_allocators_dep,
  libdrm_dep,
]

gstvsockenc = shared_library('gstvsockenc',
  vsockenc_sources,
  dependencies : vsockenc_deps,
  install : true,
  install_dir : get_option('libdir') / 'gstreamer-1.0',
)

# For use in the parent desktop build
gstvsockenc_dep = declare_dependency(
  link_with : gstvsockenc,
  include_directories : include_directories('.'),
)
