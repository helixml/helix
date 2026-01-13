use crate::comp::State;
use smithay::{
    backend::input::KeyState,
    desktop::{PopupKind, Window, WindowSurface},
    input::{
        Seat,
        keyboard::{KeyboardTarget, KeysymHandle, ModifiersState},
        pointer::{
            AxisFrame, ButtonEvent, GestureHoldBeginEvent, GestureHoldEndEvent,
            GesturePinchBeginEvent, GesturePinchEndEvent, GesturePinchUpdateEvent,
            GestureSwipeBeginEvent, GestureSwipeEndEvent, GestureSwipeUpdateEvent, MotionEvent,
            PointerTarget, RelativeMotionEvent,
        },
        touch::{DownEvent, OrientationEvent, ShapeEvent, TouchTarget, UpEvent},
    },
    reexports::wayland_server::{backend::ObjectId, protocol::wl_surface::WlSurface},
    utils::{IsAlive, Serial},
    wayland::seat::WaylandFocus,
};
use std::borrow::Cow;

#[derive(Debug, Clone, PartialEq)]
pub enum FocusTarget {
    Wayland(Window),
    Popup(PopupKind),
}

impl IsAlive for FocusTarget {
    fn alive(&self) -> bool {
        match self {
            FocusTarget::Wayland(w) => w.alive(),
            FocusTarget::Popup(p) => p.alive(),
        }
    }
}

impl From<Window> for FocusTarget {
    fn from(w: Window) -> Self {
        FocusTarget::Wayland(w)
    }
}

impl From<PopupKind> for FocusTarget {
    fn from(p: PopupKind) -> Self {
        FocusTarget::Popup(p)
    }
}

impl KeyboardTarget<State> for FocusTarget {
    fn enter(
        &self,
        seat: &Seat<State>,
        data: &mut State,
        keys: Vec<KeysymHandle<'_>>,
        serial: Serial,
    ) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    KeyboardTarget::enter(w.wl_surface(), seat, data, keys, serial)
                }
            },
            FocusTarget::Popup(p) => {
                KeyboardTarget::enter(p.wl_surface(), seat, data, keys, serial)
            }
        }
    }

    fn leave(&self, seat: &Seat<State>, data: &mut State, serial: Serial) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    KeyboardTarget::leave(w.wl_surface(), seat, data, serial)
                }
            },
            FocusTarget::Popup(p) => KeyboardTarget::leave(p.wl_surface(), seat, data, serial),
        }
    }

    fn key(
        &self,
        seat: &Seat<State>,
        data: &mut State,
        key: KeysymHandle<'_>,
        state: KeyState,
        serial: Serial,
        time: u32,
    ) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    KeyboardTarget::key(w.wl_surface(), seat, data, key, state, serial, time)
                }
            },
            FocusTarget::Popup(p) => {
                KeyboardTarget::key(p.wl_surface(), seat, data, key, state, serial, time)
            }
        }
    }

    fn modifiers(
        &self,
        seat: &Seat<State>,
        data: &mut State,
        modifiers: ModifiersState,
        serial: Serial,
    ) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    KeyboardTarget::modifiers(w.wl_surface(), seat, data, modifiers, serial)
                }
            },
            FocusTarget::Popup(p) => p.wl_surface().modifiers(seat, data, modifiers, serial),
        }
    }
}

impl PointerTarget<State> for FocusTarget {
    fn enter(&self, seat: &Seat<State>, data: &mut State, event: &MotionEvent) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    PointerTarget::enter(w.wl_surface(), seat, data, event)
                }
            },
            FocusTarget::Popup(p) => PointerTarget::enter(p.wl_surface(), seat, data, event),
        }
    }

    fn motion(&self, seat: &Seat<State>, data: &mut State, event: &MotionEvent) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    PointerTarget::motion(w.wl_surface(), seat, data, event)
                }
            },
            FocusTarget::Popup(p) => PointerTarget::motion(p.wl_surface(), seat, data, event),
        }
    }

    fn relative_motion(&self, seat: &Seat<State>, data: &mut State, event: &RelativeMotionEvent) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    PointerTarget::relative_motion(w.wl_surface(), seat, data, event)
                }
            },
            FocusTarget::Popup(p) => {
                PointerTarget::relative_motion(p.wl_surface(), seat, data, event)
            }
        }
    }

    fn button(&self, seat: &Seat<State>, data: &mut State, event: &ButtonEvent) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    PointerTarget::button(w.wl_surface(), seat, data, event)
                }
            },
            FocusTarget::Popup(p) => PointerTarget::button(p.wl_surface(), seat, data, event),
        }
    }

    fn axis(&self, seat: &Seat<State>, data: &mut State, frame: AxisFrame) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => PointerTarget::axis(w.wl_surface(), seat, data, frame),
            },
            FocusTarget::Popup(p) => PointerTarget::axis(p.wl_surface(), seat, data, frame),
        }
    }

    fn frame(&self, seat: &Seat<State>, data: &mut State) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => PointerTarget::frame(w.wl_surface(), seat, data),
            },
            FocusTarget::Popup(p) => PointerTarget::frame(p.wl_surface(), seat, data),
        }
    }

    fn gesture_swipe_begin(
        &self,
        seat: &Seat<State>,
        data: &mut State,
        event: &GestureSwipeBeginEvent,
    ) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    PointerTarget::gesture_swipe_begin(w.wl_surface(), seat, data, event)
                }
            },
            FocusTarget::Popup(p) => {
                PointerTarget::gesture_swipe_begin(p.wl_surface(), seat, data, event)
            }
        }
    }

    fn gesture_swipe_update(
        &self,
        seat: &Seat<State>,
        data: &mut State,
        event: &GestureSwipeUpdateEvent,
    ) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    PointerTarget::gesture_swipe_update(w.wl_surface(), seat, data, event)
                }
            },
            FocusTarget::Popup(p) => {
                PointerTarget::gesture_swipe_update(p.wl_surface(), seat, data, event)
            }
        }
    }

    fn gesture_swipe_end(
        &self,
        seat: &Seat<State>,
        data: &mut State,
        event: &GestureSwipeEndEvent,
    ) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    PointerTarget::gesture_swipe_end(w.wl_surface(), seat, data, event)
                }
            },
            FocusTarget::Popup(p) => {
                PointerTarget::gesture_swipe_end(p.wl_surface(), seat, data, event)
            }
        }
    }

    fn gesture_pinch_begin(
        &self,
        seat: &Seat<State>,
        data: &mut State,
        event: &GesturePinchBeginEvent,
    ) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    PointerTarget::gesture_pinch_begin(w.wl_surface(), seat, data, event)
                }
            },
            FocusTarget::Popup(p) => {
                PointerTarget::gesture_pinch_begin(p.wl_surface(), seat, data, event)
            }
        }
    }

    fn gesture_pinch_update(
        &self,
        seat: &Seat<State>,
        data: &mut State,
        event: &GesturePinchUpdateEvent,
    ) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    PointerTarget::gesture_pinch_update(w.wl_surface(), seat, data, event)
                }
            },
            FocusTarget::Popup(p) => {
                PointerTarget::gesture_pinch_update(p.wl_surface(), seat, data, event)
            }
        }
    }

    fn gesture_pinch_end(
        &self,
        seat: &Seat<State>,
        data: &mut State,
        event: &GesturePinchEndEvent,
    ) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    PointerTarget::gesture_pinch_end(w.wl_surface(), seat, data, event)
                }
            },
            FocusTarget::Popup(p) => {
                PointerTarget::gesture_pinch_end(p.wl_surface(), seat, data, event)
            }
        }
    }

    fn gesture_hold_begin(
        &self,
        seat: &Seat<State>,
        data: &mut State,
        event: &GestureHoldBeginEvent,
    ) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    PointerTarget::gesture_hold_begin(w.wl_surface(), seat, data, event)
                }
            },
            FocusTarget::Popup(p) => {
                PointerTarget::gesture_hold_begin(p.wl_surface(), seat, data, event)
            }
        }
    }

    fn gesture_hold_end(&self, seat: &Seat<State>, data: &mut State, event: &GestureHoldEndEvent) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    PointerTarget::gesture_hold_end(w.wl_surface(), seat, data, event)
                }
            },
            FocusTarget::Popup(p) => {
                PointerTarget::gesture_hold_end(p.wl_surface(), seat, data, event)
            }
        }
    }

    fn leave(&self, seat: &Seat<State>, data: &mut State, serial: Serial, time: u32) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    PointerTarget::leave(w.wl_surface(), seat, data, serial, time)
                }
            },
            FocusTarget::Popup(p) => PointerTarget::leave(p.wl_surface(), seat, data, serial, time),
        }
    }
}

impl WaylandFocus for FocusTarget {
    fn wl_surface(&self) -> Option<Cow<'_, WlSurface>> {
        match self {
            FocusTarget::Wayland(w) => w.wl_surface(),
            FocusTarget::Popup(p) => Some(Cow::Borrowed(p.wl_surface())),
        }
    }

    fn same_client_as(&self, object_id: &ObjectId) -> bool {
        match self {
            FocusTarget::Wayland(w) => w.same_client_as(object_id),
            FocusTarget::Popup(p) => p.wl_surface().same_client_as(object_id),
        }
    }
}

impl TouchTarget<State> for FocusTarget {
    fn down(&self, seat: &Seat<State>, data: &mut State, event: &DownEvent, seq: Serial) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    TouchTarget::down(w.wl_surface(), seat, data, event, seq)
                }
            },
            FocusTarget::Popup(p) => TouchTarget::down(p.wl_surface(), seat, data, event, seq),
        }
    }

    fn up(&self, seat: &Seat<State>, data: &mut State, event: &UpEvent, seq: Serial) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    TouchTarget::up(w.wl_surface(), seat, data, event, seq)
                }
            },
            FocusTarget::Popup(p) => TouchTarget::up(p.wl_surface(), seat, data, event, seq),
        }
    }

    fn motion(
        &self,
        seat: &Seat<State>,
        data: &mut State,
        event: &smithay::input::touch::MotionEvent,
        seq: Serial,
    ) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    TouchTarget::motion(w.wl_surface(), seat, data, event, seq)
                }
            },
            FocusTarget::Popup(p) => TouchTarget::motion(p.wl_surface(), seat, data, event, seq),
        }
    }

    fn frame(&self, seat: &Seat<State>, data: &mut State, seq: Serial) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => TouchTarget::frame(w.wl_surface(), seat, data, seq),
            },
            FocusTarget::Popup(p) => TouchTarget::frame(p.wl_surface(), seat, data, seq),
        }
    }

    fn cancel(&self, seat: &Seat<State>, data: &mut State, seq: Serial) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => TouchTarget::cancel(w.wl_surface(), seat, data, seq),
            },
            FocusTarget::Popup(p) => TouchTarget::cancel(p.wl_surface(), seat, data, seq),
        }
    }

    fn shape(&self, seat: &Seat<State>, data: &mut State, event: &ShapeEvent, seq: Serial) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    TouchTarget::shape(w.wl_surface(), seat, data, event, seq)
                }
            },
            FocusTarget::Popup(p) => TouchTarget::shape(p.wl_surface(), seat, data, event, seq),
        }
    }

    fn orientation(
        &self,
        seat: &Seat<State>,
        data: &mut State,
        event: &OrientationEvent,
        seq: Serial,
    ) {
        match self {
            FocusTarget::Wayland(w) => match w.underlying_surface() {
                WindowSurface::Wayland(w) => {
                    TouchTarget::orientation(w.wl_surface(), seat, data, event, seq)
                }
            },
            FocusTarget::Popup(p) => {
                TouchTarget::orientation(p.wl_surface(), seat, data, event, seq)
            }
        }
    }
}
