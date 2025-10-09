// Because ios devices suck, they can't make a right click when holding a touch.
// This script will hook into the touch apis to simulate a right click when needed

const RIGHT_CLICK_TIME_MS = 400
const RIGHT_CLICK_MAX_MOVE = 40

let rightClickEnabled = false

/// This might or might not disable all touch events and will likely simulate click / contextmenu events
export function setTouchContextMenuEnabled(enabled: boolean) {
    if (navigator?.vendor == "Apple Computer, Inc.") {
        rightClickEnabled = enabled
    }
}

const touchTracker: Map<number, {
    originX: number
    originY: number
    startTime: number
    startTarget: Element | null
    oldX: number
    oldY: number
}> = new Map()

function onTouchStart(event: TouchEvent) {
    for (const touch of event.changedTouches) {
        touchTracker.set(touch.identifier, {
            originX: touch.clientX,
            originY: touch.clientY,
            startTime: Date.now(),
            startTarget: touch?.target as Element ?? null,
            oldX: touch.clientX,
            oldY: touch.clientY
        })
    }

    if (!rightClickEnabled) {
        return
    }
    event.preventDefault()
    event.stopImmediatePropagation()
}
function onTouchMove(event: TouchEvent) {
    if (!rightClickEnabled) {
        return
    }
    event.preventDefault()
    event.stopImmediatePropagation()

    for (const touch of event.changedTouches) {
        const tracker = touchTracker.get(touch.identifier)
        if (!tracker) {
            continue
        }

        const deltaX = tracker.oldX - touch.clientX
        const deltaY = tracker.oldY - touch.clientY

        const element = tracker.startTarget?.closest(".scrollable") ?? window
        element.scrollBy({
            top: deltaY,
            left: deltaX,
            behavior: "instant"
        })

        tracker.oldX = touch.clientX
        tracker.oldY = touch.clientY
    }
}
function onTouchEnd(event: TouchEvent) {
    if (!rightClickEnabled) {
        removeTouch(event)
        return
    }

    event.stopImmediatePropagation()

    for (const touch of event.changedTouches) {
        const touchStart = touchTracker.get(touch.identifier)
        if (!touchStart) {
            continue
        }

        const timeDiff = Date.now() - touchStart.startTime

        const eventInit = {
            clientX: touch.clientX,
            clientY: touch.clientY,
            force: touch.force,
            pageX: touch.pageX,
            pageY: touch.pageY,
            radiusX: touch.radiusX,
            radiusY: touch.radiusY,
            rotationAngle: touch.rotationAngle,
            screenX: touch.screenX,
            screenY: touch.screenY,
            target: touch.target,
            // Other
            bubbles: true,
            cancellable: true
        };

        if (touch.clientX - touchStart.originX < RIGHT_CLICK_MAX_MOVE
            && touch.clientY - touchStart.originY < RIGHT_CLICK_MAX_MOVE) {
            if (timeDiff > RIGHT_CLICK_TIME_MS) {
                // dispatch fake context menu event
                const contextMenuEvent = new MouseEvent("contextmenu", eventInit)

                touch?.target.dispatchEvent(contextMenuEvent)
            } else {
                // dispatch click
                const clickEvent = new MouseEvent("click", eventInit)

                if ("target" in touch) {
                    touch.target.dispatchEvent(clickEvent)
                    if ("focus" in touch.target && typeof touch.target.focus == "function") {
                        touch.target.focus()
                    }
                }
            }
        }
    }

    removeTouch(event)
}
function removeTouch(event: TouchEvent) {
    for (const touch of event.changedTouches) {
        touchTracker.delete(touch.identifier)
    }

    if (!rightClickEnabled) {
        return
    }
    event.stopImmediatePropagation()
}

window.addEventListener("touchstart", onTouchStart, { capture: true, passive: false })
window.addEventListener("touchmove", onTouchMove, { capture: true, passive: false })
window.addEventListener("touchend", onTouchEnd, { capture: true, passive: false })
window.addEventListener("touchcancel", onTouchEnd, { capture: true, passive: false })