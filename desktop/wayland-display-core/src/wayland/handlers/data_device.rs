use smithay::wayland::selection::SelectionHandler;
use smithay::{
    delegate_data_device,
    wayland::selection::data_device::{
        ClientDndGrabHandler, DataDeviceHandler, DataDeviceState, ServerDndGrabHandler,
    },
};

use crate::comp::State;

impl ServerDndGrabHandler for State {}

impl ClientDndGrabHandler for State {}

impl SelectionHandler for State {
    type SelectionUserData = ();
}

impl DataDeviceHandler for State {
    fn data_device_state(&self) -> &DataDeviceState {
        &self.data_device_state
    }
}

delegate_data_device!(State);
