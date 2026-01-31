use crate::{
    comp::State,
    wayland::protocols::wl_drm::{DrmHandler, ImportError, delegate_wl_drm},
};
use smithay::backend::renderer::ImportDma;
use smithay::{
    backend::allocator::dmabuf::Dmabuf, reexports::wayland_server::protocol::wl_buffer::WlBuffer,
    wayland::dmabuf::DmabufGlobal,
};

impl DrmHandler<()> for State {
    fn dmabuf_imported(
        &mut self,
        _global: &DmabufGlobal,
        dmabuf: Dmabuf,
    ) -> Result<(), ImportError> {
        self.renderer
            .import_dmabuf(&dmabuf, None)
            .map(|_| ())
            .map_err(|_| ImportError::Failed)
    }

    fn buffer_created(&mut self, _buffer: WlBuffer, _result: ()) {}
}

delegate_wl_drm!(State);
