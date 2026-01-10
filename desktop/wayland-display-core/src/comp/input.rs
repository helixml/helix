use super::{State, focus::FocusTarget};
use smithay::backend::input::Keycode;
use smithay::backend::libinput::LibinputInputBackend;
use smithay::input::keyboard::Keysym;
use smithay::reexports::input::event::pointer::PointerEventTrait;
use smithay::wayland::seat::WaylandFocus;
use smithay::{
    backend::input::{
        AbsolutePositionEvent, Axis, AxisSource, ButtonState, Event, InputEvent, KeyState,
        KeyboardKeyEvent, PointerAxisEvent, PointerButtonEvent, PointerMotionEvent, TouchEvent,
        TouchSlot,
    },
    input::{
        keyboard::{FilterResult, keysyms},
        pointer::{AxisFrame, ButtonEvent, MotionEvent, RelativeMotionEvent},
        touch::{DownEvent, MotionEvent as TouchMotionEvent, UpEvent},
    },
    reexports::{
        input::LibinputInterface,
        rustix::fs::{Mode, OFlags, open},
    },
    utils::{Logical, Point, SERIAL_COUNTER, Serial},
    wayland::pointer_constraints::{PointerConstraint, with_pointer_constraint},
};
use std::{os::unix::io::OwnedFd, path::Path, time::Instant};

pub struct NixInterface;

impl LibinputInterface for NixInterface {
    fn open_restricted(&mut self, path: &Path, flags: i32) -> Result<OwnedFd, i32> {
        open(
            path,
            OFlags::from_bits_truncate(flags as u32),
            Mode::empty(),
        )
        .map_err(|err| err.raw_os_error())
    }
    fn close_restricted(&mut self, fd: OwnedFd) {
        let _ = fd;
    }
}

impl State {
    pub fn scancode_to_keycode(&self, scancode: u32) -> Keycode {
        // see: https://github.com/rust-x-bindings/xkbcommon-rs/blob/cb449998d8a3de375d492fb7ec015b2925f38ddb/src/xkb/mod.rs#L50-L58
        Keycode::new(scancode + 8)
    }

    pub fn keyboard_input(&mut self, event_time_msec: u32, keycode: Keycode, state: KeyState) {
        let serial = SERIAL_COUNTER.next_serial();
        let keyboard = self.seat.get_keyboard().unwrap();

        keyboard.input::<(), _>(
            self,
            keycode,
            state,
            serial,
            event_time_msec,
            |data, modifiers, handle| {
                if state == KeyState::Pressed {
                    if modifiers.ctrl && modifiers.shift && !modifiers.alt && !modifiers.logo {
                        match handle.modified_sym() {
                            Keysym::Tab => {
                                if let Some(element) = data.space.elements().last().cloned() {
                                    data.surpressed_keys.insert(keysyms::KEY_Tab);
                                    let location = data.space.element_location(&element).unwrap();
                                    data.space.map_element(element.clone(), location, true);
                                    data.seat.get_keyboard().unwrap().set_focus(
                                        data,
                                        Some(FocusTarget::from(element)),
                                        serial,
                                    );
                                    return FilterResult::Intercept(());
                                }
                            }
                            Keysym::Q => {
                                if let Some(target) =
                                    data.seat.get_keyboard().unwrap().current_focus()
                                {
                                    match target {
                                        FocusTarget::Wayland(window) => {
                                            window.toplevel().unwrap().send_close();
                                        }
                                        _ => return FilterResult::Forward,
                                    };
                                    data.surpressed_keys.insert(keysyms::KEY_Q);
                                    return FilterResult::Intercept(());
                                }
                            }
                            _ => {}
                        }
                    }
                } else {
                    if data.surpressed_keys.remove(&handle.modified_sym().raw()) {
                        return FilterResult::Intercept(());
                    }
                }

                FilterResult::Forward
            },
        );
    }

    pub(crate) fn maybe_activate_pointer_constraint(
        &self,
        new_under: &Option<(FocusTarget, Point<f64, Logical>)>,
        new_location: Point<f64, Logical>,
    ) {
        let pointer = self.seat.get_pointer().unwrap();

        if let Some((under, surface_location)) = new_under
            .as_ref()
            .and_then(|(target, loc)| Some((target.wl_surface()?, loc)))
        {
            with_pointer_constraint(&under, &pointer, |constraint| match constraint {
                Some(constraint) if !constraint.is_active() => {
                    let point = new_location - *surface_location;
                    if constraint
                        .region()
                        .map_or(true, |region| region.contains(point.to_i32_round()))
                    {
                        constraint.activate();
                    }
                }
                _ => {}
            });
        }
    }

    fn can_pointer_move(
        &self,
        under: &Option<(FocusTarget, Point<f64, Logical>)>,
        target_position: Point<f64, Logical>,
    ) -> bool {
        let pointer = self.seat.get_pointer().unwrap();
        let mut should_motion = true;

        let new_under = self
            .space
            .element_under(target_position)
            .map(|(w, pos)| (w.clone().into(), pos.to_f64()));

        if let Some((surface, surface_loc)) =
            under
                .as_ref()
                .and_then(|(target, l): &(FocusTarget, Point<f64, Logical>)| {
                    Some((target.wl_surface()?, l))
                })
        {
            with_pointer_constraint(&surface, &pointer, |constraint| match constraint {
                Some(constraint) if constraint.is_active() => {
                    // Constraint does not apply if not within region
                    if !constraint.region().map_or(true, |x| {
                        x.contains((pointer.current_location() - *surface_loc).to_i32_round())
                    }) {
                        return;
                    }
                    match &*constraint {
                        PointerConstraint::Locked(_locked) => {
                            should_motion = false;
                        }
                        PointerConstraint::Confined(confine) => {
                            // If confined, don't move pointer if it would go outside surface or region
                            if let Some((surface, surface_loc)) = &under {
                                if new_under.as_ref().and_then(
                                    |(under, _): &(FocusTarget, Point<f64, Logical>)| {
                                        under.wl_surface()
                                    },
                                ) != surface.wl_surface()
                                {
                                    should_motion = false;
                                }
                                if let Some(region) = confine.region() {
                                    if !region
                                        .contains((target_position - *surface_loc).to_i32_round())
                                    {
                                        should_motion = false;
                                    }
                                }
                            }
                        }
                    }
                }
                _ => {}
            });
        }

        // If pointer is now in a constraint region, activate it
        self.maybe_activate_pointer_constraint(&new_under, target_position);

        should_motion
    }

    pub fn pointer_motion(
        &mut self,
        event_time_msec: u32,
        event_time_usec: u64,
        delta: Point<f64, Logical>,
        delta_unaccelerated: Point<f64, Logical>,
    ) {
        self.last_pointer_movement = Instant::now();
        let serial = SERIAL_COUNTER.next_serial();
        let pointer = self.seat.get_pointer().unwrap();
        let under = self
            .space
            .element_under(self.pointer_location)
            .map(|(w, pos)| (w.clone().into(), pos.to_f64()));

        let possible_pos = self.clamp_coords(self.pointer_location + delta);

        // Pointer should only move if it's not locked or confined (and going out of bounds)
        if self.can_pointer_move(&under, possible_pos) {
            self.set_pointer_location(possible_pos);

            // Not sure why, but order here matters (at least when using Sway nested)!!!
            // If we send the motion event after the relative_motion it'll behave oddly
            pointer.motion(
                self,
                under.clone(),
                &MotionEvent {
                    location: self.pointer_location,
                    serial,
                    time: event_time_msec,
                },
            );
        }

        // Relative motion is always applied
        pointer.relative_motion(
            self,
            under.map(|(w, pos)| (w, pos.to_f64())),
            &RelativeMotionEvent {
                delta,
                delta_unaccel: delta_unaccelerated,
                utime: event_time_usec,
            },
        );

        pointer.frame(self);
    }

    pub fn set_pointer_location(&mut self, location: Point<f64, Logical>) {
        self.pointer_location = location;
        self.pointer_absolute_location = location;
    }

    pub fn pointer_motion_absolute(&mut self, event_time_msec: u32, position: Point<f64, Logical>) {
        let relative_movement = (
            position.x - self.pointer_absolute_location.x,
            position.y - self.pointer_absolute_location.y,
        )
            .into();

        self.pointer_motion(
            event_time_msec,
            event_time_msec as u64 * 1000,
            relative_movement,
            relative_movement,
        );
        //  pointer_absolute_location should always point to the unclamped position sent by Moonlight
        self.pointer_absolute_location = position;
    }

    pub fn pointer_button(&mut self, event_time_msec: u32, button_code: u32, state: ButtonState) {
        self.last_pointer_movement = Instant::now();
        let serial = SERIAL_COUNTER.next_serial();

        if ButtonState::Pressed == state {
            self.update_keyboard_focus(serial);
        };
        let pointer = self.seat.get_pointer().unwrap();
        pointer.button(
            self,
            &ButtonEvent {
                button: button_code,
                state: state.try_into().unwrap(),
                serial,
                time: event_time_msec,
            },
        );
        pointer.frame(self);
    }

    pub fn pointer_axis(
        &mut self,
        event_time_msec: u32,
        source: AxisSource,
        horizontal_amount: f64,
        vertical_amount: f64,
        horizontal_amount_discrete: Option<f64>,
        vertical_amount_discrete: Option<f64>,
    ) {
        let mut frame = AxisFrame::new(event_time_msec).source(source);
        if horizontal_amount != 0.0 {
            frame = frame.value(Axis::Horizontal, horizontal_amount);
            if let Some(discrete) = horizontal_amount_discrete {
                frame = frame.v120(Axis::Horizontal, discrete as i32);
            }
        } else if source == AxisSource::Finger {
            frame = frame.stop(Axis::Horizontal);
        }
        if vertical_amount != 0.0 {
            frame = frame.value(Axis::Vertical, vertical_amount);
            if let Some(discrete) = vertical_amount_discrete {
                frame = frame.v120(Axis::Vertical, discrete as i32);
            }
        } else if source == AxisSource::Finger {
            frame = frame.stop(Axis::Vertical);
        }
        let pointer = self.seat.get_pointer().unwrap();
        pointer.axis(self, frame);
        pointer.frame(self);
    }

    fn touch_location_transformed<
        B: smithay::backend::input::InputBackend,
        E: AbsolutePositionEvent<B>,
    >(
        &self,
        evt: &E,
    ) -> Option<Point<f64, Logical>> {
        let output = self
            .space
            .outputs()
            .find(|output| output.name().starts_with("eDP"))
            .or_else(|| self.space.outputs().next())?;

        let output_geometry = self.space.output_geometry(output)?;
        let transform = output.current_transform();
        let size = transform.invert().transform_size(output_geometry.size);

        Some(
            transform.transform_point_in(evt.position_transformed(size), &size.to_f64())
                + output_geometry.loc.to_f64(),
        )
    }

    pub fn relative_touch_to_logical(
        &mut self,
        relative_pos: Point<f64, Logical>, // 0.0 to 1.0
    ) -> Option<Point<f64, Logical>> {
        let output = self
            .space
            .outputs()
            .find(|output| output.name().starts_with("eDP"))
            .or_else(|| self.space.outputs().next())?;

        let output_geometry = self.space.output_geometry(output)?;
        let transform = output.current_transform();

        // Size before transform
        let untransformed_size = transform.invert().transform_size(output_geometry.size);
        let size_f64 = untransformed_size.to_f64();

        // Scaled raw position in untransformed space
        let pos = Point::from((relative_pos.x * size_f64.w, relative_pos.y * size_f64.h));

        // Now apply the output transform
        let transformed_pos = transform.transform_point_in(pos, &size_f64);

        // Map to global logical coordinates
        Some(transformed_pos + output_geometry.loc.to_f64())
    }

    pub fn touch_down(
        &mut self,
        event_time_msec: u32,
        slot: TouchSlot,
        location: Point<f64, Logical>,
    ) {
        let serial = SERIAL_COUNTER.next_serial();
        let touch = self.seat.get_touch().unwrap();
        let under = self
            .space
            .element_under(location)
            .map(|(w, pos)| (w.clone().into(), pos.to_f64()));

        touch.down(
            self,
            under,
            &DownEvent {
                slot: slot,
                location: location,
                serial,
                time: event_time_msec,
            },
        );
        touch.frame(self);
    }

    pub fn touch_up(&mut self, event_time_msec: u32, slot: TouchSlot) {
        let serial = SERIAL_COUNTER.next_serial();
        let touch = self.seat.get_touch().unwrap();

        touch.up(
            self,
            &UpEvent {
                slot: slot,
                serial,
                time: event_time_msec,
            },
        );
        touch.frame(self);
    }

    pub fn touch_motion(
        &mut self,
        event_time_msec: u32,
        slot: TouchSlot,
        location: Point<f64, Logical>,
    ) {
        let touch = self.seat.get_touch().unwrap();
        let under = self
            .space
            .element_under(location)
            .map(|(w, pos)| (w.clone().into(), pos.to_f64()));

        touch.motion(
            self,
            under,
            &TouchMotionEvent {
                slot,
                location,
                time: event_time_msec,
            },
        );
        touch.frame(self);
    }

    pub fn touch_cancel(&mut self) {
        let touch = self.seat.get_touch().unwrap();
        touch.cancel(self);
    }

    pub fn touch_frame(&mut self) {
        let touch = self.seat.get_touch().unwrap();
        touch.frame(self);
    }

    pub fn process_input_event(&mut self, event: InputEvent<LibinputInputBackend>) {
        match event {
            InputEvent::Keyboard { event, .. } => {
                self.keyboard_input(event.time_msec(), event.key_code(), event.state());
            }
            InputEvent::PointerMotion { event, .. } => {
                self.pointer_motion(
                    event.time_msec(),
                    event.time_usec(),
                    event.delta(),
                    event.delta_unaccel(),
                );
            }
            InputEvent::PointerMotionAbsolute { event } => {
                if let Some(output) = self.output.as_ref() {
                    let output_size = output
                        .current_mode()
                        .unwrap()
                        .size
                        .to_f64()
                        .to_logical(output.current_scale().fractional_scale())
                        .to_i32_round();

                    let new_x = event.x_transformed(output_size.w);
                    let new_y = event.y_transformed(output_size.h);

                    self.pointer_motion_absolute(event.time_msec(), (new_x, new_y).into());
                }
            }
            InputEvent::PointerButton { event, .. } => {
                self.pointer_button(event.time_msec(), event.button(), event.state());
            }
            InputEvent::PointerAxis { event, .. } => {
                self.last_pointer_movement = Instant::now();
                let horizontal_amount = event
                    .amount(Axis::Horizontal)
                    .or_else(|| event.amount_v120(Axis::Horizontal).map(|x| x * 3.0 / 120.0))
                    .unwrap_or(0.0);
                let vertical_amount = event
                    .amount(Axis::Vertical)
                    .or_else(|| event.amount_v120(Axis::Vertical).map(|y| y * 3.0 / 120.0))
                    .unwrap_or(0.0);
                let horizontal_amount_discrete = event.amount_v120(Axis::Horizontal);
                let vertical_amount_discrete = event.amount_v120(Axis::Vertical);

                self.pointer_axis(
                    event.time_msec(),
                    event.source(),
                    horizontal_amount,
                    vertical_amount,
                    horizontal_amount_discrete,
                    vertical_amount_discrete,
                );
            }
            InputEvent::TouchDown { event, .. } => {
                if let Some(location) = self.touch_location_transformed(&event) {
                    self.touch_down(event.time_msec(), event.slot(), location);
                }
            }
            InputEvent::TouchUp { event, .. } => {
                self.touch_up(event.time_msec(), event.slot());
            }
            InputEvent::TouchMotion { event, .. } => {
                if let Some(location) = self.touch_location_transformed(&event) {
                    self.touch_motion(event.time_msec(), event.slot(), location);
                }
            }
            InputEvent::TouchCancel { .. } => {
                self.touch_cancel();
            }
            InputEvent::TouchFrame { .. } => {
                self.touch_frame();
            }
            _ => {}
        }
    }

    fn clamp_coords(&self, pos: Point<f64, Logical>) -> Point<f64, Logical> {
        if let Some(output) = self.output.as_ref() {
            if let Some(mode) = output.current_mode() {
                return (
                    pos.x.max(0.0).min((mode.size.w - 2) as f64),
                    pos.y.max(0.0).min((mode.size.h - 2) as f64),
                )
                    .into();
            }
        }
        pos
    }

    fn update_keyboard_focus(&mut self, serial: Serial) {
        let pointer = self.seat.get_pointer().unwrap();
        let keyboard = self.seat.get_keyboard().unwrap();
        // change the keyboard focus unless the pointer or keyboard is grabbed
        // We test for any matching surface type here but always use the root
        // (in case of a window the toplevel) surface for the focus.
        // So for example if a user clicks on a subsurface or popup the toplevel
        // will receive the keyboard focus. Directly assigning the focus to the
        // matching surface leads to issues with clients dismissing popups and
        // subsurface menus (for example firefox-wayland).
        // see here for a discussion about that issue:
        // https://gitlab.freedesktop.org/wayland/wayland/-/issues/294
        if !pointer.is_grabbed() && !keyboard.is_grabbed() {
            if let Some((window, _)) = self
                .space
                .element_under(self.pointer_location)
                .map(|(w, p)| (w.clone(), p))
            {
                self.space.raise_element(&window, true);
                keyboard.set_focus(self, Some(FocusTarget::from(window)), serial);
                return;
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::comp::State;
    use crate::utils::RenderTarget;
    use smithay::input::keyboard::xkb;
    use smithay::output::{Output, PhysicalProperties, Subpixel};
    use smithay::reexports::calloop::EventLoop;
    use smithay::reexports::input::Libinput;
    use smithay::{reexports::wayland_server::Display, utils::Point};

    struct TestState {
        state: State,
    }

    impl TestState {
        fn new() -> Self {
            let libinput_context = Libinput::new_from_path(NixInterface);
            let event_loop = EventLoop::<State>::try_new().expect("Unable to create event_loop");
            let display = Display::<State>::new().unwrap();
            let dh = display.handle();

            let state = State::new(
                &RenderTarget::Software,
                &dh,
                &libinput_context,
                event_loop.handle(),
            );

            TestState { state }
        }

        fn state(&mut self) -> &mut State {
            &mut self.state
        }
    }

    #[test]
    fn keyboard_scancode_conversion() {
        let mut harness = TestState::new();
        let state = harness.state();

        let context = xkb::Context::new(xkb::CONTEXT_NO_FLAGS);
        let keymap =
            xkb::Keymap::new_from_names(&context, "", "", "us", "", None, xkb::COMPILE_NO_FLAGS)
                .unwrap();
        let xkb_state = xkb::State::new(&keymap);

        // Evdev keycode, from `input-event-codes.h`
        // Linux evdev keycode (30)
        const KEY_A: u32 = 30;
        //     ↓ +8
        // X11 keycode (38)
        let x11_key_code = state.scancode_to_keycode(KEY_A);
        //     ↓ keymap lookup
        // X11 keysym (0x61 = 'a')
        let x11_keysym = xkb_state.key_get_one_sym(x11_key_code);

        assert_eq!(Keysym::a, x11_keysym);
    }

    #[test]
    fn keyboard_input() {
        let mut harness = TestState::new();
        let state = harness.state();
        let kb = state.seat.get_keyboard().unwrap();

        const KEY_A: u32 = 30;
        let test_key_code = state.scancode_to_keycode(KEY_A);
        state.keyboard_input(0, test_key_code, KeyState::Pressed);
        assert!(kb.pressed_keys().contains(&test_key_code));

        state.keyboard_input(0, test_key_code, KeyState::Released);
        assert!(kb.pressed_keys().is_empty());
    }

    #[test]
    fn pointer_motion_moves_pointer_location() {
        let mut harness = TestState::new();
        let state = harness.state();

        state.pointer_location = Point::from((50.0, 50.0));
        let delta = Point::from((15.5, -5.0));
        let expected_location = Point::from((65.5, 45.0));

        // Call the method to test
        state.pointer_motion(0, 0, delta, delta);

        // Check that the internal pointer location is updated
        assert_eq!(state.pointer_location, expected_location);

        // Check that the pointer's location is also updated
        let pointer = state.seat.get_pointer().unwrap();
        assert_eq!(pointer.current_location(), expected_location);
    }

    #[test]
    fn pointer_motion_absolute_moves_pointer_location() {
        let mut harness = TestState::new();
        let state = harness.state();

        state.set_pointer_location(Point::from((50.0, 50.0)));
        let expected_location = Point::from((65.5, 45.0));

        state.pointer_motion_absolute(0, expected_location);

        assert_eq!(state.pointer_location, expected_location);
        let pointer = state.seat.get_pointer().unwrap();
        assert_eq!(pointer.current_location(), expected_location);
    }

    #[test]
    fn clamp_coords_keeps_within_bounds() {
        let mut harness = TestState::new();
        let state = harness.state();
        let output = Output::new(
            "HEADLESS-1".into(),
            PhysicalProperties {
                make: "Virtual".into(),
                model: "Wolf".into(),
                size: (0, 0).into(),
                subpixel: Subpixel::Unknown,
            },
        );
        output.create_global::<State>(&state.dh);
        output.change_current_state(
            Some(smithay::output::Mode {
                size: (10, 10).into(),
                refresh: 1000,
            }),
            None,
            None,
            None,
        );
        state.output = Some(output);

        let extreme_pos = Point::from((-100.0, 5000.0));
        let clamped = state.clamp_coords(extreme_pos);

        // Should clamp negative x to 0 and large y to 10
        assert!(clamped.x >= 0.0);
        assert!(clamped.y <= 10.0);
    }
}
