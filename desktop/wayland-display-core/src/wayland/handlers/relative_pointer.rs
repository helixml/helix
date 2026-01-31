use smithay::delegate_relative_pointer;
use smithay::wayland::output::OutputHandler;

use crate::comp::State;

impl OutputHandler for State {}

delegate_relative_pointer!(State);
