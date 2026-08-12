# Spec task browser Back gesture

## Symptom

Browser Back/Forward trackpad gestures stopped working after opening a spec task with a live desktop.

## Cause

`DesktopStreamViewer` cancelled every wheel event over the viewer. The viewer occupies much of the task detail page, so Chrome and Safari could not receive horizontal Back/Forward gestures even when the user had not interacted with the remote desktop.

## Fix

Only forward and cancel wheel events while focus is inside the remote desktop. Before focus, the browser owns the gesture. Clicking the viewer focuses it and preserves remote scrolling.
